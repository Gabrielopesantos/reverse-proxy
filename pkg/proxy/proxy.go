package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/balancer"
	"github.com/gabrielopesantos/reverse-proxy/pkg/config"
	"github.com/gabrielopesantos/reverse-proxy/pkg/metrics"
)

const defaultHealthCheckInterval = 5 * time.Second

type Proxy struct {
	hosts      map[string]*httputil.ReverseProxy
	lb         balancer.Balancer
	hcInterval time.Duration
	logger     *slog.Logger
	ctx        context.Context
	cancel     context.CancelFunc
}

func New(cfg *config.Route, logger *slog.Logger) (*Proxy, error) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Proxy{
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}
	hosts := make(map[string]*httputil.ReverseProxy)
	hostsHealth := make(map[string]bool)
	for _, host := range cfg.Upstreams {
		if host == "" {
			cancel()
			return nil, fmt.Errorf("invalid empty upstream host in route")
		}
		upstreamURL, err := url.Parse(host)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to parse upstream url '%s': %w", host, err)
		}
		rProxy := httputil.NewSingleHostReverseProxy(upstreamURL)

		originRewrite := rProxy.Rewrite
		rProxy.Rewrite = func(req *httputil.ProxyRequest) {
			originRewrite(req)
			injectForwardedHeaders(req)
		}
		hostsHealth[host] = false
		hosts[host] = rProxy
	}

	// Probe all upstreams concurrently so the balancer starts with accurate
	// health state instead of treating every host as unhealthy for up to hcInterval.
	var mu sync.Mutex
	var wg sync.WaitGroup
	for host := range hostsHealth {
		wg.Add(1)
		go func(h string) {
			defer wg.Done()
			alive := isAlive(h)
			mu.Lock()
			hostsHealth[h] = alive
			mu.Unlock()
		}(host)
	}
	wg.Wait()

	p.hosts = hosts
	p.lb = getLoadBalancer(cfg, hostsHealth)

	if cfg.HealthCheckIntervalSeconds > 0 {
		p.hcInterval = time.Duration(cfg.HealthCheckIntervalSeconds) * time.Second
	} else {
		p.hcInterval = defaultHealthCheckInterval
	}

	p.monitorUpstreamHostsHealth()

	return p, nil
}

func (p *Proxy) Stop() {
	p.cancel()
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, err := balancer.Pick(p.lb, r)
	if err != nil {
		p.logger.Error("no healthy upstream available", "path", r.URL.Path, "err", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	if rel, ok := p.lb.(balancer.Releaser); ok {
		defer rel.Release(host)
	}

	p.hosts[host].ServeHTTP(w, r)
}

func (p *Proxy) monitorUpstreamHostsHealth() {
	for host := range p.hosts {
		go p.healthCheck(host)
	}
}

func (p *Proxy) healthCheck(hostAddr string) {
	ticker := time.NewTicker(p.hcInterval)
	defer ticker.Stop()
	wasHealthy := false
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			alive := isAlive(hostAddr)
			if hs, ok := p.lb.(balancer.HealthSetter); ok {
				if alive && !wasHealthy {
					hs.SetHealthStatus(hostAddr, true)
					metrics.UpstreamHealth.WithLabelValues(hostAddr).Set(1)
					p.logger.Info("upstream healthy", "host", hostAddr)
				} else if !alive && wasHealthy {
					hs.SetHealthStatus(hostAddr, false)
					metrics.UpstreamHealth.WithLabelValues(hostAddr).Set(0)
					p.logger.Warn("upstream unhealthy", "host", hostAddr)
				}
			}
			wasHealthy = alive
		}
	}
}

func getLoadBalancer(cfg *config.Route, hosts map[string]bool) balancer.Balancer {
	switch cfg.LoadBalancerPolicy {
	case balancer.WEIGHTED_ROUND_ROBIN:
		return balancer.NewWeightedRoundRobinBalancer(hosts, cfg.Weights)
	case balancer.LEAST_CONNECTIONS:
		return balancer.NewLeastConnectionsBalancer(hosts)
	case balancer.IP_HASH:
		return balancer.NewIPHashBalancer(hosts)
	case balancer.ROUND_ROBIN:
		return balancer.NewRoundRobinBalancer(hosts)
	default:
		return balancer.NewRandomBalancer(hosts)
	}
}

// injectForwardedHeaders sets X-Forwarded-For, X-Real-IP, and X-Forwarded-Proto
// on the outbound request so upstreams can identify the original client.
func injectForwardedHeaders(proxyReq *httputil.ProxyRequest) {
	clientIP, _, err := net.SplitHostPort(proxyReq.In.RemoteAddr)
	if err != nil {
		clientIP = proxyReq.In.RemoteAddr
	}

	if prior := proxyReq.In.Header.Get("X-Forwarded-For"); prior != "" {
		proxyReq.In.Header.Set("X-Forwarded-For", prior+", "+clientIP)
	} else {
		proxyReq.In.Header.Set("X-Forwarded-For", clientIP)
	}

	if proxyReq.In.Header.Get("X-Real-IP") == "" {
		proxyReq.In.Header.Set("X-Real-IP", clientIP)
	}

	scheme := "http"
	if proxyReq.In.TLS != nil {
		scheme = "https"
	}
	proxyReq.In.Header.Set("X-Forwarded-Proto", scheme)
}

// isAlive checks if a TCP connection can be established to the given URL's host.
func isAlive(hostAddr string) bool {
	u, err := url.Parse(hostAddr)
	if err != nil || u.Host == "" {
		return false
	}
	host := u.Host
	// If no port specified, use default for scheme
	if _, _, err := net.SplitHostPort(host); err != nil {
		switch u.Scheme {
		case "https":
			host = host + ":443"
		default:
			host = host + ":80"
		}
	}
	conn, err := net.DialTimeout("tcp", host, 3*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
