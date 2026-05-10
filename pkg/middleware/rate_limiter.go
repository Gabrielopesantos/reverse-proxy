package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

func init() {
	RegisterYAML(RATE_LIMITER, func() *RateLimiter { return &RateLimiter{} })
}

const (
	DEFAULT_MAX_REQUESTS             = 100
	DEFAULT_WINDOW_SIZE_SECONDS      = 20
	DEFAULT_STALE_CLIENT_TTL_SECONDS = 300
)

type RateLimiter struct {
	MaxRequests           uint `yaml:"max_requests"`
	WindowSizeSeconds     uint `yaml:"window_size_seconds"`
	StaleClientTTLSeconds uint `yaml:"stale_client_ttl_seconds,omitempty"`

	// TrustProxyHeaders enables trusting the X-Forwarded-For header from the proxy.
	TrustProxyHeaders      bool `yaml:"trust_proxy_headers,omitempty"`
	ProxyHeaderMaxForwards int  `yaml:"proxy_header_max_forwards,omitempty"`

	counters     map[string]*clientCounter
	countersLock sync.RWMutex

	logger *slog.Logger

	stopCleanup chan struct{}
	cleanupOnce sync.Once
}

// clientCounter implements a sliding window counter we
// track per client the request count of the previous and
// current fixed-size windows, and approximate the rolling
// count by weighting the previous window by how far we are
// into the current one.
type clientCounter struct {
	windowStart time.Time
	prev        uint
	curr        uint
	lastSeen    time.Time
	mu          sync.Mutex
}

func newClientCounter(now time.Time) *clientCounter {
	return &clientCounter{
		windowStart: now,
		lastSeen:    now,
	}
}

// allow rolls the windows to `now`, computes the weighted estimate, and
// increments the current-window counter when admission is granted.
func (c *clientCounter) allow(now time.Time, window time.Duration, max uint) (bool, uint) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.roll(now, window)

	elapsed := now.Sub(c.windowStart)
	if elapsed < 0 {
		elapsed = 0
	}
	weight := 1 - float64(elapsed)/float64(window)
	estimated := uint(float64(c.prev)*weight) + c.curr

	c.lastSeen = now
	if estimated >= max {
		return false, estimated
	}
	c.curr++
	return true, estimated + 1
}

func (c *clientCounter) roll(now time.Time, window time.Duration) {
	delta := now.Sub(c.windowStart)
	switch {
	case delta >= 2*window:
		c.prev = 0
		c.curr = 0
		c.windowStart = now
	case delta >= window:
		c.prev = c.curr
		c.curr = 0
		c.windowStart = c.windowStart.Add(window)
	}
}

func (c *clientCounter) isStale(now time.Time, ttl time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return now.Sub(c.lastSeen) > ttl
}

func (rl *RateLimiter) Init(ctx context.Context) error {
	rl.logger = LoggerFromContext(ctx)

	if rl.MaxRequests == 0 {
		rl.MaxRequests = DEFAULT_MAX_REQUESTS
	}
	if rl.WindowSizeSeconds == 0 {
		rl.WindowSizeSeconds = DEFAULT_WINDOW_SIZE_SECONDS
	}
	if rl.StaleClientTTLSeconds == 0 {
		rl.StaleClientTTLSeconds = DEFAULT_STALE_CLIENT_TTL_SECONDS
	}
	if rl.ProxyHeaderMaxForwards <= 0 {
		rl.ProxyHeaderMaxForwards = 5
	}

	rl.counters = make(map[string]*clientCounter)

	rl.stopCleanup = make(chan struct{})
	go rl.cleanupLoop()

	return nil
}

func (rl *RateLimiter) Exec(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		clientAddr := rl.clientIP(r)

		window := time.Duration(rl.WindowSizeSeconds) * time.Second
		counter := rl.getOrCreateClientCounter(clientAddr, now)

		allowed, estimated := counter.allow(now, window, rl.MaxRequests)
		rl.logger.Debug("request", "client_addr", clientAddr, "allowed", allowed, "estimated", estimated)
		if !allowed {
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(time.Duration(rl.StaleClientTTLSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.evictStaleClients()
		case <-rl.stopCleanup:
			return
		}
	}
}

func (rl *RateLimiter) Close() error {
	rl.cleanupOnce.Do(func() {
		if rl.stopCleanup != nil {
			close(rl.stopCleanup)
		}
	})
	return nil
}

func (rl *RateLimiter) evictStaleClients() {
	now := time.Now()
	ttl := time.Duration(rl.StaleClientTTLSeconds) * time.Second

	rl.countersLock.Lock()
	defer rl.countersLock.Unlock()

	for client, counter := range rl.counters {
		if counter.isStale(now, ttl) {
			delete(rl.counters, client)
		}
	}
}

func (rl *RateLimiter) getOrCreateClientCounter(clientAddr string, now time.Time) *clientCounter {
	rl.countersLock.RLock()
	c, exists := rl.counters[clientAddr]
	rl.countersLock.RUnlock()
	if exists {
		return c
	}

	rl.countersLock.Lock()
	defer rl.countersLock.Unlock()

	if c, exists = rl.counters[clientAddr]; exists {
		return c
	}

	c = newClientCounter(now)
	rl.counters[clientAddr] = c
	return c
}

func (rl *RateLimiter) clientIP(r *http.Request) string {
	if rl.TrustProxyHeaders {
		if ip := parseClientIPFromHeaders(r, rl.ProxyHeaderMaxForwards); ip != "" {
			return ip
		}
	}
	return remoteAddrIP(r.RemoteAddr)
}

func parseClientIPFromHeaders(r *http.Request, maxForwards int) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		limit := len(parts)
		if maxForwards > 0 && limit > maxForwards {
			limit = maxForwards
		}
		for i := 0; i < limit; i++ {
			candidate := strings.TrimSpace(parts[i])
			if candidate == "" {
				continue
			}
			if host, _, err := net.SplitHostPort(candidate); err == nil {
				candidate = host
			}
			if net.ParseIP(candidate) != nil {
				return candidate
			}
		}
	}

	xri := strings.TrimSpace(r.Header.Get("X-Real-IP"))
	if xri != "" {
		if host, _, err := net.SplitHostPort(xri); err == nil {
			xri = host
		}
		if net.ParseIP(xri) != nil {
			return xri
		}
	}

	return ""
}

func remoteAddrIP(remoteAddr string) string {
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return ip
}

func clientIP(r *http.Request) string {
	return remoteAddrIP(r.RemoteAddr)
}
