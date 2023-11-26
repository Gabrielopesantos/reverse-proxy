package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/gabrielopesantos/reverse-proxy/pkg/config"
	"github.com/gabrielopesantos/reverse-proxy/pkg/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg, err := config.LoadConfig(config.DefaultPath)
	if err != nil {
		logger.Error(fmt.Sprintf("could not parse the provided configuration file: %s", err))
		return
	}
	go cfg.Watch(logger)

	server := server.New(cfg, logger)
	if err = server.ListenAndServe(); err != nil {
		logger.Error(fmt.Sprintf("failed to start server: %s", err))
		return
	}
}
