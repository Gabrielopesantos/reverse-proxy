package server

import (
	"log"
	"time"

	"github.com/gabrielopesantos/reverse-proxy/pkg/config"
)

type server struct {
	config *config.Config
}

func NewServer(config *config.Config) *server {
	return &server{
		config: config,
	}
}

func (s *server) Run() {
	log.Println("I am running")
	time.Sleep(5 * time.Second)
}
