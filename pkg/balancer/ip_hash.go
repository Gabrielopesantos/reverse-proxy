package balancer

import (
	"hash/fnv"
	"math/rand"
	"net"
	"net/http"
	"strings"
)

// IPHashBalancer implements session affinity: the same client IP always maps to
// the same upstream (as long as it is healthy). It implements RequestBalancer.
type IPHashBalancer struct {
	*BaseBalancer
}

func NewIPHashBalancer(hosts map[string]bool) Balancer {
	return &IPHashBalancer{
		BaseBalancer: newBaseBalancer(hosts),
	}
}

// BalanceFor hashes the client IP with FNV-32a and maps it to a host index.
// If the selected host is unhealthy the algorithm walks forward through the
// index until it finds a healthy one.
func (ih *IPHashBalancer) BalanceFor(r *http.Request) (string, error) {
	ih.BaseBalancer.Lock()
	defer ih.BaseBalancer.Unlock()

	n := len(ih.hostList)
	if n == 0 {
		return "", ErrNoHost
	}

	clientIP := extractClientIP(r)
	h := fnv.New32a()
	h.Write([]byte(clientIP))
	startIdx := int(h.Sum32()) % n

	for i := 0; i < n; i++ {
		host := ih.hostList[(startIdx+i)%n]
		if ih.hosts[host] {
			return host, nil
		}
	}

	return "", ErrNoHost
}

// Balance is the fallback for callers unaware of RequestBalancer; it picks a
// random healthy host.
func (ih *IPHashBalancer) Balance() (string, error) {
	ih.BaseBalancer.Lock()
	defer ih.BaseBalancer.Unlock()

	n := len(ih.hostList)
	if n == 0 {
		return "", ErrNoHost
	}

	start := rand.Intn(n)
	for i := 0; i < n; i++ {
		host := ih.hostList[(start+i)%n]
		if ih.hosts[host] {
			return host, nil
		}
	}

	return "", ErrNoHost
}

// extractClientIP returns the original client IP from the request, preferring
// X-Real-IP, then the first value of X-Forwarded-For, then RemoteAddr.
func extractClientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return strings.TrimSpace(ip)
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the leftmost address (original client)
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
