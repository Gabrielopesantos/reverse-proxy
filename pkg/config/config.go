package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/balancer"
	"github.com/gabrielopesantos/reverse-proxy/pkg/middleware"
	"gopkg.in/yaml.v3"
)

const (
	DefaultPath = "config.yaml"
)

type Config struct {
	Server `yaml:"server"`
	Routes map[string]*Route `yaml:"routes"`
}

type Server struct {
	Address            string `yaml:"address"`
	ReadTimeoutSeconds uint   `yaml:"read_timeout"`
}

type Route struct {
	Upstreams          []string                    `yaml:"upstreams"`
	LoadBalancerPolicy balancer.LoadBalancerPolicy `yaml:"lb_policy"`
	// FIXME: Currently doesn't seem to be possible to unmarshall directly into a slice of MiddlewareInternalRepr
	MiddlewareInternalRepr map[string]interface{} `yaml:"middleware"`

	middlewareList []middleware.Middleware `yaml:"middleware"`
}

// FIXME: Temporary solution for testing purposes
func (r *Route) Middleware(index int) middleware.Middleware {
	return r.middlewareList[index]
}

func ReadConfig(configPath string) (*Config, error) {
	config := &Config{}
	err := readConfig(config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func WatchConfig(config *Config) error {
	var err error
	for {
		err = readConfig(config)
		if err != nil {
			log.Printf("could not read updated configuration file: %v", err)
		}

		time.Sleep(5 * time.Second)
	}
}

func readConfig(config *Config) error {
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
			switch mwType {
			case "logger":
				enc, err := json.Marshal(mwConfig)
				if err != nil {
					return fmt.Errorf("failed to marshal logger middleware configuration: %w", err)
				}
				loggerConfig := &middleware.Logger{}
				err = json.Unmarshal(enc, loggerConfig)
				if err != nil {
					return fmt.Errorf("failed to unmarshal logger middleware configuration enconding: %w", err)
				}
				loggerConfig.Initialize()
				routeConfig.middlewareList = append(routeConfig.middlewareList, middleware.Middleware(loggerConfig))
			case "rate_limiter":
				enc, err := json.Marshal(mwConfig)
				if err != nil {
					return fmt.Errorf("failed to encode rate limiter middleware configuration: %w", err)
				}
				raterLimiterConfig := &middleware.RateLimiter{}
				err = json.Unmarshal(enc, raterLimiterConfig)
				if err != nil {
					return fmt.Errorf("failed to unmarshal rate limiter middleware configuration enconding: %w", err)
				}
				routeConfig.middlewareList = append(routeConfig.middlewareList, middleware.Middleware(raterLimiterConfig))
			default:
				return fmt.Errorf("unknown middleware type: %s", mwType)
			}
		}
	}

	return nil
}
