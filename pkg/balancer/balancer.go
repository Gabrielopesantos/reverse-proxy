package balancer

import (
	"errors"
	"net/http"
	"sync/atomic"
)

var (
	ErrNoHost = errors.New("no healthy upstream host found")
)

type LoadBalancerStrategy string

const (
	RANDOM               LoadBalancerStrategy = "random"
	ROUND_ROBIN          LoadBalancerStrategy = "round_robin"
	WEIGHTED_ROUND_ROBIN LoadBalancerStrategy = "weighted_round_robin"
	LEAST_CONNECTIONS    LoadBalancerStrategy = "least_connections"
	IP_HASH              LoadBalancerStrategy = "ip_hash"
)

// Balancer selects which target host is going to serve the request.
// All strategies receive the incoming request; implementations that do not use
// it (e.g. round-robin) simply ignore it.
type Balancer interface {
	BalanceFor(r *http.Request) (string, error)
}

// HealthSetter lets the health-checker update a balancer's view of upstream
// state. Balancers that don't embed BaseBalancer can opt out entirely.
type HealthSetter interface {
	SetHealthStatus(host string, healthy bool)
}

// Releaser is implemented by balancers that track active connections.
// Release must be called after the upstream request completes.
type Releaser interface {
	Release(host string)
}

// BaseBalancer holds the immutable host set and a lock-free per-host health
// vector. The host list and host->index map are populated at construction and
// never mutated, so reads are race-free without synchronisation. Only the
// per-host health bools are atomic.
type BaseBalancer struct {
	hostList []string
	hostIdx  map[string]int
	healthy  []atomic.Bool
}

func newBaseBalancer(hosts map[string]bool) *BaseBalancer {
	b := &BaseBalancer{
		hostList: make([]string, 0, len(hosts)),
		hostIdx:  make(map[string]int, len(hosts)),
		healthy:  make([]atomic.Bool, len(hosts)),
	}
	for h, ok := range hosts {
		i := len(b.hostList)
		b.hostList = append(b.hostList, h)
		b.hostIdx[h] = i
		b.healthy[i].Store(ok)
	}
	return b
}

// SetHealthStatus is safe to call concurrently with BalanceFor on any
// embedding balancer.
func (b *BaseBalancer) SetHealthStatus(host string, isHealthy bool) {
	if i, ok := b.hostIdx[host]; ok {
		b.healthy[i].Store(isHealthy)
	}
}

func (b *BaseBalancer) isHealthy(i int) bool {
	return b.healthy[i].Load()
}

// New constructs a Balancer for the given strategy using the registered factory.
// Falls back to random if the strategy is not registered.
func New(strategy LoadBalancerStrategy, hosts map[string]bool, weights map[string]int) Balancer {
	if f, ok := balancerRegistry[strategy]; ok {
		return f(hosts, weights)
	}
	return NewRandomBalancer(hosts)
}
