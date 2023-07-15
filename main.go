package main

import (
	"log"

	"github.com/gabrielopesantos/reverse-proxy/pkg/config"
	"github.com/gabrielopesantos/reverse-proxy/pkg/server"
)

func main() {
	cfg, err := config.ReadConfig(config.DefaultPath)
	if err != nil {
		log.Fatalf("failed to parse the configuration file: %s", err)
	}

	go config.WatchConfig(cfg)

	server := server.New(cfg)
	if err = server.Start(); err != nil {
		log.Fatalf("failed to start server: %s", err)
	}
}
