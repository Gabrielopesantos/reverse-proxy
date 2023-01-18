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

type Route struct {
	Destination string   `yaml:"destination"`
	Middleware  []string `yaml:"middleware"`
}

type Config struct {
	Routes map[string]Route
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
			log.Printf("Could not read updated Configuration file: %v", err)
		}

		time.Sleep(5 * time.Second)
	}
}
func readConfig(config *Config) error {
	configFileContent, err := os.ReadFile(DefaultPath)
	if err != nil {
		return err
	}

	configRoutes := make(map[string]Route)
	err = yaml.Unmarshal(configFileContent, configRoutes)
	if err != nil {
		return err
	}

	config.Routes = configRoutes

	return nil
}
