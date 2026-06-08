package balancer

import (
	"hash/fnv"
	"net"
	"net/http"
	"strings"
)

func init() {
	Register(IPHash, func(hosts map[string]bool, _ map[string]int) Balancer {
		return NewIPHashBalancer(hosts)
	})
}

// IPHashBalancer implements session affinity: the same client IP always maps to
// the same upstream (as long as it is healthy). Lock-free.
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
	n := len(ih.hostList)
	if n == 0 {
		return "", ErrNoHost
	}

	clientIP := extractClientIP(r)
	h := fnv.New32a()
	h.Write([]byte(clientIP))
	startIdx := int(h.Sum32()) % n

	for i := range n {
		idx := (startIdx + i) % n
		if ih.isHealthy(idx) {
			return ih.hostList[idx], nil
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
