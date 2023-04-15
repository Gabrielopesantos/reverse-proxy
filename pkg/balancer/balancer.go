package balancer

import (
	"errors"
	"math/rand"
	"sync"

	"github.com/gabrielopesantos/reverse-proxy/pkg/utilities"
)

var (
	NoHostError = errors.New("no healthy upstream host found")
)

type LoadBalancerPolicy string

const (
	RANDOM      LoadBalancerPolicy = "random"
	ROUND_ROBIN                    = "round_robin"
)

// A Balancer selects which target host is going to be consumed based on the
// logic defined
//
// Balance should return URL of the host that is going to be requests
type Balancer interface {
	SetHealthStatus(string, bool)
	Balance() (string, error)
}

type BaseBalancer struct {
	sync.RWMutex
	hosts       map[string]bool
	indexToHost map[uint]string
}

func (b *BaseBalancer) SetHealthStatus(host string, isHealthy bool) {
	b.Lock()
	defer b.Unlock()

	b.hosts[host] = isHealthy
}

type RandomBalancer struct {
	BaseBalancer
}

func (b *BaseBalancer) setHostToIndex() {
	b.Lock()
	defer b.Unlock()

	var indexPosition uint
	for k := range b.hosts {
		b.indexToHost[indexPosition] = k
		indexPosition += 1
	}
}

func NewRandomBalancer(hosts map[string]bool) Balancer {
	balancer := &RandomBalancer{
		BaseBalancer: BaseBalancer{
			hosts:       hosts,
			indexToHost: make(map[uint]string, len(hosts)),
		},
	}
	balancer.setHostToIndex()

	return balancer
}

func (r *RandomBalancer) Balance() (string, error) {
	hostsChecked := utilities.NewSet[string]()
	for {
		randHostIndex := rand.Intn(len(r.hosts))
		host := r.indexToHost[uint(randHostIndex)]
		if r.hosts[host] {
			return host, nil
		}

		hostsChecked.Add(host)
		if hostsChecked.Len() == len(r.hosts) {
			return "", NoHostError
		}
	}
}

// type RoundRobinBalancer struct {
// 	BaseBalancer
// 	currentHostIndex int
// }
//
// func NewRoundRobinBalancer(hosts []string) Balancer {
// 	return &RoundRobinBalancer{
// 		BaseBalancer: BaseBalancer{
// 			hosts: hosts,
// 		},
// 		currentHostIndex: 0,
// 	}
// }
//
// // NOTE: Doesn't work with current approach
// func (rr *RoundRobinBalancer) Balance() (string, error) {
// 	if len(rr.hosts) == 0 {
// 		return "", NoHostError
// 	}
// 	host := rr.hosts[rr.currentHostIndex]
//
// 	if rr.currentHostIndex <= len(rr.hosts)-2 {
// 		rr.currentHostIndex += 1
// 	} else {
// 		rr.currentHostIndex = 0
// 	}
//
// 	return host, nil
// }
