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
