package middleware

import (
	"context"
	"log/slog"
	"net/http"
)

type MiddlewareType string

const (
	LOGGER        MiddlewareType = "logger"
	RATE_LIMITER  MiddlewareType = "rate_limiter"
	BASIC_AUTH    MiddlewareType = "basic_auth"
	CACHE_CONTROL MiddlewareType = "cache_control"
	PROMETHEUS    MiddlewareType = "prometheus"
	WAF           MiddlewareType = "waf"
)

type Middleware interface {
	Init(context.Context) error
	Exec(http.HandlerFunc) http.HandlerFunc
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
