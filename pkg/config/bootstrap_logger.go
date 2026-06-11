package config

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const logFileMode = 0o666

// NewBootstrapLogger creates a process-wide logger from bootstrap settings.
//
// It returns:
//   - a configured logger
//   - a cleanup function (closes file outputs when used; no-op otherwise)
//   - an error if configuration is invalid or output cannot be opened
func NewBootstrapLogger(cfg *BootstrapConfig) (*slog.Logger, func(), error) {
	writer, cleanup, err := OpenLogWriter(cfg.LogOutput)
	if err != nil {
		return nil, func() {}, err
	}

	opts := &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}

	var handler slog.Handler
	switch cfg.LogFormat {
	case LogFormatText:
		w := io.Writer(writer)
		if isTTY(writer) {
			w = colorizingWriter{writer}
		}
		handler = slog.NewTextHandler(w, opts)
	case LogFormatJSON:
		handler = slog.NewJSONHandler(writer, opts)
	default:
		cleanup()
		return nil, func() {}, fmt.Errorf("unsupported log format %q (expected text|json)", cfg.LogFormat)
	}

	return slog.New(handler), cleanup, nil
}

func isTTY(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	fileInfo, err := file.Stat()
	return err == nil && (fileInfo.Mode()&os.ModeCharDevice) != 0
}

// OpenLogWriter resolves a LogOutput to a writer. LogOutputStdout and
// LogOutputStderr map to the corresponding OS streams; any other value is
// treated as a file path (parent directory is created automatically).
// The returned cleanup function closes file outputs; it is a no-op for streams.
func OpenLogWriter(output LogOutput) (io.Writer, func(), error) {
	switch output {
	case LogOutputStdout:
		return os.Stdout, func() {}, nil
	case LogOutputStderr:
		return os.Stderr, func() {}, nil
	default:
		path := string(output)
		if dir := filepath.Dir(path); dir != "." {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return nil, func() {}, fmt.Errorf("failed to create log directory %q: %w", dir, err)
			}
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, logFileMode)
		if err != nil {
			return nil, func() {}, fmt.Errorf("failed to open log output %q: %w", path, err)
		}
		return f, func() { _ = f.Close() }, nil
	}
}

// colorizingWriter wraps an io.Writer and colorizes the level= field in each
// slog text-format line. The underlying slog.TextHandler calls Write once per
// record with the full formatted line, so this is the right interception point.
type colorizingWriter struct{ w io.Writer }

func (cw colorizingWriter) Write(p []byte) (int, error) {
	return cw.w.Write([]byte(colorizeLevelField(string(p))))
}

// colorizeLevelField finds the "level=VALUE" token in a slog text line and
// wraps VALUE in the appropriate ANSI escape sequence. It handles offset levels
// like "INFO+2" by prefix-matching the base level name.
func colorizeLevelField(line string) string {
	const prefix = "level="
	idx := strings.Index(line, prefix)
	if idx < 0 {
		return line
	}
	start := idx + len(prefix)
	end := strings.IndexByte(line[start:], ' ')
	if end < 0 {
		end = len(line[start:])
	}
	levelStr := line[start : start+end]
	ansi := ansiForLevelString(levelStr)
	if ansi == "" {
		return line
	}
	return line[:start] + ansi + levelStr + "\x1b[0m" + line[start+end:]
}

func ansiForLevelString(s string) string {
	switch {
	case strings.HasPrefix(s, "ERROR"):
		return "\x1b[31m" // red
	case strings.HasPrefix(s, "WARN"):
		return "\x1b[33m" // yellow
	case strings.HasPrefix(s, "INFO"):
		return "\x1b[32m" // green
	case strings.HasPrefix(s, "DEBUG"):
		return "\x1b[36m" // cyan
	default:
		return ""
	}
}
