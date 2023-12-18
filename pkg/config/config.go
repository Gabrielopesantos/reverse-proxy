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
	// NOTE: Why not RWMutex?
	sync.Mutex
}

type Server struct {
	Address            string `yaml:"address"`
	ReadTimeoutSeconds uint   `yaml:"read_timeout"`
}

type Route struct {
	Upstreams          []string                    `yaml:"upstreams"`
	LoadBalancerPolicy balancer.LoadBalancerPolicy `yaml:"lb_policy"`
	// FIXME: Currently doesn't seem to be possible to unmarshall directly into a slice of MiddlewareInternalRepr
	MiddlewareInternalRepr map[middleware.MiddlewareType]interface{} `yaml:"middleware"`

	middlewareList []middleware.Middleware `yaml:"middleware"`
}

// FIXME: Temporary solution for testing purposes
func (r *Route) Middleware() []middleware.Middleware {
	return r.middlewareList
}

func LoadConfig(configPath string) (*Config, error) {
	config := &Config{}
	err := readConfig(config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func (c *Config) Watch(logger *slog.Logger) error {
	for {
		if err := readConfig(c); err != nil {
			logger.Warn(fmt.Sprintf("could not read updated configuration file: %v", err))
		}
		time.Sleep(5 * time.Second)
	}
}

func readConfig(config *Config) error {
	config.Lock()
	defer config.Unlock()

	configFileContent, err := os.ReadFile(DefaultPath)
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

func parseRoutesMiddleware(config *Config) error {
	for _, routeConfig := range config.Routes {
		for mwType, mwConfig := range routeConfig.MiddlewareInternalRepr {
			enc, err := json.Marshal(mwConfig)
			if err != nil {
				return fmt.Errorf("failed to marshal middleware configuration with type: %s: %w", mwType, err)
			}
			switch mwType {

			case middleware.LOGGER:
				loggerConfig := &middleware.LoggerConfig{}
				err = json.Unmarshal(enc, loggerConfig)
				if err != nil {
					return fmt.Errorf("failed to unmarshal logger middleware configuration enconding: %w", err)
				}
				if err := loggerConfig.Init(context.TODO()); err != nil {
					return err
				}
				routeConfig.middlewareList = append(routeConfig.middlewareList, middleware.Middleware(loggerConfig))

			case middleware.RATE_LIMITER:
				raterLimiterConfig := &middleware.RateLimiterConfig{}
				err = json.Unmarshal(enc, raterLimiterConfig)
				if err != nil {
					return fmt.Errorf("failed to unmarshal rate limiter middleware configuration enconding: %w", err)
				}
				raterLimiterConfig.Init(context.TODO())
				routeConfig.middlewareList = append(routeConfig.middlewareList, middleware.Middleware(raterLimiterConfig))

			case middleware.BASIC_AUTH:
				basicAuthConfig := &middleware.BasicAuthConfig{}
				err = json.Unmarshal(enc, basicAuthConfig)
				if err != nil {
					return fmt.Errorf("failed to unmarshal basic auth middleware configuration enconding: %w", err)
				}
				if err := basicAuthConfig.Init(context.TODO()); err != nil {
					return err
				}
				routeConfig.middlewareList = append(routeConfig.middlewareList, middleware.Middleware(basicAuthConfig))

			default:
				return fmt.Errorf("unknown middleware type: %s", mwType)
			}
		}
	}

	return nil
}
