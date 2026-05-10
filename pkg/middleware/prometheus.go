package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/metrics"
)

func init() {
	RegisterYAML(PROMETHEUS, func() *Prometheus { return &Prometheus{} })
}

// Prometheus is middleware that records per-route request metrics.
type Prometheus struct {
	Route  string `yaml:"route"`
	logger *slog.Logger
}

func (p *Prometheus) Init(ctx context.Context) error {
	p.logger = LoggerFromContext(ctx)
	if p.Route == "" {
		return fmt.Errorf("prometheus middleware requires a non-empty route label")
	}
	return nil
}

func (p *Prometheus) Exec(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := NewLoggingResponseWriter(w)
		next.ServeHTTP(lrw, r)

		status := lrw.statusCode
		if status == 0 {
			status = http.StatusOK
		}

		metrics.RequestsTotal.
			WithLabelValues(p.Route, r.Method, statusBucket(status)).
			Inc()
		metrics.RequestDuration.
			WithLabelValues(p.Route, r.Method).
			Observe(time.Since(start).Seconds())
	})
}

func (p *Prometheus) Close() error { return nil }

// statusBucket collapses HTTP status codes into "1xx".."5xx" so that the
// Prometheus label cardinality stays bounded regardless of upstream behaviour.
func statusBucket(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	case code >= 200:
		return "2xx"
	case code >= 100:
		return "1xx"
	default:
		return "unknown"
	}
}
