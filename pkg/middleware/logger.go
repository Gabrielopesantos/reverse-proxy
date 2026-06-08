package middleware

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

func init() {
	RegisterYAML(TypeLogger, func() *Logger { return &Logger{} })
}

type StreamType string

const (
	StreamTypeStdout StreamType = "stdout"
	StreamTypeStderr StreamType = "stderr"
	// If none of the above, has to be a file path
)

type LoggerMode string

const (
	LoggerModeJSON LoggerMode = "json"
	LoggerModeText LoggerMode = "text"
)

const (
	defaultLogBufferBytes   = 64 * 1024
	defaultLogFlushInterval = 200 * time.Millisecond
)

type Logger struct {
	Stream StreamType `yaml:"stream"`
	Mode   LoggerMode `yaml:"mode"`
	// BufferBytes sets the size of the buffered writer wrapping the underlying
	// stream. 0 -> 64 KiB.
	BufferBytes int `yaml:"buffer_bytes,omitempty"`
	// FlushIntervalMs forces a flush of the buffered writer at this cadence
	// so logs aren't held forever during quiet periods. 0 -> 200 ms.
	FlushIntervalMs int `yaml:"flush_interval_ms,omitempty"`

	accessLog *slog.Logger
	logger    *slog.Logger
	file      *os.File
	bw        *bufio.Writer
	bwMu      sync.Mutex // serializes Flush/Write conflict between handler and ticker
	stopFlush chan struct{}
	flushDone chan struct{}
}

func (l *Logger) Init(ctx context.Context) error {
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
		file, err := os.OpenFile(string(l.Stream), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o666)
		if err != nil {
			return err
		}
		l.file = file
		writer = file
	}

	bufBytes := l.BufferBytes
	if bufBytes <= 0 {
		bufBytes = defaultLogBufferBytes
	}
	l.bw = bufio.NewWriterSize(writer, bufBytes)

	// lockedWriter serialises slog handler writes with the periodic flusher.
	lw := &lockedWriter{w: l.bw, mu: &l.bwMu}

	var handler slog.Handler
	switch l.Mode {
	case LoggerModeJSON:
		handler = slog.NewJSONHandler(lw, nil)
	case LoggerModeText:
		handler = slog.NewTextHandler(lw, nil)
	default:
		return fmt.Errorf("invalid logger mode provided, '%s'", l.Mode)
	}

	l.accessLog = slog.New(handler)

	flushEvery := time.Duration(l.FlushIntervalMs) * time.Millisecond
	if flushEvery <= 0 {
		flushEvery = defaultLogFlushInterval
	}
	l.stopFlush = make(chan struct{})
	l.flushDone = make(chan struct{})
	go l.flushLoop(flushEvery)

	return nil
}

func (l *Logger) flushLoop(every time.Duration) {
	defer close(l.flushDone)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			l.bwMu.Lock()
			_ = l.bw.Flush()
			l.bwMu.Unlock()
		case <-l.stopFlush:
			l.bwMu.Lock()
			_ = l.bw.Flush()
			l.bwMu.Unlock()
			return
		}
	}
}

func (l *Logger) Exec(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lrw := NewLoggingResponseWriter(w)
		next.ServeHTTP(lrw, r)
		l.accessLog.Info("request", "path", r.URL.Path, "method", r.Method, "status_code", lrw.statusCode)
	})
}

func (l *Logger) Close() error {
	if l.stopFlush != nil {
		close(l.stopFlush)
		<-l.flushDone
		l.stopFlush = nil
	}
	if l.bw != nil {
		l.bwMu.Lock()
		_ = l.bw.Flush()
		l.bwMu.Unlock()
	}
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// lockedWriter funnels writes through bwMu so the background flusher and slog
// handler writes never race on the underlying bufio.Writer.
type lockedWriter struct {
	w  *bufio.Writer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
