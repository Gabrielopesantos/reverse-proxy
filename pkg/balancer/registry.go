package balancer

// BalancerFactory creates a new Balancer from the host-health map and weights.
type BalancerFactory func(hosts map[string]bool, weights map[string]int) Balancer

var balancerRegistry = map[LoadBalancerStrategy]BalancerFactory{}

// Register associates a factory with a load-balancing strategy.
// Balancer implementations call this from init() to self-register.
func Register(strategy LoadBalancerStrategy, f BalancerFactory) {
	balancerRegistry[strategy] = f
}
