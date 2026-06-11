package server

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/config"
)

const (
	accessLogBufferBytes   = 64 * 1024
	accessLogFlushInterval = 200 * time.Millisecond
	accessLogFileMode      = 0o666
)

// NewAccessLog returns an http.Handler wrapper that logs every proxied request,
// a cleanup function that flushes and closes the underlying writer, and any
// initialisation error.
func NewAccessLog(output config.LogOutput, format config.AccessLogFormat) (func(http.Handler) http.Handler, func(), error) {
	writer, cleanup, err := openAccessLogWriter(output)
	if err != nil {
		return nil, cleanup, err
	}

	mu := &sync.Mutex{}
	bw := bufio.NewWriterSize(writer, accessLogBufferBytes)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(accessLogFlushInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				mu.Lock()
				_ = bw.Flush()
				mu.Unlock()
			case <-stop:
				mu.Lock()
				_ = bw.Flush()
				mu.Unlock()
				return
			}
		}
	}()

	fullCleanup := func() {
		close(stop)
		<-done
		cleanup()
	}

	switch format {
	case config.AccessLogFormatCombined:
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				start := time.Now()
				tw := &accessTrackingWriter{ResponseWriter: w, status: http.StatusOK}
				next.ServeHTTP(tw, r)
				writeCombinedLine(bw, mu, r, tw, start)
			})
		}, fullCleanup, nil
	default: // config.AccessLogFormatJSON
		lw := &accessLockedWriter{w: bw, mu: mu}
		log := slog.New(slog.NewJSONHandler(lw, nil))
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				start := time.Now()
				tw := &accessTrackingWriter{ResponseWriter: w, status: http.StatusOK}
				next.ServeHTTP(tw, r)
				log.Info("request", "remote_addr", r.RemoteAddr, "method", r.Method,
					"path", r.URL.RequestURI(), "proto", r.Proto, "status", tw.status,
					"bytes_written", tw.bytes, "duration_ms", time.Since(start).Milliseconds(),
					"user_agent", r.Header.Get("User-Agent"), "referer", r.Header.Get("Referer"),
				)
			})
		}, fullCleanup, nil
	}
}

func writeCombinedLine(bw *bufio.Writer, mu *sync.Mutex, r *http.Request, tw *accessTrackingWriter, t time.Time) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "-"
	}
	userAgent := r.Header.Get("User-Agent")
	if userAgent == "" {
		userAgent = "-"
	}
	bytesStr := "-"
	if tw.bytes > 0 {
		bytesStr = strconv.FormatInt(tw.bytes, 10)
	}
	mu.Lock()
	fmt.Fprintf(bw, "%s - - [%s] \"%s %s %s\" %d %s \"%s\" \"%s\"\n",
		host, t.Format("02/Jan/2006:15:04:05 -0700"), r.Method,
		r.URL.RequestURI(), r.Proto, tw.status, bytesStr, referer,
		userAgent,
	)
	mu.Unlock()
}

// accessTrackingWriter records the response status code and body bytes.
type accessTrackingWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *accessTrackingWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *accessTrackingWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

// accessLockedWriter serialises slog handler writes with the background flusher.
type accessLockedWriter struct {
	w  *bufio.Writer
	mu *sync.Mutex
}

func (l *accessLockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func openAccessLogWriter(output config.LogOutput) (io.Writer, func(), error) {
	switch output {
	case config.LogOutputStdout:
		return os.Stdout, func() {}, nil
	case config.LogOutputStderr:
		return os.Stderr, func() {}, nil
	default:
		path := string(output)
		if dir := filepath.Dir(path); dir != "." {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return nil, func() {}, fmt.Errorf("failed to create access log directory %q: %w", dir, err)
			}
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, accessLogFileMode)
		if err != nil {
			return nil, func() {}, fmt.Errorf("failed to open access log %q: %w", path, err)
		}
		return f, func() { _ = f.Close() }, nil
	}
}
