package proxy

import (
	"context"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/config"
	//"github.com/gabrielopesantos/reverse-proxy/pkg/middleware"
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

func (p *Proxy) Start() error {
	go func() {
		log.Printf("Server listening on address: %s", p.config.Server.Address)
		if err := p.server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Error starting server. Err: %s", err)
		}
	}()

	// Map Handlers
	p.provision()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	<-quit

	ctx, shutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdown()

	log.Println("Server Exited Properly")
	return p.server.Shutdown(ctx)
}

func (p *Proxy) provision() {
	for pattern, routeConfig := range p.config.Routes {
		// NOTE: Let's assume that there's only an upstream target for now
		url, err := url.Parse(routeConfig.Upstreams[0])
		if err != nil {
			// NOTE: Return an error
			log.Fatalf("Failed to parse url %s. Err: %s", url, err)
		}
		proxy := httputil.NewSingleHostReverseProxy(url)

		originDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originDirector(req)
		}

		http.Handle(pattern, proxy)
	}
}
