package middleware

import (
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
	Stream StreamType `json:"stream"`
	Mode   LoggerMode `json:"mode"`
	logger *slog.Logger
}

func (l *LoggerConfig) Init() error {
	var writer io.Writer
	switch l.Stream {
	case StreamTypeStdout:
		writer = os.Stdout
	case StreamTypeStderr:
		writer = os.Stderr
	default:
		file, err := os.OpenFile(string(l.Stream), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return err
		}
		// NOTE: Has to be closed when the program is finished;
		// defer file.Close()
		// NOTE: Multiwriter?
		// wrt := io.MultiWriter(os.Stdout, file)
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

	l.logger = slog.New(handler)
	return nil
}

func (l *LoggerConfig) Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		lrw := NewLoggingResponseWriter(w)
		next.ServeHTTP(lrw, r)

		l.logger.Info(fmt.Sprintf("Path: %s | Method: %s | Status Code: %d", r.URL.Path, r.Method, lrw.statusCode))
	}
}
