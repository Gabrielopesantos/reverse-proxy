package main

import (
	"log"

	"github.com/gabrielopesantos/reverse-proxy/pkg/config"
)

func exec(c *config.Config) {
	log.Println("Hello world")
}

func main() {
	cfg := config.ReadConfig(config.DefaultPath)

	for {
		switch cfg {
		case nil:
			log.Printf("Failed to read configuration file. Check if file has read permission or is in the following path: %s", config.DefaultPath)
		default:
			exec(cfg)
		}
	}
}
