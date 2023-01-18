package main

import (
	"log"

	"github.com/gabrielopesantos/reverse-proxy/pkg/config"
	"github.com/gabrielopesantos/reverse-proxy/pkg/server"
)

func main() {
	cfg, err := config.ReadConfig(config.DefaultPath)
	if err != nil {
		log.Fatalf("Failed to read configuration file: %s", err)
	}

	// ?
	go config.WatchConfig(cfg)

	server := server.NewServer(cfg)
	server.Run()
}
