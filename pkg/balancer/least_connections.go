package balancer

import (
	"math"
	"net/http"
	"sync/atomic"
)

func init() {
	Register(LeastConnections, func(hosts map[string]bool, _ map[string]int) Balancer {
		return NewLeastConnectionsBalancer(hosts)
	})
}

// LeastConnectionsBalancer routes each request to the healthy upstream with
// the fewest active connections. Selection is lock-free: per-host counters
// are atomic and the host list is immutable after construction.
type LeastConnectionsBalancer struct {
	*BaseBalancer
	conns []atomic.Int64
}

func NewLeastConnectionsBalancer(hosts map[string]bool) Balancer {
	b := &LeastConnectionsBalancer{
		BaseBalancer: newBaseBalancer(hosts),
		conns:        make([]atomic.Int64, len(hosts)),
	}
	return b
}

func (lc *LeastConnectionsBalancer) BalanceFor(_ *http.Request) (string, error) {
	n := len(lc.hostList)
	if n == 0 {
		return "", ErrNoHost
	}

	bestIdx := -1
	bestConns := int64(math.MaxInt64)

	for i := range n {
		if !lc.isHealthy(i) {
			continue
		}
		c := lc.conns[i].Load()
		if c < bestConns {
			bestConns = c
			bestIdx = i
		}
	}

	if bestIdx < 0 {
		return "", ErrNoHost
	}

	lc.conns[bestIdx].Add(1)
	return lc.hostList[bestIdx], nil
}

// Release decrements the active-connection counter for host.
// Safe to call concurrently.
func (lc *LeastConnectionsBalancer) Release(host string) {
	if i, ok := lc.hostIdx[host]; ok {
		lc.conns[i].Add(-1)
	}
}
