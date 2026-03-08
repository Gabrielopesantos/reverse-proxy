package balancer

import (
	"errors"
	"net/http"
	"sync"
)

var (
	ErrNoHost = errors.New("no healthy upstream host found")
)

type LoadBalancerPolicy string

const (
	RANDOM               LoadBalancerPolicy = "random"
	ROUND_ROBIN          LoadBalancerPolicy = "round_robin"
	WEIGHTED_ROUND_ROBIN LoadBalancerPolicy = "weighted_round_robin"
	LEAST_CONNECTIONS    LoadBalancerPolicy = "least_connections"
	IP_HASH              LoadBalancerPolicy = "ip_hash"
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
