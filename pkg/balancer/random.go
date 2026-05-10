package balancer

import (
	"math/rand"
	"net/http"
)

// RandomBalancer is a balancer that selects a host randomly.
type RandomBalancer struct {
	*BaseBalancer
}

func NewRandomBalancer(hosts map[string]bool) Balancer {
	return &RandomBalancer{
		BaseBalancer: newBaseBalancer(hosts),
	}
}

func (r *RandomBalancer) BalanceFor(_ *http.Request) (string, error) {
	r.BaseBalancer.Lock()
	defer r.BaseBalancer.Unlock()

	n := len(r.hostList)
	if n == 0 {
		return "", ErrNoHost
	}

	start := rand.Intn(n)
	for i := 0; i < n; i++ {
		host := r.hostList[(start+i)%n]
		if r.hosts[host] {
			return host, nil
		}
	}

	return "", ErrNoHost
}

func init() {
	Register(RANDOM, func(hosts map[string]bool, _ map[string]int) Balancer {
		return NewRandomBalancer(hosts)
	})
}
