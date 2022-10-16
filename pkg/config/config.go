package config

import (
	"gopkg.in/yaml.v3"
	"os"
	"time"
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

func ReadConfig(configPath string) *Config {
	config := &Config{}
	go func() {
		_ = watchConfig(config)
	}()

	return config
}

func watchConfig(config *Config) error {
	for {
		err := readConfig(config)
		if err != nil {
			return err
		}

		time.Sleep(3 * time.Second)
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
