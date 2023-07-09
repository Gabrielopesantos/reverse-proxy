package middleware

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"log"
	"net/http"
	"os"
)

type MiddlewareConfig struct {
	// FIXME: Make this private
	Middlewares []Middleware
}

func (mc *MiddlewareConfig) UnmarshalYAML(value *yaml.Node) error {
	middlewares := make([]Middleware, 0)

	for i := 0; i < len(value.Content); i += 2 {
		middlewareName := value.Content[i].Value
		middlewareConfig := value.Content[i+1]
		switch middlewareName {
		case "logger":
			logger := &Logger{}
			err := middlewareConfig.Decode(logger)
			if err != nil {
				return fmt.Errorf("error parsing logger configuration: %w", err)
			}
			middlewares = append(middlewares, logger)
		}
	}

	mc.Middlewares = middlewares

	return nil
}

type Middleware interface {
	// FIXME: Not sure about `Exec`
	Exec(http.HandlerFunc) http.HandlerFunc
}

// TODO: Add logging type (json/human readable), etc
type Logger struct {
	Stream io.Writer `yaml:"stream"`
	Mode   string    `yaml:"mode"`
	name   string
	logger *log.Logger
}

func NewLogger(stream io.Writer) *Logger {
	if stream == nil {
		stream = os.Stdout
	}
	return &Logger{
		name:   "Logger",
		Stream: stream,
		logger: log.New(stream, "", log.LstdFlags),
	}
}

func (l *Logger) Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		lrw := NewLoggingResponseWriter(w)
		next.ServeHTTP(lrw, r)

		l.logger.Printf("Path: %s | Method: %s | Status Code: %d", r.URL.Path, r.Method, lrw.statusCode)
	}
}

// func (l *Logger) UnmarshalYAML(value *yaml.Node) error {
// 	log.Println("LKAJDKLASJDKLASJDKLASJDKLASJDKLASJDKLASJDKLASJDKLASJDKLSAJDL")
//
// 	return nil
// }
