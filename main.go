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
	bootstrapCfg, err := config.LoadBootstrap(os.Args[1:], os.Environ())
	if err != nil {
		slog.Error("could not load bootstrap configuration", "err", err)
		os.Exit(1)
	}

	logger, cleanupLogger, err := config.NewBootstrapLogger(bootstrapCfg)
	if err != nil {
		slog.Error("could not initialize logger from bootstrap configuration", "err", err)
		os.Exit(1)
	}
	defer cleanupLogger()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtimeConfig, err := config.LoadConfig(ctx, bootstrapCfg, logger)
	if err != nil {
		logger.Error("could not parse runtime configuration file", "path", bootstrapCfg.ConfigPath, "err", err)
		os.Exit(1)
	}

	go func() {
		if err := runtimeConfig.Watch(ctx, logger); err != nil {
			logger.Error("config watcher exited", "err", err)
			os.Exit(1)
		}
	}()

	srvOpts := []server.Option{
		server.WithLogger(logger),
		server.WithAddress(bootstrapCfg.ListenAddr),
		server.WithAdminAddress(bootstrapCfg.AdminAddr),
		server.WithReadTimeout(bootstrapCfg.ReadTimeout),
	}
	if bootstrapCfg.TLSCertFile != "" {
		srvOpts = append(srvOpts, server.WithTLSFiles(bootstrapCfg.TLSCertFile, bootstrapCfg.TLSKeyFile))
	}
	srv := server.New(runtimeConfig, srvOpts...)

	logger.Info(
		"starting reverse-proxy",
		"config_path", bootstrapCfg.ConfigPath,
		"reload_interval", bootstrapCfg.ReloadInterval.String(),
		"listen_addr", bootstrapCfg.ListenAddr,
		"admin_addr", bootstrapCfg.AdminAddr,
		"tls", bootstrapCfg.TLSCertFile != "",
		"log_level", bootstrapCfg.LogLevel,
		"log_format", bootstrapCfg.LogFormat,
		"log_output", bootstrapCfg.LogOutput,
		"read_timeout", bootstrapCfg.ReadTimeout.String(),
	)

	if err := srv.ListenAndServe(ctx); err != nil {
		logger.Error("failed to start server", "err", err)
		os.Exit(1)
	}
}
