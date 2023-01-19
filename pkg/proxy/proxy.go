package proxy

import (
	"log"
	"net/http"

	"github.com/gabrielopesantos/reverse-proxy/pkg/config"
)

type Proxy struct {
	server http.Server
	config *config.Config
}

func NewServer(config *config.Config) *Proxy {
	return &Proxy{
		server: http.Server{Addr: "localhost:8080"},
		config: config,
	}
}

func (p *Proxy) Run() {
	// Setup endpoints
	p.mapHandlers()

	// Run
	if err := p.server.ListenAndServe(); err != http.ErrServerClosed {
		log.Printf("Error ListenAndServe: %v", err)
	}
	//for {
	//log.Printf("%+v", s.config)

	//time.Sleep(5 * time.Second)
	//}
}

func (p *Proxy) mapHandlers() {
	for endpoint, route := range p.config.Routes {
		go func(endpoint, destination string) {
			http.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				w.Write([]byte(destination))
			})
		}(endpoint, route.Destination)
	}
}
