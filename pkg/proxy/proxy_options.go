package proxy

import (
	"log/slog"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/balancer"
)

// Option configures a Proxy at construction time.
type Option func(*options)

type options struct {
	lbStrategy balancer.LoadBalancerStrategy
	weights    map[string]int
	hcInterval time.Duration
	hcPath     string
	logger     *slog.Logger
}

func WithLoadBalancerStrategy(s balancer.LoadBalancerStrategy) Option {
	return func(o *options) { o.lbStrategy = s }
}

func WithWeights(w map[string]int) Option {
	return func(o *options) { o.weights = w }
}

func WithHealthCheckInterval(d time.Duration) Option {
	return func(o *options) { o.hcInterval = d }
}

func WithHealthCheckPath(path string) Option {
	return func(o *options) { o.hcPath = path }
}

func WithLogger(l *slog.Logger) Option {
	return func(o *options) { o.logger = l }
}
