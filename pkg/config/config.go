package config

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/balancer"
	"github.com/gabrielopesantos/reverse-proxy/pkg/middleware"
	"gopkg.in/yaml.v3"
)

const (
	DefaultPath = "examples/config.yaml"
)

type Config struct {
	Routes map[string]*Route `yaml:"routes"`
	mu     sync.RWMutex

	configPath      string
	watchInterval   time.Duration
	reloadCallbacks []func()
}

// Option configures a Config at construction time.
type Option func(*Config)

func WithWatchInterval(d time.Duration) Option {
	return func(c *Config) { c.watchInterval = d }
}

type Route struct {
	Upstreams            []string                      `yaml:"upstreams"`
	LoadBalancerStrategy balancer.LoadBalancerStrategy `yaml:"lb_strategy"`
	// Weights maps upstream URL to its relative weight for weighted_round_robin.
	// Omitted hosts default to weight 1.
	Weights                    map[string]int `yaml:"weights"`
	HealthCheckIntervalSeconds uint           `yaml:"healthcheck_interval_seconds"`
	// HealthCheckPath is the HTTP path used for upstream health probes.
	// Defaults to "/" when empty.
	HealthCheckPath string `yaml:"healthcheck_path"`
	// MiddlewareInternalRepr is an ordered list of single-key maps: [{type: config}, ...]
	MiddlewareInternalRepr []map[middleware.MiddlewareType]interface{} `yaml:"middleware"`

	middlewareList []middleware.Middleware
}

func (r *Route) Middleware() []middleware.Middleware {
	return r.middlewareList
}

// Snapshot returns a stable copy of the routes map under the read lock.
func (c *Config) Snapshot() map[string]*Route {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]*Route, len(c.Routes))
	for k, v := range c.Routes {
		out[k] = v
	}
	return out
}

// OnReload registers a callback that is called after each successful config reload.
func (c *Config) OnReload(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reloadCallbacks = append(c.reloadCallbacks, fn)
}

func LoadConfig(ctx context.Context, logger *slog.Logger, configPath string, opts ...Option) (*Config, error) {
	cfg := &Config{
		configPath:    configPath,
		watchInterval: 5 * time.Second,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if err := readConfigFile(ctx, logger, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Watch(ctx context.Context, logger *slog.Logger) error {
	ticker := time.NewTicker(c.watchInterval)
	defer ticker.Stop()
	var lastHash [32]byte
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			data, err := os.ReadFile(c.configPath)
			if err != nil {
				logger.Warn("could not read config file", "err", err)
				continue
			}
			hash := sha256.Sum256(data)
			if hash == lastHash {
				continue
			}
			if err := readConfigFile(ctx, logger, c); err != nil {
				logger.Warn("could not parse updated config file", "err", err)
				continue
			}
			lastHash = hash
			c.mu.RLock()
			callbacks := make([]func(), len(c.reloadCallbacks))
			copy(callbacks, c.reloadCallbacks)
			c.mu.RUnlock()
			for _, fn := range callbacks {
				fn()
			}
		}
	}
}

func readConfigFile(ctx context.Context, logger *slog.Logger, config *Config) error {
	config.mu.Lock()
	defer config.mu.Unlock()

	configFile, err := os.ReadFile(config.configPath)
	if err != nil {
		return err
	}

	if err = yaml.Unmarshal(configFile, config); err != nil {
		return err
	}

	ctx = middleware.ContextWithLogger(ctx, logger)
	return parseRoutesMiddleware(ctx, config)
}

// middlewareFactory creates and initialises a Middleware from its raw YAML encoding.
type middlewareFactory func(ctx context.Context, enc []byte) (middleware.Middleware, error)

var middlewareRegistry = map[middleware.MiddlewareType]middlewareFactory{
	middleware.LOGGER: func(ctx context.Context, enc []byte) (middleware.Middleware, error) {
		cfg := &middleware.LoggerConfig{}
		if err := yaml.Unmarshal(enc, cfg); err != nil {
			return nil, err
		}
		if err := cfg.Init(ctx); err != nil {
			return nil, err
		}
		return cfg, nil
	},

	middleware.RATE_LIMITER: func(ctx context.Context, enc []byte) (middleware.Middleware, error) {
		cfg := &middleware.RateLimiterConfig{}
		if err := yaml.Unmarshal(enc, cfg); err != nil {
			return nil, err
		}
		if err := cfg.Init(ctx); err != nil {
			return nil, err
		}
		return cfg, nil
	},

	middleware.BASIC_AUTH: func(ctx context.Context, enc []byte) (middleware.Middleware, error) {
		cfg := &middleware.BasicAuthConfig{}
		if err := yaml.Unmarshal(enc, cfg); err != nil {
			return nil, err
		}
		if err := cfg.Init(ctx); err != nil {
			return nil, err
		}
		return cfg, nil
	},

	middleware.CACHE_CONTROL: func(ctx context.Context, enc []byte) (middleware.Middleware, error) {
		cfg := &middleware.CacheControlConfig{}
		if err := yaml.Unmarshal(enc, cfg); err != nil {
			return nil, err
		}
		if err := cfg.Init(ctx); err != nil {
			return nil, err
		}
		return cfg, nil
	},

	middleware.PROMETHEUS: func(ctx context.Context, enc []byte) (middleware.Middleware, error) {
		cfg := &middleware.PrometheusConfig{}
		if err := yaml.Unmarshal(enc, cfg); err != nil {
			return nil, err
		}
		if err := cfg.Init(ctx); err != nil {
			return nil, err
		}
		return cfg, nil
	},

	middleware.WAF: func(ctx context.Context, enc []byte) (middleware.Middleware, error) {
		cfg := &middleware.WAFConfig{}
		if err := yaml.Unmarshal(enc, cfg); err != nil {
			return nil, err
		}
		if err := cfg.Init(ctx); err != nil {
			return nil, err
		}
		return cfg, nil
	},
}

func parseRoutesMiddleware(ctx context.Context, config *Config) error {
	for _, routeConfig := range config.Routes {
		routeConfig.middlewareList = routeConfig.middlewareList[:0]
		for _, entry := range routeConfig.MiddlewareInternalRepr {
			for mwType, mwConfig := range entry {
				ctx := middleware.ContextWithMiddlewareType(ctx, string(mwType))
				factory, ok := middlewareRegistry[mwType]
				if !ok {
					return fmt.Errorf("unknown middleware type: %s", mwType)
				}
				enc, err := yaml.Marshal(mwConfig)
				if err != nil {
					return fmt.Errorf("failed to marshal middleware config for type %s: %w", mwType, err)
				}
				mw, err := factory(ctx, enc)
				if err != nil {
					return fmt.Errorf("failed to initialize middleware %s: %w", mwType, err)
				}
				routeConfig.middlewareList = append(routeConfig.middlewareList, mw)
			}
		}
	}

	return nil
}
