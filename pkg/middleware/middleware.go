package middleware

import (
	"context"
	"net/http"
)

type MiddlewareType string

const (
	LOGGER        MiddlewareType = "logger"
	RATE_LIMITER                 = "rate_limiter"
	BASIC_AUTH                   = "basic_auth"
	CACHE_CONTROL                = "cache_control"
)

type Middleware interface {
	Init(context.Context) error
	Exec(http.HandlerFunc) http.HandlerFunc
}
