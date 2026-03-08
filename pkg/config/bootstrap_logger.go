package config

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// NewBootstrapLogger creates a process-wide logger from bootstrap settings.
//
// It returns:
//   - a configured logger
//   - a cleanup function (closes file outputs when used; no-op otherwise)
//   - an error if configuration is invalid or output cannot be opened
func NewBootstrapLogger(cfg BootstrapConfig) (*slog.Logger, func(), error) {
	writer, cleanup, err := bootstrapLogWriter(cfg.LogOutput)
	if err != nil {
		return nil, func() {}, err
	}

	opts := &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}

	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(cfg.LogFormat)) {
	case "text":
		colorMode := strings.ToLower(strings.TrimSpace(cfg.LogColor))
		w := io.Writer(writer)
		if resolveColorEnabled(colorMode, writer) {
			w = colorizingWriter{writer}
		}
		handler = slog.NewTextHandler(w, opts)
	case "json":
		handler = slog.NewJSONHandler(writer, opts)
	default:
		cleanup()
		return nil, func() {}, fmt.Errorf("unsupported log format %q (expected text|json)", cfg.LogFormat)
	}

	return slog.New(handler), cleanup, nil
}

func bootstrapLogWriter(output string) (io.Writer, func(), error) {
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "", "stdout":
		return os.Stdout, func() {}, nil
	case "stderr":
		return os.Stderr, func() {}, nil
	default:
		f, err := os.OpenFile(output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o666)
		if err != nil {
			return nil, func() {}, fmt.Errorf("failed to open log output %q: %w", output, err)
		}
		return f, func() { _ = f.Close() }, nil
	}
}

func resolveColorEnabled(mode string, w io.Writer) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	case "auto", "":
		if f, ok := w.(*os.File); ok {
			fi, err := f.Stat()
			if err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
				return true
			}
		}
		return false
	default:
		return false
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
