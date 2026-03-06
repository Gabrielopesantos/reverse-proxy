package balancer

import (
	"errors"
	"math/rand"
	"sync"

	"github.com/gabrielopesantos/reverse-proxy/pkg/utilities"
)

var (
	ErrNoHost = errors.New("no healthy upstream host found")
)

type LoadBalancerPolicy string

const (
	RANDOM      LoadBalancerPolicy = "random"
	ROUND_ROBIN LoadBalancerPolicy = "round_robin"
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

func newBaseBalancer(hosts map[string]bool) *BaseBalancer {
	b := &BaseBalancer{
		hosts:       hosts,
		indexToHost: make(map[uint]string, len(hosts)),
	}
	b.setHostToIndex()

	return b
}

func (b *BaseBalancer) SetHealthStatus(host string, isHealthy bool) {
	b.Lock()
	defer b.Unlock()

	b.hosts[host] = isHealthy
}

type RandomBalancer struct {
	*BaseBalancer
}

func (b *BaseBalancer) setHostToIndex() {
	var indexPosition uint
	for k := range b.hosts {
		b.indexToHost[indexPosition] = k
		indexPosition += 1
	}
}

func NewRandomBalancer(hosts map[string]bool) Balancer {
	return &RandomBalancer{
		BaseBalancer: newBaseBalancer(hosts),
	}
}

func (r *RandomBalancer) Balance() (string, error) {
	r.BaseBalancer.Lock()
	defer r.BaseBalancer.Unlock()

	if len(r.hosts) == 0 {
		return "", ErrNoHost
	}

	hostsChecked := utilities.NewSet[string]()
	for {
		randHostIndex := rand.Intn(len(r.hosts))
		host := r.indexToHost[uint(randHostIndex)]
		if r.hosts[host] {
			return host, nil
		}

		hostsChecked.Add(host)
		if hostsChecked.Len() == len(r.hosts) {
			return "", ErrNoHost
		}
	}
}

type RoundRobinBalancer struct {
	*BaseBalancer
	currentHostIndex int
}

func NewRoundRobinBalancer(hosts map[string]bool) Balancer {
	balancer := &RoundRobinBalancer{
		BaseBalancer:     newBaseBalancer(hosts),
		currentHostIndex: 0,
	}

	return balancer
}

func (rr *RoundRobinBalancer) Balance() (string, error) {
	rr.BaseBalancer.Lock()
	defer rr.BaseBalancer.Unlock()

	if len(rr.hosts) == 0 {
		return "", ErrNoHost
	}

	hostsChecked := utilities.NewSet[string]()
	for {
		host := rr.indexToHost[uint(rr.currentHostIndex)]
		if healthy := rr.hosts[host]; !healthy {
			hostsChecked.Add(host)
			if hostsChecked.Len() == len(rr.hosts) {
				return "", ErrNoHost
			}
			rr.incrementCurrentHostIndex()
			continue
		}
		rr.incrementCurrentHostIndex()
		return host, nil
	}
}

// incrementCurrentHostIndex advances the round-robin pointer.
// Must be called with BaseBalancer.Lock already held.
func (rr *RoundRobinBalancer) incrementCurrentHostIndex() {
	rr.currentHostIndex += 1
	rr.currentHostIndex %= len(rr.hosts)
}
