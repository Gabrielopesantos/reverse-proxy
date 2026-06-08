package balancer

import (
	"math"
	"net/http"
	"sync"
)

func init() {
	Register(WeightedRoundRobin, func(hosts map[string]bool, weights map[string]int) Balancer {
		return NewWeightedRoundRobinBalancer(hosts, weights)
	})
}

// WeightedRoundRobinBalancer implements the Nginx smooth weighted round-robin
// algorithm. A small mutex serialises currentWeights mutations; health reads
// are lock-free via the embedded BaseBalancer.
type WeightedRoundRobinBalancer struct {
	*BaseBalancer
	weights        []int
	currentWeights []int
	mu             sync.Mutex
}

func NewWeightedRoundRobinBalancer(hosts map[string]bool, weights map[string]int) Balancer {
	b := &WeightedRoundRobinBalancer{
		BaseBalancer:   newBaseBalancer(hosts),
		weights:        make([]int, len(hosts)),
		currentWeights: make([]int, len(hosts)),
	}
	for i, host := range b.hostList {
		w := 1
		if weights != nil {
			if wt, ok := weights[host]; ok && wt > 0 {
				w = wt
			}
		}
		b.weights[i] = w
	}
	return b
}

func (w *WeightedRoundRobinBalancer) BalanceFor(_ *http.Request) (string, error) {
	n := len(w.hostList)
	if n == 0 {
		return "", ErrNoHost
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	totalWeight := 0
	bestIdx := -1
	bestWeight := math.MinInt

	for i := range n {
		if !w.isHealthy(i) {
			continue
		}
		w.currentWeights[i] += w.weights[i]
		totalWeight += w.weights[i]
		if w.currentWeights[i] > bestWeight {
			bestWeight = w.currentWeights[i]
			bestIdx = i
		}
	}

	if bestIdx < 0 {
		return "", ErrNoHost
	}

	w.currentWeights[bestIdx] -= totalWeight
	return w.hostList[bestIdx], nil
}
