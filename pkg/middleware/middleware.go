package middleware

import (
	"log"
	"net/http"
)

func Logger(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		lrw := NewLoggingResponseWriter(w)
		next.ServeHTTP(lrw, r)

		log.Printf("Path: %s | Method: %s | Status Code: %d", r.URL.Path, r.Method, lrw.statusCode)
	}
}
