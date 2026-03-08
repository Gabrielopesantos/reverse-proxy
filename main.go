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
	boostrapCfg, err := config.LoadBootstrap(os.Args[1:], os.Environ())
	if err != nil {
		slog.Error("could not load bootstrap configuration", "err", err)
		os.Exit(1)
	}

	logger, cleanupLogger, err := config.NewBootstrapLogger(boostrapCfg)
	if err != nil {
		slog.Error("could not initialize logger from bootstrap configuration", "err", err)
		os.Exit(1)
	}
	defer cleanupLogger()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtimeConfig, err := config.LoadConfig(
		ctx,
		logger,
		boostrapCfg.ConfigPath,
		config.WithWatchInterval(boostrapCfg.ReloadInterval),
	)
	if err != nil {
		logger.Error("could not parse runtime configuration file", "path", boostrapCfg.ConfigPath, "err", err)
		os.Exit(1)
	}

	go func() {
		if err := runtimeConfig.Watch(ctx, logger); err != nil {
			logger.Error("config watcher exited", "err", err)
			os.Exit(1)
		}
	}()

	srv := server.New(
		runtimeConfig,
		server.WithLogger(logger),
		server.WithAddress(boostrapCfg.ListenAddr),
		server.WithReadTimeout(boostrapCfg.ReadTimeout),
	)

	logger.Info(
		"starting reverse-proxy",
		"config_path", boostrapCfg.ConfigPath,
		"reload_interval", boostrapCfg.ReloadInterval.String(),
		"listen_addr", boostrapCfg.ListenAddr,
		"log_level", boostrapCfg.LogLevel,
		"log_format", boostrapCfg.LogFormat,
		"log_output", boostrapCfg.LogOutput,
		"read_timeout", boostrapCfg.ReadTimeout.String(),
	)

	if err := srv.ListenAndServe(ctx); err != nil {
		logger.Error("failed to start server", "err", err)
		os.Exit(1)
	}
}
