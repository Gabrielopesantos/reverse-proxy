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
	//"github.com/gabrielopesantos/reverse-proxy/pkg/middleware"
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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	<-quit

	ctx, shutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdown()

	log.Println("Server Exited Properly")
	return s.server.Shutdown(ctx)
}

func (s *Server) mapProxyHandlers() error {
	router := http.NewServeMux()
	for pattern, routeConfig := range s.config.Routes {
		// FIXME: proxy name
		proxy, err := proxy.New(routeConfig)
		if err != nil {
			return err
		}
		router.Handle(pattern, proxy)

		log.Printf("Handler set for route %s", pattern)
	}
	s.server.Handler = router

	return nil
}
