// Package metrics provides Prometheus instrumentation for the reverse proxy.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RequestsTotal counts all proxied requests by route, method and status code.
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "proxy_requests_total",
			Help: "Total number of HTTP requests proxied.",
		},
		[]string{"route", "method", "status"},
	)

	// RequestDuration measures the latency of each proxied request.
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "proxy_request_duration_seconds",
			Help:    "Histogram of proxied request latencies.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"route", "method"},
	)

	// UpstreamHealth tracks whether each upstream host is healthy (1) or not (0).
	UpstreamHealth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "proxy_upstream_healthy",
			Help: "1 if the upstream host is healthy, 0 otherwise.",
		},
		[]string{"host"},
	)
)
