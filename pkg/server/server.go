package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/config"
	"github.com/gabrielopesantos/reverse-proxy/pkg/proxy"
)

type Server struct {
	server http.Server
	config *config.Config
	logger *slog.Logger
}

func New(config *config.Config, logger *slog.Logger) *Server {
	return &Server{
		server: http.Server{
			Addr:        config.Server.Address,
			ReadTimeout: time.Duration(config.Server.ReadTimeoutSeconds) * time.Second,
		},
		config: config,
		logger: logger,
	}
}

func (s *Server) ListenAndServe() error {
	err := s.mapProxyRoutes()
	if err != nil {
		s.logger.Error(fmt.Sprintf("error while mapping proxy routes: %s", err))
		return err
	}
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	go func(quitChannel chan os.Signal) {
		s.logger.Info(fmt.Sprintf("Server listening on address: %s", s.config.Server.Address))
		if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
			s.logger.Error(fmt.Sprintf("error starting server: %s", err))
			quitChannel <- os.Interrupt
		}
	}(quit)
	// TODO: Healthcheck endpoint for server

	<-quit
	ctx, shutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdown()

	s.logger.Info("Server gracefully exited properly")
	return s.server.Shutdown(ctx)
}

func (s *Server) mapProxyRoutes() error {
	router := http.NewServeMux()
	for routePathPattern, routeConfig := range s.config.Routes {
		rProxy, err := proxy.New(routeConfig)
		if err != nil {
			return err
		}

		// Set middleware
		handler := http.HandlerFunc(rProxy.ServeHTTP)
		for i := len(routeConfig.Middleware()) - 1; i >= 0; i-- {
			handler = routeConfig.Middleware()[i].Exec(handler.ServeHTTP)
		}

		router.Handle(routePathPattern, handler)
		s.logger.Info(fmt.Sprintf("Handler set for route %s", routePathPattern))
	}
	s.server.Handler = router

	return nil
}
