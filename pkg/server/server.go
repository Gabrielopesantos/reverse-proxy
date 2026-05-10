package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/config"
	"github.com/gabrielopesantos/reverse-proxy/pkg/middleware"
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
	server           http.Server
	config           *config.Config
	logger           *slog.Logger
	handler          *muxHandler
	activeProxies    []*proxy.Proxy
	activeMiddleware []middleware.Middleware
	proxiesMu        sync.Mutex
}

// Option configures a Server at construction time.
type Option func(*Server)

func WithLogger(l *slog.Logger) Option {
	return func(s *Server) { s.logger = l }
}

func WithAddress(addr string) Option {
	return func(s *Server) { s.server.Addr = addr }
}

func WithReadTimeout(timeout time.Duration) Option {
	return func(s *Server) { s.server.ReadTimeout = timeout }
}

func New(cfg *config.Config, opts ...Option) *Server {
	s := &Server{
		config:  cfg,
		logger:  slog.Default(),
		handler: &muxHandler{},
	}
	s.server = http.Server{
		Handler: s.handler,
	}
	for _, opt := range opts {
		opt(s)
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
		s.logger.Info("server listening", "addr", s.server.Addr)
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
	var allMiddleware []middleware.Middleware
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

		var handler http.Handler = http.HandlerFunc(p.ServeHTTP)
		mwList := routeConfig.Middleware()
		allMiddleware = append(allMiddleware, mwList...)
		for i := len(mwList) - 1; i >= 0; i-- {
			handler = mwList[i].Exec(handler)
		}

		router.Handle(routePathPattern, handler)
	}

	s.logger.Debug("routes applied", "count", len(routes))
	s.handler.mux.Store(router)

	s.proxiesMu.Lock()
	oldProxies := s.activeProxies
	oldMiddleware := s.activeMiddleware
	s.activeProxies = proxies
	s.activeMiddleware = allMiddleware
	s.proxiesMu.Unlock()

	for _, p := range oldProxies {
		p.Stop()
	}
	for _, mw := range oldMiddleware {
		if err := mw.Close(); err != nil {
			s.logger.Warn("middleware close error", "err", err)
		}
	}

	return nil
}
