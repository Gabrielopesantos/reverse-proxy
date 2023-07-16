package middleware

import (
	"io"
	"log"
	"net/http"
	"os"
)

type LoggerConfig struct {
	// FIXME: Rename
	StreamType string `json:"stream"`
	Mode       string `json:"mode"`

	// FIXME: Is this field going to be needed?
	name   string
	stream io.Writer
	logger *log.Logger
}

// FIXME: Consider rename and accept files
func (l *LoggerConfig) Initialize() {
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

func (l *LoggerConfig) Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		lrw := NewLoggingResponseWriter(w)
		next.ServeHTTP(lrw, r)

		l.logger.Printf("Path: %s | Method: %s | Status Code: %d", r.URL.Path, r.Method, lrw.statusCode)
	}
}
