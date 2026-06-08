package balancer

import (
	"net/http"
	"sync/atomic"
)

func init() {
	Register(RoundRobin, func(hosts map[string]bool, _ map[string]int) Balancer {
		return NewRoundRobinBalancer(hosts)
	})
}

// RoundRobinBalancer selects hosts in a round-robin fashion using a lock-free
// atomic counter. Skips unhealthy hosts.
type RoundRobinBalancer struct {
	*BaseBalancer
	counter atomic.Uint64
}

func NewRoundRobinBalancer(hosts map[string]bool) Balancer {
	return &RoundRobinBalancer{
		BaseBalancer: newBaseBalancer(hosts),
	}
}

func (rr *RoundRobinBalancer) BalanceFor(_ *http.Request) (string, error) {
	n := len(rr.hostList)
	if n == 0 {
		return "", ErrNoHost
	}

	start := int(rr.counter.Add(1)-1) % n
	for i := range n {
		idx := (start + i) % n
		if rr.isHealthy(idx) {
			return rr.hostList[idx], nil
		}
	}

	return "", ErrNoHost
}
