package balancer

import "math"

type weightedHost struct {
	weight        int
	currentWeight int
}

// WeightedRoundRobinBalancer implements the Nginx smooth weighted round-robin
// algorithm. Hosts with higher weights receive proportionally more traffic
// without clustering bias.
type WeightedRoundRobinBalancer struct {
	*BaseBalancer
	wHosts map[string]*weightedHost
}

func NewWeightedRoundRobinBalancer(hosts map[string]bool, weights map[string]int) Balancer {
	b := &WeightedRoundRobinBalancer{
		BaseBalancer: newBaseBalancer(hosts),
		wHosts:       make(map[string]*weightedHost, len(hosts)),
	}
	for _, host := range b.hostList {
		w := 1
		if weights != nil {
			if wt, ok := weights[host]; ok && wt > 0 {
				w = wt
			}
		}
		b.wHosts[host] = &weightedHost{weight: w}
	}
	return b
}

func (w *WeightedRoundRobinBalancer) Balance() (string, error) {
	w.BaseBalancer.Lock()
	defer w.BaseBalancer.Unlock()

	if len(w.hosts) == 0 {
		return "", ErrNoHost
	}

	totalWeight := 0
	best := ""
	bestWeight := math.MinInt

	for host, wh := range w.wHosts {
		if !w.hosts[host] {
			continue
		}
		wh.currentWeight += wh.weight
		totalWeight += wh.weight
		if wh.currentWeight > bestWeight {
			bestWeight = wh.currentWeight
			best = host
		}
	}

	if best == "" {
		return "", ErrNoHost
	}

	w.wHosts[best].currentWeight -= totalWeight
	return best, nil
}
