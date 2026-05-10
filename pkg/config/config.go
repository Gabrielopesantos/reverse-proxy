package config

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gabrielopesantos/reverse-proxy/pkg/balancer"
	"github.com/gabrielopesantos/reverse-proxy/pkg/middleware"
	"gopkg.in/yaml.v3"
)

const (
	// fsnotifyDebounce coalesces rapid bursts of write events from editors that
	// touch the file multiple times during a save.
	fsnotifyDebounce = 100 * time.Millisecond
)

type Config struct {
	Routes map[string]*Route `yaml:"routes"`
	mu     sync.RWMutex

	configPath      string
	watchInterval   time.Duration
	reloadCallbacks []func()
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
	maps.Copy(out, c.Routes)
	return out
}

// OnReload registers a callback that is called after each successful config reload.
func (c *Config) OnReload(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reloadCallbacks = append(c.reloadCallbacks, fn)
}

func LoadConfig(ctx context.Context, bootstrapCfg *BootstrapConfig, logger *slog.Logger) (*Config, error) {
	cfg := &Config{
		configPath:    bootstrapCfg.ConfigPath,
		watchInterval: bootstrapCfg.ReloadInterval,
	}
	if err := readConfigFile(ctx, logger, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Watch reloads the config when the file changes. fsnotify is used for
// low-latency change detection; a periodic ticker is run alongside as a
// safety net for filesystems where notifications can be missed (NFS, some
// container overlay setups, atomic-rename editors).
func (c *Config) Watch(ctx context.Context, logger *slog.Logger) error {
	watcher, watcherErr := fsnotify.NewWatcher()
	if watcherErr != nil {
		logger.Warn("fsnotify unavailable, falling back to polling only", "err", watcherErr)
	} else {
		defer watcher.Close()
		// Watch the parent directory: most editors atomically rename a temp
		// file over the target, which delivers as a Create on the directory
		// rather than a Write on the original inode.
		if err := watcher.Add(filepath.Dir(c.configPath)); err != nil {
			logger.Warn("fsnotify add dir failed, polling only", "err", err)
			watcher.Close()
			watcher = nil
		}
	}

	ticker := time.NewTicker(c.watchInterval)
	defer ticker.Stop()

	target := filepath.Clean(c.configPath)
	var lastHash [32]byte
	debounce := time.NewTimer(time.Hour)
	debounce.Stop()
	pending := false

	tryReload := func() {
		data, err := os.ReadFile(c.configPath)
		if err != nil {
			logger.Warn("could not read config file", "err", err)
			return
		}
		hash := sha256.Sum256(data)
		if hash == lastHash {
			return
		}
		if err := readConfigFile(ctx, logger, c); err != nil {
			logger.Warn("could not parse updated config file", "err", err)
			return
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

	var watcherEvents <-chan fsnotify.Event
	var watcherErrors <-chan error
	if watcher != nil {
		watcherEvents = watcher.Events
		watcherErrors = watcher.Errors
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			tryReload()
		case <-debounce.C:
			pending = false
			tryReload()
		case ev, ok := <-watcherEvents:
			if !ok {
				watcherEvents = nil
				continue
			}
			if filepath.Clean(ev.Name) != target {
				continue
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			if !pending {
				pending = true
				debounce.Reset(fsnotifyDebounce)
			}
		case err, ok := <-watcherErrors:
			if !ok {
				watcherErrors = nil
				continue
			}
			logger.Warn("fsnotify error", "err", err)
		}
	}
}

// readConfigFile reads and parses the config file, builds middleware (which
// may spawn long-lived goroutines), and only then takes the write lock to
// swap the routes map. Holding the lock across YAML parsing or middleware
// initialisation would block all in-flight Snapshot() calls for the full
// duration of the parse.
func readConfigFile(ctx context.Context, logger *slog.Logger, config *Config) error {
	data, err := os.ReadFile(config.configPath)
	if err != nil {
		return err
	}

	parsed := &Config{}
	if err := yaml.Unmarshal(data, parsed); err != nil {
		return err
	}

	ctx = middleware.ContextWithLogger(ctx, logger)
	if err := parseRoutesMiddleware(ctx, parsed); err != nil {
		return err
	}

	config.mu.Lock()
	config.Routes = parsed.Routes
	config.mu.Unlock()
	return nil
}

func parseRoutesMiddleware(ctx context.Context, config *Config) error {
	for _, routeConfig := range config.Routes {
		routeConfig.middlewareList = routeConfig.middlewareList[:0]
		for _, entry := range routeConfig.MiddlewareInternalRepr {
			for mwType, mwConfig := range entry {
				mwCtx := middleware.ContextWithMiddlewareType(ctx, string(mwType))
				enc, err := yaml.Marshal(mwConfig)
				if err != nil {
					return fmt.Errorf("failed to marshal middleware config for type %s: %w", mwType, err)
				}
				mw, err := middleware.Build(mwType, mwCtx, enc)
				if err != nil {
					return fmt.Errorf("failed to initialize middleware %s: %w", mwType, err)
				}
				routeConfig.middlewareList = append(routeConfig.middlewareList, mw)
			}
		}
	}

	return nil
}
