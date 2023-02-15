package proxy

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

type Proxy struct {
	hosts []*httputil.ReverseProxy
}

func New(targetHosts []string) *Proxy {
	p := &Proxy{}
	hosts := make([]*httputil.ReverseProxy, 0)

	for _, host := range targetHosts {
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

		hosts = append(hosts, proxy)
	}
	p.hosts = hosts

	return p
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // Assuming first host as only host for now
	p.hosts[0].ServeHTTP(w, r)
}
