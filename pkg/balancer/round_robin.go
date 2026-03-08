package balancer

// RoundRobinBalancer is a balancer that selects a host in a round-robin fashion.
type RoundRobinBalancer struct {
	*BaseBalancer
	currentHostIndex int
}

func NewRoundRobinBalancer(hosts map[string]bool) Balancer {
	return &RoundRobinBalancer{
		BaseBalancer:     newBaseBalancer(hosts),
		currentHostIndex: 0,
	}
}

func (rr *RoundRobinBalancer) Balance() (string, error) {
	rr.BaseBalancer.Lock()
	defer rr.BaseBalancer.Unlock()

	n := len(rr.hostList)
	if n == 0 {
		return "", ErrNoHost
	}

	for i := 0; i < n; i++ {
		host := rr.hostList[rr.currentHostIndex]
		rr.currentHostIndex = (rr.currentHostIndex + 1) % n
		if rr.hosts[host] {
			return host, nil
		}
	}

	return "", ErrNoHost
}
