package balancer

import (
	"errors"
	"net/http"
	"sync"
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

type BaseBalancer struct {
	sync.Mutex
	hosts    map[string]bool
	hostList []string
}

func newBaseBalancer(hosts map[string]bool) *BaseBalancer {
	hostList := make([]string, 0, len(hosts))
	for h := range hosts {
		hostList = append(hostList, h)
	}
	return &BaseBalancer{
		hosts:    hosts,
		hostList: hostList,
	}
}

func (b *BaseBalancer) SetHealthStatus(host string, isHealthy bool) {
	b.Lock()
	defer b.Unlock()

	b.hosts[host] = isHealthy
}

// New constructs a Balancer for the given strategy using the registered factory.
// Falls back to random if the strategy is not registered.
func New(strategy LoadBalancerStrategy, hosts map[string]bool, weights map[string]int) Balancer {
	if f, ok := balancerRegistry[strategy]; ok {
		return f(hosts, weights)
	}
	return NewRandomBalancer(hosts)
}
