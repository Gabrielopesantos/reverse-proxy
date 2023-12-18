package middleware

import (
	"context"
	"net/http"
)

type CachingConfig struct{}

func Init(ctx context.Context) error {
	return nil
}

func Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	}
}
