package proxy

import (
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/config"
	"github.com/gabrielopesantos/reverse-proxy/pkg/middleware"
)

type Proxy struct {
	server http.Server
	config *config.Config
}

func NewServer(config *config.Config) *Proxy {
	return &Proxy{
		server: http.Server{
			Addr:        config.Server.Address,
			ReadTimeout: time.Duration(config.Server.ReadTimeoutSeconds) * time.Second,
		},
		config: config,
	}
}

func (p *Proxy) Start() {
	// Setup endpoints
	p.provision()

	if err := p.server.ListenAndServe(); err != http.ErrServerClosed {
		log.Printf("Error ListenAndServe: %v", err)
	}
}

func (p *Proxy) provision() {
	for endpoint, route := range p.config.Routes {
		p.mapRoute(endpoint, route)
	}
}

func (p *Proxy) mapRoute(endpoint string, route config.Route) {
	http.HandleFunc(endpoint, middleware.Logger(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Server", "Gabriel")
		resp, err := http.Get("http://" + route.Upstreams[0])
		if err != nil {
			w.WriteHeader(resp.StatusCode)
			return
		}
		w.WriteHeader(200)
		body, _ := io.ReadAll(resp.Body)
		w.Write(body)
	}))
}
