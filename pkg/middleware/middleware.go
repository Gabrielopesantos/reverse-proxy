package middleware

import (
	// "fmt"
	"io"
	"log"
	"net/http"
	"os"
)

type Middleware interface {
	// FIXME: Not sure about `Exec`
	Exec(http.HandlerFunc) http.HandlerFunc
}

type Logger struct {
	// FIXME: Rename
	StreamType string `json:"stream"`
	Mode       string `json:"mode"`

	// FIXME: Is this field going to be needed?
	name   string
	stream io.Writer
	logger *log.Logger
}

// FIXME: Consider rename and accept files
func (l *Logger) Initialize() {
	var stream io.Writer
	switch l.StreamType {
	case "stdout":
		stream = os.Stdout

	case "stderr":
		stream = os.Stderr

	default:
		stream = os.Stdout
	}

	l.logger = log.New(stream, "", log.LstdFlags)
}

func (l *Logger) Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		lrw := NewLoggingResponseWriter(w)
		next.ServeHTTP(lrw, r)

		l.logger.Printf("Path: %s | Method: %s | Status Code: %d", r.URL.Path, r.Method, lrw.statusCode)
	}
}

type RateLimiter struct {
	// WIP
	Rqs uint `json:"rqs"`
}

func (rl *RateLimiter) Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("RateLimiter middleware")
		next.ServeHTTP(w, r)
	}
}
