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
)

type Proxy struct {
	sync.RWMutex
	hosts       map[string]*httputil.ReverseProxy
	hostsHealth map[string]bool
	lb          balancer.Balancer
}

// TODO: Log if no upstream is available
func New(config *config.Route) (*Proxy, error) {
	p := &Proxy{}
	hosts := make(map[string]*httputil.ReverseProxy)
	hostsHealth := make(map[string]bool)
	for _, host := range config.Upstreams {
		if host == "" {
			// FIXME: This should include the route name
			return nil, fmt.Errorf("invalid host '%s' provided", host)
		}
		upstreamUrl, err := url.Parse(host)
		if err != nil {
			return nil, fmt.Errorf("failed to parse url '%s': %w", upstreamUrl, err)
		}
		rProxy := httputil.NewSingleHostReverseProxy(upstreamUrl)

		originDirector := rProxy.Director
		rProxy.Director = func(req *http.Request) {
			originDirector(req)
		}
		hostsHealth[host] = false
		hosts[host] = rProxy
	}

	// Set Hosts
	p.hosts = hosts
	// Set hostsHealth
	p.hostsHealth = hostsHealth
	// Set load balancer policy
	p.lb = getLoadBalancer(config.LoadBalancerPolicy)(p.hostsHealth)

	// FIXME: This should be executed on start
	go p.monitorUpstreamHostsHealth(context.TODO())

	return p, nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, err := p.lb.Balance()
	if err != nil {
		// 502?
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
	// FIXME: healthcheck interval config
	ticker := time.NewTicker(5 * time.Second)
	// NOTE: Remove / Remove what?

	for range ticker.C {
		if isAlive(hostAddr) {
			if !p.markedAsHealthy(hostAddr) {
				log.Printf("successfully reached upstream host '%s', marking as healthy", hostAddr)
				p.lb.SetHealthStatus(hostAddr, true)
			}
		} else {
			if p.markedAsHealthy(hostAddr) {
				p.lb.SetHealthStatus(hostAddr, false)
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

// *** Unrelated functions ***
// isAlive: Check if a connection is alive, a connection is considered alive if we can establish a tcp connection
func isAlive(hostAddr string) bool {
	// `host` cannot include the protocol (http://)
	hostAddr = hostAddr[7:] // FIXME: Protocol cannot be in host addr
	addr, err := net.ResolveTCPAddr("tcp", hostAddr)
	if err != nil {
		return false
	}
	resolveAddr := fmt.Sprintf("%s:%d", addr.IP, addr.Port)
	conn, err := net.DialTimeout("tcp", resolveAddr, 3*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
