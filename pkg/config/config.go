package config

import (
	"context"
	"encoding/json"
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
	Server `yaml:"server"`
	Routes map[string]*Route `yaml:"routes"`
	sync.RWMutex

	configPath      string
	reloadCallbacks []func()
}

type Server struct {
	Address            string `yaml:"address"`
	ReadTimeoutSeconds uint   `yaml:"read_timeout"`
}

type Route struct {
	Upstreams                  []string                    `yaml:"upstreams"`
	LoadBalancerPolicy         balancer.LoadBalancerPolicy `yaml:"lb_policy"`
	// Weights maps upstream URL to its relative weight for weighted_round_robin.
	// Omitted hosts default to weight 1.
	Weights                    map[string]int              `yaml:"weights"`
	HealthCheckIntervalSeconds uint                        `yaml:"healthcheck_interval_seconds"`
	// Middleware is an ordered list of single-key maps: [{type: config}, ...]
	MiddlewareInternalRepr []map[middleware.MiddlewareType]interface{} `yaml:"middleware"`

	middlewareList []middleware.Middleware
}

func (r *Route) Middleware() []middleware.Middleware {
	return r.middlewareList
}

// OnReload registers a callback that is called after each successful config reload.
func (c *Config) OnReload(fn func()) {
	c.Lock()
	defer c.Unlock()
	c.reloadCallbacks = append(c.reloadCallbacks, fn)
}

func LoadConfig(configPath string) (*Config, error) {
	config := &Config{configPath: configPath}
	err := readConfig(config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func (c *Config) Watch(logger *slog.Logger) error {
	for {
		time.Sleep(5 * time.Second)
		if err := readConfig(c); err != nil {
			logger.Warn(fmt.Sprintf("could not read updated configuration file: %v", err))
			continue
		}
		c.RLock()
		callbacks := make([]func(), len(c.reloadCallbacks))
		copy(callbacks, c.reloadCallbacks)
		c.RUnlock()
		for _, fn := range callbacks {
			fn()
		}
	}
}

func readConfig(config *Config) error {
	config.Lock()
	defer config.Unlock()

	configFileContent, err := os.ReadFile(config.configPath)
	if err != nil {
		return err
	}

	if err = yaml.Unmarshal(configFileContent, config); err != nil {
		return err
	}

	if err = parseRoutesMiddleware(config); err != nil {
		return err
	}

	return nil
}

// middlewareFactory creates and initialises a Middleware from its raw JSON encoding.
type middlewareFactory func(enc []byte) (middleware.Middleware, error)

var middlewareRegistry = map[middleware.MiddlewareType]middlewareFactory{
	middleware.LOGGER: func(enc []byte) (middleware.Middleware, error) {
		cfg := &middleware.LoggerConfig{}
		if err := json.Unmarshal(enc, cfg); err != nil {
			return nil, err
		}
		if err := cfg.Init(context.TODO()); err != nil {
			return nil, err
		}
		return cfg, nil
	},
	middleware.RATE_LIMITER: func(enc []byte) (middleware.Middleware, error) {
		cfg := &middleware.RateLimiterConfig{}
		if err := json.Unmarshal(enc, cfg); err != nil {
			return nil, err
		}
		if err := cfg.Init(context.TODO()); err != nil {
			return nil, err
		}
		return cfg, nil
	},
	middleware.BASIC_AUTH: func(enc []byte) (middleware.Middleware, error) {
		cfg := &middleware.BasicAuthConfig{}
		if err := json.Unmarshal(enc, cfg); err != nil {
			return nil, err
		}
		if err := cfg.Init(context.TODO()); err != nil {
			return nil, err
		}
		return cfg, nil
	},
	middleware.CACHE_CONTROL: func(enc []byte) (middleware.Middleware, error) {
		cfg := &middleware.CacheControlConfig{}
		if err := json.Unmarshal(enc, cfg); err != nil {
			return nil, err
		}
		if err := cfg.Init(context.TODO()); err != nil {
			return nil, err
		}
		return cfg, nil
	},
	middleware.PROMETHEUS: func(enc []byte) (middleware.Middleware, error) {
		cfg := &middleware.PrometheusConfig{}
		if err := json.Unmarshal(enc, cfg); err != nil {
			return nil, err
		}
		if err := cfg.Init(context.TODO()); err != nil {
			return nil, err
		}
		return cfg, nil
	},
}

func parseRoutesMiddleware(config *Config) error {
	for _, routeConfig := range config.Routes {
		routeConfig.middlewareList = routeConfig.middlewareList[:0]
		for _, entry := range routeConfig.MiddlewareInternalRepr {
			for mwType, mwConfig := range entry {
				factory, ok := middlewareRegistry[mwType]
				if !ok {
					return fmt.Errorf("unknown middleware type: %s", mwType)
				}
				enc, err := json.Marshal(mwConfig)
				if err != nil {
					return fmt.Errorf("failed to marshal middleware config for type %s: %w", mwType, err)
				}
				mw, err := factory(enc)
				if err != nil {
					return fmt.Errorf("failed to initialize middleware %s: %w", mwType, err)
				}
				routeConfig.middlewareList = append(routeConfig.middlewareList, mw)
			}
		}
	}

	return nil
}
