package proxy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/balancer"
	"github.com/gabrielopesantos/reverse-proxy/pkg/metrics"
)

const defaultHealthCheckInterval = 5 * time.Second

// healthCheckClient is shared across all health probes. DisableKeepAlives
// ensures each probe opens a fresh connection (no stale pool entries).
var healthCheckClient = &http.Client{
	Timeout: 3 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{DisableKeepAlives: true},
}

type Proxy struct {
	// Hosts maps upstream URLs to httputil.ReverseProxy instances.
	hosts map[string]*httputil.ReverseProxy

	// Load balancer used to select an upstream for each request.
	lb balancer.Balancer

	// Health check interval and path.
	hcInterval time.Duration
	hcPath     string

	logger *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc
}

// buildUpstreams parses each upstream URL, creates an httputil.ReverseProxy
// with forwarded-header injection, and returns both the proxy map and an
// initial health map (all false until probed).
func buildUpstreams(upstreams []string) (map[string]*httputil.ReverseProxy, map[string]bool, error) {
	hostRevProxyMap := make(map[string]*httputil.ReverseProxy, len(upstreams))
	hostHealthMap := make(map[string]bool, len(upstreams))
	for _, host := range upstreams {
		if host == "" {
			return nil, nil, fmt.Errorf("invalid empty upstream host in route")
		}
		upstreamURL, err := url.Parse(host)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse upstream url '%s': %w", host, err)
		}
		target := upstreamURL // capture for closure
		reverseProxy := httputil.NewSingleHostReverseProxy(target)
		reverseProxy.Director = nil // NewSingleHostReverseProxy sets Director; clear it so Rewrite is the sole handler
		reverseProxy.Rewrite = func(req *httputil.ProxyRequest) {
			req.SetURL(target)
			injectForwardedHeaders(req)
		}
		hostHealthMap[host] = false
		hostRevProxyMap[host] = reverseProxy
	}
	return hostRevProxyMap, hostHealthMap, nil
}

func New(upstreams []string, opts ...Option) (*Proxy, error) {
	o := &options{
		lbStrategy: balancer.RANDOM,
		hcInterval: defaultHealthCheckInterval,
		hcPath:     "/",
		logger:     slog.Default(),
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.hcPath == "" {
		o.hcPath = "/"
	}

	hosts, hostsHealth, err := buildUpstreams(upstreams)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &Proxy{
		logger:     o.logger,
		ctx:        ctx,
		cancel:     cancel,
		hcPath:     o.hcPath,
		hcInterval: o.hcInterval,
	}

	// Probe all upstreams concurrently so the balancer starts with accurate
	// health state instead of treating every host as unhealthy for up to hcInterval.
	var mu sync.Mutex
	var wg sync.WaitGroup
	for host := range hostsHealth {
		wg.Add(1)
		go func(h string) {
			defer wg.Done()
			alive := probeUpstream(h, p.hcPath)
			mu.Lock()
			hostsHealth[h] = alive
			mu.Unlock()
		}(host)
	}
	wg.Wait()

	p.hosts = hosts
	p.lb = balancer.New(o.lbStrategy, hostsHealth, o.weights)

	p.startHealthChecks()

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

func (p *Proxy) startHealthChecks() {
	for host := range p.hosts {
		go p.healthCheck(host)
	}
}

func (p *Proxy) healthCheck(hostAddr string) {
	ticker := time.NewTicker(p.hcInterval)
	defer ticker.Stop()
	prevHealthy := false
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			alive := probeUpstream(hostAddr, p.hcPath)
			if hs, ok := p.lb.(balancer.HealthSetter); ok {
				if alive && !prevHealthy {
					hs.SetHealthStatus(hostAddr, true)
					metrics.UpstreamHealth.WithLabelValues(hostAddr).Set(1)
					p.logger.Debug("upstream healthy", "host", hostAddr)
				} else if !alive && prevHealthy {
					hs.SetHealthStatus(hostAddr, false)
					metrics.UpstreamHealth.WithLabelValues(hostAddr).Set(0)
					p.logger.Warn("upstream unhealthy", "host", hostAddr)
				}
			}
			prevHealthy = alive
		}
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

// probeUpstream performs an HTTP GET to path on hostAddr and returns true when
// the response status is below 500. Any 1xx–4xx is treated as healthy (the
// application is reachable and processing requests). A connection error,
// timeout, or 5xx means the upstream is unhealthy.
func probeUpstream(hostAddr, path string) bool {
	u, err := url.Parse(hostAddr)
	if err != nil || u.Host == "" {
		return false
	}
	u.Path = path
	resp, err := healthCheckClient.Get(u.String())
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode < http.StatusInternalServerError
}
