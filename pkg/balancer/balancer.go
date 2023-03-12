package balancer

import (
	"errors"
	"math/rand"
	"sync"

	"golang.org/x/exp/slices"
)

var (
	NoHostError = errors.New("No healthy upstream host found")
)

// A Balancer selects which target host is going to be consumed based on the
// logic defined
//
// Balance should return URL of the host that is going to be requests
type Balancer interface {
	// These methods will be used for checking alives
	Add(int)
	Remove(int)
	Balance() (string, error)
}

type BaseBalancer struct {
	sync.RWMutex
	hosts     []string
	unhealthy []int // Indexes of unhealthy upstream hosts
}

func (b *BaseBalancer) Remove(hostPosIndex int) {
	b.Lock()
	defer b.Unlock()
	b.unhealthy = append(b.unhealthy, hostPosIndex)
}

func (b *BaseBalancer) Add(hostPosIndex int) {
	b.Lock()
	defer b.Unlock()
	var unhealthyIndexPos int
	for i, hostIndex := range b.unhealthy {
		if hostPosIndex == hostIndex {
			unhealthyIndexPos = i
		}
	}
	b.unhealthy = slices.Delete(b.unhealthy, unhealthyIndexPos, unhealthyIndexPos+1)
}

type RandomBalancer struct {
	BaseBalancer
}

func NewRandomBalancer(hosts []string) Balancer {
	return &RandomBalancer{
		BaseBalancer: BaseBalancer{
			hosts:     hosts,
			unhealthy: make([]int, 1),
		},
	}
}

func (r *RandomBalancer) Balance() (string, error) {
	// Still doesn't handle cases where none of the upstreams hosts is unhealth
	return r.hosts[rand.Intn(len(r.hosts))], nil
}

type RoundRobinBalancer struct {
	BaseBalancer
	currentHostIndex int
}

func NewRoundRobinBalancer(hosts []string) Balancer {
	return &RoundRobinBalancer{
		BaseBalancer: BaseBalancer{
			hosts:     hosts,
			unhealthy: make([]int, 1),
		},
		currentHostIndex: 0,
	}
}

func (rr *RoundRobinBalancer) Balance() (string, error) {
	// Still doesn't handle cases where none of the upstreams hosts is unhealth
	host := rr.hosts[rr.currentHostIndex]

	if rr.currentHostIndex <= len(rr.hosts)-2 {
		rr.currentHostIndex += 1
	} else {
		rr.currentHostIndex = 0
	}

	return host, nil
}
