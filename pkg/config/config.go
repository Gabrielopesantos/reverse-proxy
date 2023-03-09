package config

import (
	"log"
	"os"
	"time"

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
	Upstreams          []string `yaml:"upstreams"`
	LoadBalancerPolicy string   `yaml:"lb_policy"`
	Middleware         []string `yaml:"middleware"`
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
			log.Printf("Could not read updated configuration file: %v", err)
		}

		time.Sleep(5 * time.Second)
	}
}

func readConfig(config *Config) error {
	configFileContent, err := os.ReadFile(DefaultPath)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(configFileContent, config)
	if err != nil {
		return err
	}

	return nil
}
