package balancer

import "math/rand"

// A Balancer selects which target host is going to be consumed based on the
// logic defined
//
// Balance should return URL of the host that is going to be requests
type Balancer interface {
	// These methods will be used for checking alives
	//Add(string)
	//Remove(string)
	Balance() string
}

type RandomBalancer struct {
	Hosts []string
}

func NewRandomBalancer(hosts []string) Balancer {
	return &RandomBalancer{
		Hosts: hosts,
	}
}

func (rb *RandomBalancer) Balance() string {
	return rb.Hosts[rand.Intn(len(rb.Hosts))]
}
