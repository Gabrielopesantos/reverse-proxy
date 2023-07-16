package server

import (
	"context"
	"log"
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
}

func New(config *config.Config) *Server {
	return &Server{
		server: http.Server{
			Addr:        config.Server.Address,
			ReadTimeout: time.Duration(config.Server.ReadTimeoutSeconds) * time.Second,
		},
		config: config,
	}
}

func (s *Server) Start() error {
	go func() {
		log.Printf("Server listening on address: %s", s.config.Server.Address)
		if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("error starting server: %s", err)
		}
	}()

	err := s.mapProxyHandlers()
	if err != nil {
		return err
	}
	// TODO: Healthcheck endpoint for server

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	<-quit

	ctx, shutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdown()

	log.Println("Server exited properly")
	return s.server.Shutdown(ctx)
}

func (s *Server) mapProxyHandlers() error {
	router := http.NewServeMux()
	for pattern, routeConfig := range s.config.Routes {
		rProxy, err := proxy.New(routeConfig)
		if err != nil {
			return err
		}

		// Set middleware
		handler := http.HandlerFunc(rProxy.ServeHTTP)
		for i := len(routeConfig.Middleware()) - 1; i >= 0; i-- {
			handler = routeConfig.Middleware()[i].Exec(handler.ServeHTTP)
		}

		router.Handle(pattern, handler)
		log.Printf("Handler set for route %s", pattern)
	}
	s.server.Handler = router

	return nil
}
