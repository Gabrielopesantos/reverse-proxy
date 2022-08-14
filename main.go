package main

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	configFilePath = "config.yaml"
)

type Route struct {
	Destination string   `yaml:"destination"`
	Middleware  []string `yaml:"middleware"`
}

type Config struct {
	Routes map[string]Route
}

func readConfig() (*Config, error) {
	// check if file exists and is readable
	configFileContent, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, err
	}

	configRoutes := make(map[string]Route)
	err = yaml.Unmarshal(configFileContent, configRoutes)
	if err != nil {
		return nil, err
	}
	config := &Config{Routes: configRoutes}

	return config, nil
}

func main() {
	config, err := readConfig()
	if err != nil {
		log.Fatal("failed to read file config")
	}

	log.Printf("%+v", config.Routes)
}
