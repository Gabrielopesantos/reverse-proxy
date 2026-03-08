package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gabrielopesantos/reverse-proxy/pkg/config"
	"github.com/gabrielopesantos/reverse-proxy/pkg/server"
)

func main() {
	// TODO: Make configurable...
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// TODO: Make configurable...config file path and watch interval
	cfg, err := config.LoadConfig(config.DefaultPath)
	if err != nil {
		logger.Error("could not parse the provided configuration file", "err", err)
		os.Exit(1)
	}

	go func() {
		if err := cfg.Watch(ctx, logger); err != nil {
			logger.Error("config watcher exited", "err", err)
			os.Exit(1)
		}
	}()

	srv := server.New(cfg, server.WithLogger(logger))
	if err = srv.ListenAndServe(ctx); err != nil {
		logger.Error("failed to start server", "err", err)
		os.Exit(1)
	}
}
