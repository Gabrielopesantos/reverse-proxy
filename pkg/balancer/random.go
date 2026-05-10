package balancer

import (
	"math/rand"
	"net/http"
)

func init() {
	Register(RANDOM, func(hosts map[string]bool, _ map[string]int) Balancer {
		return NewRandomBalancer(hosts)
	})
}

// RandomBalancer selects a host at random. Lock-free; relies on math/rand's
// internal synchronisation.
type RandomBalancer struct {
	*BaseBalancer
}

func NewRandomBalancer(hosts map[string]bool) Balancer {
	return &RandomBalancer{
		BaseBalancer: newBaseBalancer(hosts),
	}
}

func (r *RandomBalancer) BalanceFor(_ *http.Request) (string, error) {
	n := len(r.hostList)
	if n == 0 {
		return "", ErrNoHost
	}

	start := rand.Intn(n)
	for i := range n {
		idx := (start + i) % n
		if r.isHealthy(idx) {
			return r.hostList[idx], nil
		}
	}

	return "", ErrNoHost
}
