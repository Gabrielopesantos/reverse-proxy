package proxy

import (
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
	hosts map[string]*httputil.ReverseProxy
	lb    balancer.Balancer

	// Keep a record of healthy upstream hosts
	healthy map[string]bool
}

func New(routeConfig *config.Route) *Proxy {
	p := &Proxy{}
	hosts := make(map[string]*httputil.ReverseProxy)
	healthy := make(map[string]bool)
	for _, host := range routeConfig.Upstreams {
		upstreamUrl, err := url.Parse(host)
		if err != nil {
			// NOTE: Return an error
			log.Fatalf("Failed to parse url %s. Err: %s", upstreamUrl, err)
		}
		proxy := httputil.NewSingleHostReverseProxy(upstreamUrl)

		originDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originDirector(req)
		}

        // NOTE: Start by considering all upstreams unhealhty and validate it first isAlive check 
		healthy[host] = true
		hosts[host] = proxy
	}

	p.hosts = hosts
	// Load Balancer Policy
	switch routeConfig.LoadBalancerPolicy {
	case "random":
		p.lb = balancer.NewRandomBalancer(routeConfig.Upstreams)
	case "round_robin":
		p.lb = balancer.NewRoundRobinBalancer(routeConfig.Upstreams)
	default:
		// Select first host defined by default (random for now)
		p.lb = balancer.NewRandomBalancer(routeConfig.Upstreams)
	}

	p.healthy = healthy

	go p.monitorUpstreamHostsHealth()

	return p
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, err := p.lb.Balance()
	if err != nil {
		// 502?
		http.Error(w, err.Error(), http.StatusBadGateway)
        return
	}

	log.Println(host)
	p.hosts[host].ServeHTTP(w, r)
}

func (p *Proxy) monitorUpstreamHostsHealth() {
	for host := range p.hosts {
		go p.healthCheck(host)
	}
}

func (p *Proxy) healthCheck(hostAddr string) {
	ticker := time.NewTicker(5 * time.Second) // Going to be healthcheck interval

	for range ticker.C {
		if isAlive(hostAddr) && !p.markedAsHealthy(hostAddr) {
			log.Printf("Successfully reached upstream '%s'", hostAddr)
			p.setHealthy(hostAddr, true)
			p.lb.Add(hostAddr)
		} else if !isAlive(hostAddr) && p.markedAsHealthy(hostAddr) {
			log.Printf("Failed to reach upstream '%s'", hostAddr)
			p.setHealthy(hostAddr, false)
			p.lb.Remove(hostAddr)
		}
	}
}

func (p *Proxy) markedAsHealthy(host string) bool {
	p.Lock()
	defer p.Unlock()

	return p.healthy[host]
}

func (p *Proxy) setHealthy(host string, healthy bool) {
	p.Lock()
	defer p.Unlock()

	p.healthy[host] = healthy
}

// *** Unrelated functions ***
// isAlive: Check if a connection is alive, a connection is considered alive if we can establish a tcp connection
func isAlive(hostAddr string) bool {
	// `host` cannot include the protocol (http://)
	hostAddr = hostAddr[7:] // tmp
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
