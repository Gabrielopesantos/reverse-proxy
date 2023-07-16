package middleware

import (
	"net/http"
)

type MiddlewareType string

const (
	LOGGER       MiddlewareType = "logger"
	RATE_LIMITER                = "rate_limiter"
	BASIC_AUTH                  = "basic_auth"
)

type Middleware interface {
	// FIXME: Not sure about `Exec`
	Exec(http.HandlerFunc) http.HandlerFunc
}
