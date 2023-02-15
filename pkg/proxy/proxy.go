package proxy

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

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
	p.lb = balancer.NewRandomBalancer(routeConfig.Upstreams)

	return p
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := p.lb.Balance()
    log.Printf("Selected %s", host)
	p.hosts[host].ServeHTTP(w, r)
}
