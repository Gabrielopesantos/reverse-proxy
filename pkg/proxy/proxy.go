package proxy

import (
	"context"
	"fmt"
	"log"
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
	sync.RWMutex
	hosts       map[string]*httputil.ReverseProxy
	hostsHealth map[string]bool
	lb          balancer.Balancer
	hcInterval  time.Duration
}

func New(cfg *config.Route) (*Proxy, error) {
	p := &Proxy{}
	hosts := make(map[string]*httputil.ReverseProxy)
	hostsHealth := make(map[string]bool)
	for _, host := range cfg.Upstreams {
		if host == "" {
			return nil, fmt.Errorf("invalid empty upstream host in route")
		}
		upstreamURL, err := url.Parse(host)
		if err != nil {
			return nil, fmt.Errorf("failed to parse upstream url '%s': %w", host, err)
		}
		rProxy := httputil.NewSingleHostReverseProxy(upstreamURL)

		originDirector := rProxy.Director
		rProxy.Director = func(req *http.Request) {
			originDirector(req)
			injectForwardedHeaders(req)
		}
		hostsHealth[host] = false
		hosts[host] = rProxy
	}

	p.hosts = hosts
	p.hostsHealth = hostsHealth
	p.lb = getLoadBalancer(cfg.LoadBalancerPolicy)(p.hostsHealth)

	if cfg.HealthCheckIntervalSeconds > 0 {
		p.hcInterval = time.Duration(cfg.HealthCheckIntervalSeconds) * time.Second
	} else {
		p.hcInterval = defaultHealthCheckInterval
	}

	go p.monitorUpstreamHostsHealth(context.TODO())

	return p, nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, err := p.lb.Balance()
	if err != nil {
		log.Printf("no healthy upstream available for %s: %v", r.URL.Path, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	log.Printf("Host returned: %s", host)
	p.hosts[host].ServeHTTP(w, r)
}

func (p *Proxy) monitorUpstreamHostsHealth(ctx context.Context) {
	for host := range p.hosts {
		go p.healthCheck(host)
	}
}

func (p *Proxy) healthCheck(hostAddr string) {
	ticker := time.NewTicker(p.hcInterval)

	for range ticker.C {
		if isAlive(hostAddr) {
			if !p.markedAsHealthy(hostAddr) {
				log.Printf("successfully reached upstream host '%s', marking as healthy", hostAddr)
				p.lb.SetHealthStatus(hostAddr, true)
				metrics.UpstreamHealth.WithLabelValues(hostAddr).Set(1)
			}
		} else {
			if p.markedAsHealthy(hostAddr) {
				p.lb.SetHealthStatus(hostAddr, false)
				metrics.UpstreamHealth.WithLabelValues(hostAddr).Set(0)
				log.Printf("could not reach upstream host '%s', marking as unhealthy", hostAddr)
			}
		}
	}
}

func (p *Proxy) markedAsHealthy(host string) bool {
	p.Lock()
	defer p.Unlock()

	return p.hostsHealth[host]
}

func getLoadBalancer(policy balancer.LoadBalancerPolicy) func(map[string]bool) balancer.Balancer {
	switch policy {
	case balancer.RANDOM:
		return balancer.NewRandomBalancer
	case balancer.ROUND_ROBIN:
		return balancer.NewRoundRobinBalancer
	default:
		return balancer.NewRandomBalancer
	}
}

// injectForwardedHeaders sets X-Forwarded-For, X-Real-IP, and X-Forwarded-Proto
// on the outbound request so upstreams can identify the original client.
func injectForwardedHeaders(req *http.Request) {
	clientIP, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		clientIP = req.RemoteAddr
	}

	if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
		req.Header.Set("X-Forwarded-For", prior+", "+clientIP)
	} else {
		req.Header.Set("X-Forwarded-For", clientIP)
	}

	if req.Header.Get("X-Real-IP") == "" {
		req.Header.Set("X-Real-IP", clientIP)
	}

	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	}
	req.Header.Set("X-Forwarded-Proto", scheme)
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
