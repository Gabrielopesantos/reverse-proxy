package balancer

import (
	"math"
	"sync/atomic"
)

// LeastConnectionsBalancer routes each request to the healthy upstream with
// the fewest active connections. It implements Releaser so proxy.ServeHTTP can
// decrement the counter when the upstream response finishes.
type LeastConnectionsBalancer struct {
	*BaseBalancer
	conns map[string]*atomic.Int64
}

func NewLeastConnectionsBalancer(hosts map[string]bool) Balancer {
	b := &LeastConnectionsBalancer{
		BaseBalancer: newBaseBalancer(hosts),
		conns:        make(map[string]*atomic.Int64, len(hosts)),
	}
	for _, host := range b.hostList {
		b.conns[host] = &atomic.Int64{}
	}
	return b
}

func (lc *LeastConnectionsBalancer) Balance() (string, error) {
	lc.BaseBalancer.Lock()
	defer lc.BaseBalancer.Unlock()

	if len(lc.hostList) == 0 {
		return "", ErrNoHost
	}

	best := ""
	bestConns := int64(math.MaxInt64)

	for _, host := range lc.hostList {
		if !lc.hosts[host] {
			continue
		}
		c := lc.conns[host].Load()
		if c < bestConns {
			bestConns = c
			best = host
		}
	}

	if best == "" {
		return "", ErrNoHost
	}

	lc.conns[best].Add(1)
	return best, nil
}

// Release decrements the active-connection counter for host.
// It is safe to call concurrently without holding the balancer lock.
func (lc *LeastConnectionsBalancer) Release(host string) {
	if c, ok := lc.conns[host]; ok {
		c.Add(-1)
	}
}
