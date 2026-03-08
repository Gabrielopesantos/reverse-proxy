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
type Balancer interface {
	Balance() (string, error)
}

// HealthSetter lets the health-checker update a balancer's view of upstream
// state. Balancers that don't embed BaseBalancer can opt out entirely.
type HealthSetter interface {
	SetHealthStatus(host string, healthy bool)
}

// RequestBalancer is implemented by balancers that need the incoming request
// to make a routing decision (e.g. IP hash).
type RequestBalancer interface {
	Balancer
	BalanceFor(r *http.Request) (string, error)
}

// Releaser is implemented by balancers that track active connections.
// Release must be called after the upstream request completes.
type Releaser interface {
	Release(host string)
}

// Pick calls BalanceFor if lb implements RequestBalancer, otherwise Balance.
func Pick(lb Balancer, r *http.Request) (string, error) {
	if rb, ok := lb.(RequestBalancer); ok {
		return rb.BalanceFor(r)
	}
	return lb.Balance()
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

// New constructs a Balancer for the given strategy.
func New(strategy LoadBalancerStrategy, hosts map[string]bool, weights map[string]int) Balancer {
	switch strategy {
	case WEIGHTED_ROUND_ROBIN:
		return NewWeightedRoundRobinBalancer(hosts, weights)
	case LEAST_CONNECTIONS:
		return NewLeastConnectionsBalancer(hosts)
	case IP_HASH:
		return NewIPHashBalancer(hosts)
	case ROUND_ROBIN:
		return NewRoundRobinBalancer(hosts)
	default:
		return NewRandomBalancer(hosts)
	}
}
