package config

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/middleware"
)

// middlewareYAML lists middleware across phases in a non-canonical order,
// with two rewrite blocks straddling a headers block (all in the "shape"
// phase) to exercise intra-phase ordering and repeated entries.
const middlewareYAML = `
routes:
  /r:
    upstreams:
      - "http://localhost:8081"
    middleware:
      - cache_control:
          duration: 1s
      - rewrite:
          rules:
            - match: "^/a/(.*)"
              replace: "/$1"
      - headers:
          request:
            set:
              X-H: "1"
      - prometheus:
          route: /r
      - rewrite:
          rules:
            - match: "^/b/(.*)"
              replace: "/$1"
      - rate_limiter:
          max_requests: 10
          window_size_seconds: 60
`

// TestMiddlewareOrderedByPhase verifies the built chain is reordered by phase
// (observe -> guard -> shape -> cache here) while preserving the config list
// order within the shape phase, including the two rewrite blocks.
func TestMiddlewareOrderedByPhase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(middlewareYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(
		context.Background(),
		&BootstrapConfig{ConfigPath: path, ReloadInterval: time.Second},
		slog.New(slog.NewTextHandler(os.Stderr, nil)),
	)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	route, ok := cfg.Routes["/r"]
	if !ok {
		t.Fatal("route /r not found")
	}
	mws := route.Middleware()
	t.Cleanup(func() {
		for _, mw := range mws {
			_ = mw.Close() // stop rate_limiter goroutines
		}
	})

	wantTypes := []middleware.MiddlewareType{
		middleware.TypePrometheus,   // observe
		middleware.TypeRateLimiter,  // guard
		middleware.TypeRewrite,      // shape: /a (first in list)
		middleware.TypeHeaders,      // shape: between the rewrites
		middleware.TypeRewrite,      // shape: /b (last in list)
		middleware.TypeCacheControl, // cache
	}
	if len(mws) != len(wantTypes) {
		t.Fatalf("got %d middleware, want %d", len(mws), len(wantTypes))
	}
	for i, want := range wantTypes {
		if got := typeOf(mws[i]); got != want {
			t.Errorf("position %d: got %s, want %s", i, got, want)
		}
	}

	// Intra-phase ordering: the /a rewrite must precede the /b rewrite.
	first := mws[2].(*middleware.Rewrite)
	second := mws[4].(*middleware.Rewrite)
	if first.Rules[0].Match != "^/a/(.*)" {
		t.Errorf("first rewrite match = %q, want ^/a/(.*)", first.Rules[0].Match)
	}
	if second.Rules[0].Match != "^/b/(.*)" {
		t.Errorf("second rewrite match = %q, want ^/b/(.*)", second.Rules[0].Match)
	}
}

func typeOf(mw middleware.Middleware) middleware.MiddlewareType {
	switch mw.(type) {
	case *middleware.Prometheus:
		return middleware.TypePrometheus
	case *middleware.RateLimiter:
		return middleware.TypeRateLimiter
	case *middleware.WAFMiddleware:
		return middleware.TypeWAF
	case *middleware.BasicAuth:
		return middleware.TypeBasicAuth
	case *middleware.Headers:
		return middleware.TypeHeaders
	case *middleware.Rewrite:
		return middleware.TypeRewrite
	case *middleware.CacheControl:
		return middleware.TypeCacheControl
	default:
		return middleware.MiddlewareType("unknown")
	}
}
