package server

import (
	"context"
	"errors"
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

const (
	defaultShutdownTimeout   = 5 * time.Second
	defaultProxyDrainTimeout = 30 * time.Second
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
	adminServer      http.Server
	adminAddr        string
	config           *config.Config
	logger           *slog.Logger
	handler          *muxHandler
	activeProxies    []*proxy.Proxy
	activeMiddleware []middleware.Middleware
	proxiesMu        sync.Mutex
	tlsCert          string
	tlsKey           string

	// lastReloadErr stores the most recent reload failure (or nil) so the
	// admin /healthz endpoint can report a stale-config condition.
	lastReloadErr atomic.Pointer[string]
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

func WithTLSFiles(cert, key string) Option {
	return func(s *Server) {
		s.tlsCert = cert
		s.tlsKey = key
	}
}

// WithAdminAddress binds /healthz and /metrics on a separate listener.
// Empty string disables the admin listener entirely (admin endpoints are
// not registered on the public mux).
func WithAdminAddress(addr string) Option {
	return func(s *Server) { s.adminAddr = addr }
}

func New(cfg *config.Config, opts ...Option) *Server {
	s := &Server{
		config:  cfg,
		logger:  slog.Default(),
		handler: &muxHandler{},
	}
	s.server = http.Server{Handler: s.handler}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	if err := s.applyRoutes(ctx); err != nil {
		s.logger.Error("error while mapping proxy routes", "err", err)
		return err
	}

	// Re-map routes on every successful config reload.
	s.config.OnReload(func() {
		if err := s.applyRoutes(ctx); err != nil {
			msg := err.Error()
			s.lastReloadErr.Store(&msg)
			s.logger.Error("error remapping routes after config reload", "err", err)
			return
		}
		s.lastReloadErr.Store(nil)
	})

	errCh := make(chan error, 2)
	go func() {
		s.logger.Info("server listening", "addr", s.server.Addr, "tls", s.tlsCert != "")
		var err error
		if s.tlsCert != "" {
			err = s.server.ListenAndServeTLS(s.tlsCert, s.tlsKey)
		} else {
			err = s.server.ListenAndServe()
		}
		if err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	if s.adminAddr != "" {
		s.adminServer = http.Server{
			Addr:    s.adminAddr,
			Handler: s.buildAdminMux(),
		}
		go func() {
			s.logger.Info("admin listening", "addr", s.adminAddr)
			if err := s.adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()
	}

	select {
	case err := <-errCh:
		s.logger.Error("error starting server", "err", err)
		return err
	case <-ctx.Done():
	}

	// Parent ctx is already Done here (we just exited <-ctx.Done()); strip
	// cancellation so the shutdown deadline is the only stop signal.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultShutdownTimeout)
	defer cancel()

	mainErr := s.server.Shutdown(shutdownCtx)
	var adminErr error
	if s.adminAddr != "" {
		adminErr = s.adminServer.Shutdown(shutdownCtx)
	}
	if mainErr == nil && adminErr == nil {
		s.logger.Info("server gracefully exited")
	}

	return errors.Join(mainErr, adminErr)
}

// buildAdminMux registers /healthz and /metrics on a dedicated mux so the
// public listener is not exposing operational endpoints.
func (s *Server) buildAdminMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthzHandler)
	mux.Handle("/metrics", promhttp.Handler())
	return mux
}

func (s *Server) healthzHandler(w http.ResponseWriter, _ *http.Request) {
	if msg := s.lastReloadErr.Load(); msg != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("stale config: "))
		_, _ = w.Write([]byte(*msg))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) applyRoutes(ctx context.Context) error {
	router := http.NewServeMux()

	// Snapshot routes outside any lock so proxy creation does not
	// block the config watcher from writing a new config.
	routes := s.config.Snapshot()

	var proxies []*proxy.Proxy
	var allMiddleware []middleware.Middleware
	for routePathPattern, routeConfig := range routes {
		p, err := proxy.New(
			ctx,
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

	drainCtx, drainCancel := context.WithTimeout(ctx, defaultProxyDrainTimeout)
	defer drainCancel()
	for _, p := range oldProxies {
		p.Stop()
		p.Drain(drainCtx)
	}
	for _, mw := range oldMiddleware {
		if err := mw.Close(); err != nil {
			s.logger.Warn("middleware close error", "err", err)
		}
	}

	return nil
}
