package middleware

import (
	"context"
	"net/http"
)

type MiddlewareType string

const (
	LOGGER      MiddlewareType = "logger"
	RATE_LIMITER MiddlewareType = "rate_limiter"
	BASIC_AUTH  MiddlewareType = "basic_auth"
	CACHE_CONTROL MiddlewareType = "cache_control"
	PROMETHEUS  MiddlewareType = "prometheus"
)

type Middleware interface {
	Init(context.Context) error
	Exec(http.HandlerFunc) http.HandlerFunc
}
