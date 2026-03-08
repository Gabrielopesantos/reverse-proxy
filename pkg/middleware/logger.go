package middleware

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
)

type StreamType string

const (
	StreamTypeStdout StreamType = "stdout"
	StreamTypeStderr            = "stderr"
	// If none of the above, has to be a file path
)

type LoggerMode string

const (
	LoggerModeJSON LoggerMode = "json"
	LoggerModeText            = "text"
)

type LoggerConfig struct {
	Stream    StreamType `yaml:"stream"`
	Mode      LoggerMode `yaml:"mode"`
	accessLog *slog.Logger
	logger    *slog.Logger
	file      *os.File
}

func (l *LoggerConfig) Init(ctx context.Context) error {
	l.logger = LoggerFromContext(ctx)

	var writer io.Writer
	switch l.Stream {
	case StreamTypeStdout:
		writer = os.Stdout
	case StreamTypeStderr:
		writer = os.Stderr
	default:
		if l.file != nil {
			_ = l.file.Close()
		}
		file, err := os.OpenFile(string(l.Stream), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return err
		}
		l.file = file
		writer = file
	}

	var handler slog.Handler
	switch l.Mode {
	case LoggerModeJSON:
		handler = slog.NewJSONHandler(writer, nil)
	case LoggerModeText:
		handler = slog.NewTextHandler(writer, nil)
	default:
		return fmt.Errorf("invalid logger mode provided, '%s'", l.Mode)
	}

	l.accessLog = slog.New(handler)
	return nil
}

func (l *LoggerConfig) Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		lrw := NewLoggingResponseWriter(w)
		next.ServeHTTP(lrw, r)

		l.accessLog.Info("request", "path", r.URL.Path, "method", r.Method, "status_code", lrw.statusCode)
	}
}
