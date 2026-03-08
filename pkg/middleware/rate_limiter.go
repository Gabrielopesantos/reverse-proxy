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

const (
	DEFAULT_MAX_REQUESTS             = 100
	DEFAULT_TIME_FRAME_SECONDS       = 20
	DEFAULT_STALE_CLIENT_TTL_SECONDS = 300
)

type RateLimiterConfig struct {
	MaxReqs                uint `yaml:"max_requests"`
	TimeFrameSecs          uint `yaml:"time_frame_seconds"`
	StaleClientTTLSeconds  uint `yaml:"stale_client_ttl_seconds,omitempty"`
	TrustProxyHeaders      bool `yaml:"trust_proxy_headers,omitempty"`
	ProxyHeaderMaxForwards int  `yaml:"proxy_header_max_forwards,omitempty"`

	counter     map[string]*ClientRequestsCounter
	counterLock sync.RWMutex
	logger      *slog.Logger

	stopCleanup chan struct{}
	cleanupOnce sync.Once
}

type ClientRequestsCounter struct {
	reqsTimestamps []time.Time
	lastSeen       time.Time
	sync.Mutex
}

func NewClientRequestsCounter() *ClientRequestsCounter {
	return &ClientRequestsCounter{
		reqsTimestamps: make([]time.Time, 0),
		lastSeen:       time.Now(),
	}
}

// allow performs an atomic rate-limit decision for a client:
// prune old timestamps, check current in-window count, and append this request
// if allowed. Returns true when request should proceed.
func (c *ClientRequestsCounter) allow(now time.Time, timeframe time.Duration, max uint) bool {
	c.Lock()
	defer c.Unlock()

	windowStart := now.Add(-timeframe)

	// Prune old entries.
	pruneIdx := 0
	for pruneIdx < len(c.reqsTimestamps) && c.reqsTimestamps[pruneIdx].Before(windowStart) {
		pruneIdx++
	}
	if pruneIdx > 0 {
		c.reqsTimestamps = c.reqsTimestamps[pruneIdx:]
	}

	if uint(len(c.reqsTimestamps)) >= max {
		c.lastSeen = now
		return false
	}

	c.reqsTimestamps = append(c.reqsTimestamps, now)
	c.lastSeen = now
	return true
}

func (c *ClientRequestsCounter) inWindow(now time.Time, timeframe time.Duration) int {
	c.Lock()
	defer c.Unlock()

	windowStart := now.Add(-timeframe)
	pruneIdx := 0
	for pruneIdx < len(c.reqsTimestamps) && c.reqsTimestamps[pruneIdx].Before(windowStart) {
		pruneIdx++
	}
	if pruneIdx > 0 {
		c.reqsTimestamps = c.reqsTimestamps[pruneIdx:]
	}
	return len(c.reqsTimestamps)
}

func (c *ClientRequestsCounter) isStale(now time.Time, ttl time.Duration) bool {
	c.Lock()
	defer c.Unlock()
	return now.Sub(c.lastSeen) > ttl
}

func (rl *RateLimiterConfig) Init(ctx context.Context) error {
	rl.logger = LoggerFromContext(ctx)

	if rl.MaxReqs == 0 {
		rl.MaxReqs = DEFAULT_MAX_REQUESTS
	}
	if rl.TimeFrameSecs == 0 {
		rl.TimeFrameSecs = DEFAULT_TIME_FRAME_SECONDS
	}
	if rl.StaleClientTTLSeconds == 0 {
		rl.StaleClientTTLSeconds = DEFAULT_STALE_CLIENT_TTL_SECONDS
	}
	if rl.ProxyHeaderMaxForwards <= 0 {
		rl.ProxyHeaderMaxForwards = 5
	}

	rl.counter = make(map[string]*ClientRequestsCounter)

	// Best-effort background cleanup to prevent unbounded growth.
	rl.stopCleanup = make(chan struct{})
	go rl.cleanupLoop()

	return nil
}

func (rl *RateLimiterConfig) Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		clientAddr := rl.clientIP(r)

		clientCounter := rl.getOrCreateClientCounter(clientAddr)

		allowed := clientCounter.allow(now, time.Duration(rl.TimeFrameSecs)*time.Second, rl.MaxReqs)
		if !allowed {
			inWindow := clientCounter.inWindow(now, time.Duration(rl.TimeFrameSecs)*time.Second)
			rl.logger.Debug(
				"rate_limiter_blocked",
				"client_addr", clientAddr,
				"allowed", false,
				"in_window", inWindow,
				"max_requests", rl.MaxReqs,
				"time_frame_seconds", rl.TimeFrameSecs,
			)
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}

		inWindow := clientCounter.inWindow(now, time.Duration(rl.TimeFrameSecs)*time.Second)
		rl.logger.Debug(
			"rate_limiter_allowed",
			"client_addr", clientAddr,
			"allowed", true,
			"in_window", inWindow,
			"max_requests", rl.MaxReqs,
			"time_frame_seconds", rl.TimeFrameSecs,
		)

		next.ServeHTTP(w, r)
	}
}

func (rl *RateLimiterConfig) cleanupLoop() {
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

func (rl *RateLimiterConfig) Stop() {
	rl.cleanupOnce.Do(func() {
		if rl.stopCleanup != nil {
			close(rl.stopCleanup)
		}
	})
}

func (rl *RateLimiterConfig) evictStaleClients() {
	now := time.Now()
	ttl := time.Duration(rl.StaleClientTTLSeconds) * time.Second

	rl.counterLock.Lock()
	defer rl.counterLock.Unlock()

	for client, counter := range rl.counter {
		if counter.isStale(now, ttl) {
			delete(rl.counter, client)
		}
	}
}

func (rl *RateLimiterConfig) getOrCreateClientCounter(clientAddr string) *ClientRequestsCounter {
	rl.counterLock.RLock()
	clientCounter, exists := rl.counter[clientAddr]
	rl.counterLock.RUnlock()
	if exists {
		return clientCounter
	}

	rl.counterLock.Lock()
	defer rl.counterLock.Unlock()

	// Double-check after taking write lock.
	if clientCounter, exists = rl.counter[clientAddr]; exists {
		return clientCounter
	}

	clientCounter = NewClientRequestsCounter()
	rl.counter[clientAddr] = clientCounter
	return clientCounter
}

func (rl *RateLimiterConfig) clientIP(r *http.Request) string {
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
			// Handle potential host:port formats conservatively.
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
