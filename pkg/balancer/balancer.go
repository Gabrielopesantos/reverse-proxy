package balancer

import (
	"errors"
	"math/rand"
	"sync"
)

var (
	NoHostError = errors.New("no healthy upstream host found")
)

// A Balancer selects which target host is going to be consumed based on the
// logic defined
//
// Balance should return URL of the host that is going to be requests
type Balancer interface {
	Add(string)
	Remove(string)
	Balance() (string, error)
}

type BaseBalancer struct {
	sync.RWMutex
	hosts []string
}

func (b *BaseBalancer) Remove(host string) {
	b.Lock()
	defer b.Unlock()

	for i, h := range b.hosts {
		if h == host {
			b.hosts = append(b.hosts[:i], b.hosts[i+1:]...)
			break
		}
	}
}

func (b *BaseBalancer) Add(host string) {
	b.Lock()
	defer b.Unlock()

	b.hosts = append(b.hosts, host)
}

type RandomBalancer struct {
	BaseBalancer
}

func NewRandomBalancer(hosts []string) Balancer {
	return &RandomBalancer{
		BaseBalancer: BaseBalancer{
			hosts: hosts,
		},
	}
}

func (r *RandomBalancer) Balance() (string, error) {
	if len(r.hosts) == 0 {
		return "", NoHostError
	}
	randomHostIndex := rand.Intn(len(r.hosts))
	return r.hosts[randomHostIndex], nil
}

type RoundRobinBalancer struct {
	BaseBalancer
	currentHostIndex int
}

func NewRoundRobinBalancer(hosts []string) Balancer {
	return &RoundRobinBalancer{
		BaseBalancer: BaseBalancer{
			hosts: hosts,
		},
		currentHostIndex: 0,
	}
}

// NOTE: Doesn't work with current approach
func (rr *RoundRobinBalancer) Balance() (string, error) {
	if len(rr.hosts) == 0 {
		return "", NoHostError
	}
	host := rr.hosts[rr.currentHostIndex]

	if rr.currentHostIndex <= len(rr.hosts)-2 {
		rr.currentHostIndex += 1
	} else {
		rr.currentHostIndex = 0
	}

	return host, nil
}
