package proxy

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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

func (p *Proxy) Start() error {
	go func() {
		log.Printf("Server is listening on address: %s", p.config.Server.Address)
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
	for route, routeData := range p.config.Routes {
		p.mapHandler(route, routeData)
	}
}

func (p *Proxy) mapHandler(route string, routeData config.Route) {
	http.HandleFunc(route, middleware.Logger(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Server", "Gabriel")
		resp, err := http.Get("http://" + routeData.Upstreams[0])
		if err != nil {
			w.WriteHeader(resp.StatusCode)
			return
		}
		w.WriteHeader(200)
		body, _ := io.ReadAll(resp.Body)
		w.Write(body)
	}))
}
