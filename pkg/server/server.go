package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/config"
	"github.com/gabrielopesantos/reverse-proxy/pkg/proxy"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// atomicMuxHandler wraps an http.ServeMux behind an atomic pointer so the
// active router can be swapped without downtime during config hot-reload.
type atomicMuxHandler struct {
	mux atomic.Pointer[http.ServeMux]
}

func (a *atomicMuxHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mux.Load().ServeHTTP(w, r)
}

type Server struct {
	server        http.Server
	config        *config.Config
	logger        *slog.Logger
	handler       *atomicMuxHandler
	activeProxies []*proxy.Proxy
	proxiesMu     sync.Mutex
}

func New(config *config.Config, logger *slog.Logger) *Server {
	s := &Server{
		config:  config,
		logger:  logger,
		handler: &atomicMuxHandler{},
	}
	s.server = http.Server{
		Addr:        config.Server.Address,
		ReadTimeout: time.Duration(config.Server.ReadTimeoutSeconds) * time.Second,
		Handler:     s.handler,
	}
	return s
}

func (s *Server) ListenAndServe() error {
	if err := s.reloadRoutes(); err != nil {
		s.logger.Error(fmt.Sprintf("error while mapping proxy routes: %s", err))
		return err
	}

	// Re-map routes on every successful config reload.
	s.config.OnReload(func() {
		if err := s.reloadRoutes(); err != nil {
			s.logger.Error(fmt.Sprintf("error remapping routes after config reload: %s", err))
		}
	})

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	go func(quitChannel chan os.Signal) {
		s.logger.Info(fmt.Sprintf("Server listening on address: %s", s.config.Server.Address))
		if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
			s.logger.Error(fmt.Sprintf("error starting server: %s", err))
			quitChannel <- os.Interrupt
		}
	}(quit)

	<-quit
	ctx, shutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdown()

	err := s.server.Shutdown(ctx)
	if err == nil {
		s.logger.Info("Server gracefully exited")
	}
	return err
}

func (s *Server) reloadRoutes() error {
	router := http.NewServeMux()

	// Health and metrics endpoints.
	router.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	router.Handle("/metrics", promhttp.Handler())

	s.config.RLock()
	defer s.config.RUnlock()

	var newProxies []*proxy.Proxy
	for routePathPattern, routeConfig := range s.config.Routes {
		rProxy, err := proxy.New(routeConfig, s.logger)
		if err != nil {
			return err
		}
		newProxies = append(newProxies, rProxy)

		handler := http.HandlerFunc(rProxy.ServeHTTP)
		for i := len(routeConfig.Middleware()) - 1; i >= 0; i-- {
			handler = routeConfig.Middleware()[i].Exec(handler.ServeHTTP)
		}

		router.Handle(routePathPattern, handler)
		s.logger.Info(fmt.Sprintf("Handler set for route %s", routePathPattern))
	}

	s.handler.mux.Store(router)

	s.proxiesMu.Lock()
	old := s.activeProxies
	s.activeProxies = newProxies
	s.proxiesMu.Unlock()

	for _, p := range old {
		p.Stop()
	}

	return nil
}
