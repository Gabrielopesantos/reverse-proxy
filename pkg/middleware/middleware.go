package middleware

import (
	"context"
	"log/slog"
	"net/http"
)

type MiddlewareType string

const (
	TypeLogger       MiddlewareType = "logger"
	TypeRateLimiter  MiddlewareType = "rate_limiter"
	TypeBasicAuth    MiddlewareType = "basic_auth"
	TypeCacheControl MiddlewareType = "cache_control"
	TypePrometheus   MiddlewareType = "prometheus"
	TypeWAF          MiddlewareType = "waf"
	TypeHeaders      MiddlewareType = "headers"
	TypeRewrite      MiddlewareType = "rewrite"
)

// Phase is a middleware's slot in the fixed outermost->innermost execution
// order. Lower phases run first on the way in and last on the way out. Phase
// boundaries carry the correctness guarantees (observers wrap everything, cheap
// rejections precede expensive auth, shaping precedes the cache so the cache
// key reflects the rewritten path, and the cache is innermost so a hit
// short-circuits the upstream). Within a phase, config order is preserved and
// repeated entries are allowed.
//
// Each middleware declares its phase at registration (see Register), so custom
// middleware slot into the chain without editing any central list.
type Phase int

const (
	PhaseObserve      Phase = iota // logger, prometheus
	PhaseGuard                     // rate_limiter, waf
	PhaseAuthenticate              // basic_auth
	PhaseShape                     // headers, rewrite
	PhaseCache                     // cache_control

	// phaseCount is the rank assigned to unregistered types so they sort after
	// every known phase; such types fail later in Build with a clear error.
	phaseCount
)

func (p Phase) String() string {
	switch p {
	case PhaseObserve:
		return "observe"
	case PhaseGuard:
		return "guard"
	case PhaseAuthenticate:
		return "authenticate"
	case PhaseShape:
		return "shape"
	case PhaseCache:
		return "cache"
	default:
		return "unknown"
	}
}

type Middleware interface {
	Init(context.Context) error
	Exec(http.Handler) http.Handler
	Close() error
}

type loggerCtxKey struct{}
type middlewareTypeCtxKey struct{}

// ContextWithLogger returns a copy of ctx carrying l.
func ContextWithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerCtxKey{}, l)
}

// ContextWithMiddlewareType returns a copy of ctx carrying the middleware type name.
func ContextWithMiddlewareType(ctx context.Context, mwType string) context.Context {
	return context.WithValue(ctx, middlewareTypeCtxKey{}, mwType)
}

// LoggerFromContext retrieves the logger stored by ContextWithLogger and, if a
// middleware type was stored via ContextWithMiddlewareType, automatically embeds
// it as a "middleware" attribute.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	l := slog.Default()
	if stored, ok := ctx.Value(loggerCtxKey{}).(*slog.Logger); ok {
		l = stored
	}
	if mwType, ok := ctx.Value(middlewareTypeCtxKey{}).(string); ok && mwType != "" {
		l = l.With("middleware", mwType)
	}
	return l
}
