package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/config"
	"github.com/gabrielopesantos/reverse-proxy/pkg/proxy"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// muxHandler wraps an http.ServeMux behind an atomic pointer so the
// active router can be swapped without downtime during config hot-reload.
type muxHandler struct {
	mux atomic.Pointer[http.ServeMux]
}

func (a *muxHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mux.Load().ServeHTTP(w, r)
}

type Server struct {
	server        http.Server
	config        *config.Config
	logger        *slog.Logger
	handler       *muxHandler
	activeProxies []*proxy.Proxy
	proxiesMu     sync.Mutex
}

// Option configures a Server at construction time.
type Option func(*Server)

func WithLogger(l *slog.Logger) Option {
	return func(s *Server) { s.logger = l }
}

func New(cfg *config.Config, opts ...Option) *Server {
	s := &Server{
		config:  cfg,
		logger:  slog.Default(),
		handler: &muxHandler{},
	}
	for _, opt := range opts {
		opt(s)
	}
	s.server = http.Server{
		Addr:        cfg.ServerConfig.Address,
		ReadTimeout: time.Duration(cfg.ServerConfig.ReadTimeoutSeconds) * time.Second,
		Handler:     s.handler,
	}
	return s
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	if err := s.applyRoutes(); err != nil {
		s.logger.Error("error while mapping proxy routes", "err", err)
		return err
	}

	// Re-map routes on every successful config reload.
	s.config.OnReload(func() {
		if err := s.applyRoutes(); err != nil {
			s.logger.Error("error remapping routes after config reload", "err", err)
		}
	})

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("server listening", "addr", s.config.ServerConfig.Address)
		if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		s.logger.Error("error starting server", "err", err)
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.server.Shutdown(shutdownCtx)
	if err == nil {
		s.logger.Info("server gracefully exited")
	}
	return err
}

func (s *Server) applyRoutes() error {
	router := http.NewServeMux()

	// Health and metrics endpoints.
	router.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	router.Handle("/metrics", promhttp.Handler())

	// Snapshot routes outside any lock so proxy creation (including concurrent
	// health probes) does not block the config watcher from writing a new config.
	routes := s.config.Snapshot()

	var proxies []*proxy.Proxy
	for routePathPattern, routeConfig := range routes {
		p, err := proxy.New(
			routeConfig.Upstreams,
			proxy.WithLoadBalancerStrategy(routeConfig.LoadBalancerStrategy),
			proxy.WithWeights(routeConfig.Weights),
			proxy.WithHealthCheckInterval(time.Duration(routeConfig.HealthCheckIntervalSeconds)*time.Second),
			proxy.WithHealthCheckPath(routeConfig.HealthCheckPath),
			proxy.WithLogger(s.logger),
		)
		if err != nil {
			return err
		}
		proxies = append(proxies, p)

		handler := http.HandlerFunc(p.ServeHTTP)
		for i := len(routeConfig.Middleware()) - 1; i >= 0; i-- {
			handler = routeConfig.Middleware()[i].Exec(handler.ServeHTTP)
		}

		router.Handle(routePathPattern, handler)
	}

	s.logger.Debug("routes applied", "count", len(routes))
	s.handler.mux.Store(router)

	s.proxiesMu.Lock()
	old := s.activeProxies
	s.activeProxies = proxies
	s.proxiesMu.Unlock()

	for _, p := range old {
		p.Stop()
	}

	return nil
}
