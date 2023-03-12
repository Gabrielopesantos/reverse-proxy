package proxy

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/balancer"
	"github.com/gabrielopesantos/reverse-proxy/pkg/config"
)

type Proxy struct {
	hosts map[string]*httputil.ReverseProxy
	lb    balancer.Balancer
}

func New(routeConfig *config.Route) *Proxy {
	p := &Proxy{}
	hosts := make(map[string]*httputil.ReverseProxy, 0)

	for _, host := range routeConfig.Upstreams {
		url, err := url.Parse(host)
		if err != nil {
			// NOTE: Return an error
			log.Fatalf("Failed to parse url %s. Err: %s", url, err)
		}
		proxy := httputil.NewSingleHostReverseProxy(url)

		originDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originDirector(req)
		}

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

	go p.monitorUpstreamHostsHealth()

	return p
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, _ := p.lb.Balance()
	p.hosts[host].ServeHTTP(w, r)
}

func (p *Proxy) monitorUpstreamHostsHealth() {
	var hostIndex int
	for host := range p.hosts {
		go p.healthCheck(host, hostIndex)
	}
}

func (p *Proxy) healthCheck(hostAddr string, hostIndex int) {
	ticker := time.NewTicker(5 * time.Second) // Going to be healthcheck interval

	for range ticker.C {
		if isAlive(hostAddr) {
			log.Printf("Successfully reached upstream '%s', removing from list of unhealhty hosts", hostAddr)
			p.lb.Remove(hostIndex)
		} else {
			log.Printf("Failed to reach upstream '%s', adding to list of unhealthy hosts", hostAddr)
			p.lb.Add(hostIndex)
		}
	}
}

// Unrelated functions
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
