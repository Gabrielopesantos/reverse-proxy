package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/metrics"
)

// PrometheusConfig is middleware that records per-route request metrics.
type PrometheusConfig struct {
	Route string `json:"route"`
}

func (p *PrometheusConfig) Init(_ context.Context) error {
	if p.Route == "" {
		return fmt.Errorf("prometheus middleware requires a non-empty route label")
	}
	return nil
}

func (p *PrometheusConfig) Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := NewLoggingResponseWriter(w)
		next.ServeHTTP(lrw, r)

		status := lrw.statusCode
		if status == 0 {
			status = http.StatusOK
		}

		metrics.RequestsTotal.
			WithLabelValues(p.Route, r.Method, strconv.Itoa(status)).
			Inc()
		metrics.RequestDuration.
			WithLabelValues(p.Route, r.Method).
			Observe(time.Since(start).Seconds())
	}
}
