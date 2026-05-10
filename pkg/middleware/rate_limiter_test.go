package middleware

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newRateLimiterForTest(t *testing.T, cfg RateLimiterConfig) *RateLimiterConfig {
	t.Helper()

	ctx := ContextWithLogger(context.Background(), testLogger())
	if err := cfg.Init(ctx); err != nil {
		t.Fatalf("init rate limiter: %v", err)
	}
	t.Cleanup(cfg.Stop)
	return &cfg
}

func okHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}

func TestRateLimiter_DefaultsAppliedInInit(t *testing.T) {
	rl := newRateLimiterForTest(t, RateLimiterConfig{})

	if rl.MaxReqs != DEFAULT_MAX_REQUESTS {
		t.Fatalf("expected default max requests %d, got %d", DEFAULT_MAX_REQUESTS, rl.MaxReqs)
	}
	if rl.TimeFrameSecs != DEFAULT_TIME_FRAME_SECONDS {
		t.Fatalf("expected default timeframe %d, got %d", DEFAULT_TIME_FRAME_SECONDS, rl.TimeFrameSecs)
	}
	if rl.StaleClientTTLSeconds != DEFAULT_STALE_CLIENT_TTL_SECONDS {
		t.Fatalf("expected default stale ttl %d, got %d", DEFAULT_STALE_CLIENT_TTL_SECONDS, rl.StaleClientTTLSeconds)
	}
	if rl.ProxyHeaderMaxForwards != 5 {
		t.Fatalf("expected default proxy max forwards 5, got %d", rl.ProxyHeaderMaxForwards)
	}
}

func TestRateLimiter_AllowsUnderLimitThenBlocksAtLimit(t *testing.T) {
	rl := newRateLimiterForTest(t, RateLimiterConfig{
		MaxReqs:       2,
		TimeFrameSecs: 1,
	})
	h := rl.Exec(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:1234"

	rr1 := httptest.NewRecorder()
	h(rr1, req)
	if rr1.Code != http.StatusOK {
		t.Fatalf("request 1 expected 200, got %d", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	h(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("request 2 expected 200, got %d", rr2.Code)
	}

	rr3 := httptest.NewRecorder()
	h(rr3, req)
	if rr3.Code != http.StatusTooManyRequests {
		t.Fatalf("request 3 expected 429, got %d", rr3.Code)
	}
}

func TestRateLimiter_ResetsAfterTimeframe(t *testing.T) {
	rl := newRateLimiterForTest(t, RateLimiterConfig{
		MaxReqs:       1,
		TimeFrameSecs: 1,
	})
	h := rl.Exec(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.11:1234"

	rr1 := httptest.NewRecorder()
	h(rr1, req)
	if rr1.Code != http.StatusOK {
		t.Fatalf("request 1 expected 200, got %d", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	h(rr2, req)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("request 2 expected 429, got %d", rr2.Code)
	}

	time.Sleep(1100 * time.Millisecond)

	rr3 := httptest.NewRecorder()
	h(rr3, req)
	if rr3.Code != http.StatusOK {
		t.Fatalf("request 3 after timeframe expected 200, got %d", rr3.Code)
	}
}

func TestRateLimiter_PerClientIsolation(t *testing.T) {
	rl := newRateLimiterForTest(t, RateLimiterConfig{
		MaxReqs:       1,
		TimeFrameSecs: 2,
	})
	h := rl.Exec(okHandler())

	reqA := httptest.NewRequest(http.MethodGet, "/", nil)
	reqA.RemoteAddr = "198.51.100.1:1111"

	reqB := httptest.NewRequest(http.MethodGet, "/", nil)
	reqB.RemoteAddr = "198.51.100.2:2222"

	rrA1 := httptest.NewRecorder()
	h(rrA1, reqA)
	if rrA1.Code != http.StatusOK {
		t.Fatalf("client A req1 expected 200, got %d", rrA1.Code)
	}

	rrA2 := httptest.NewRecorder()
	h(rrA2, reqA)
	if rrA2.Code != http.StatusTooManyRequests {
		t.Fatalf("client A req2 expected 429, got %d", rrA2.Code)
	}

	rrB1 := httptest.NewRecorder()
	h(rrB1, reqB)
	if rrB1.Code != http.StatusOK {
		t.Fatalf("client B req1 expected 200, got %d", rrB1.Code)
	}
}

func TestRateLimiter_ConcurrentBurstDoesNotExceedMax(t *testing.T) {
	const (
		maxReqs      = 10
		totalWorkers = 200
	)

	rl := newRateLimiterForTest(t, RateLimiterConfig{
		MaxReqs:       maxReqs,
		TimeFrameSecs: 2,
	})
	h := rl.Exec(okHandler())

	var okCount int64
	var tooManyCount int64
	var wg sync.WaitGroup
	wg.Add(totalWorkers)

	for i := 0; i < totalWorkers; i++ {
		go func() {
			defer wg.Done()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "192.0.2.55:9999"

			rr := httptest.NewRecorder()
			h(rr, req)

			switch rr.Code {
			case http.StatusOK:
				atomic.AddInt64(&okCount, 1)
			case http.StatusTooManyRequests:
				atomic.AddInt64(&tooManyCount, 1)
			default:
				t.Errorf("unexpected status code: %d", rr.Code)
			}
		}()
	}

	wg.Wait()

	if okCount > maxReqs {
		t.Fatalf("allowed requests exceeded max: got %d, max %d", okCount, maxReqs)
	}
	if okCount+tooManyCount != totalWorkers {
		t.Fatalf("unexpected total classified requests: got %d, expected %d", okCount+tooManyCount, totalWorkers)
	}
}

func TestRateLimiter_UsesRemoteAddrByDefault(t *testing.T) {
	rl := newRateLimiterForTest(t, RateLimiterConfig{
		MaxReqs:       1,
		TimeFrameSecs: 2,
	})
	h := rl.Exec(okHandler())

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "10.0.0.1:4444"
	req1.Header.Set("X-Forwarded-For", "203.0.113.99")

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "10.0.0.1:5555"
	req2.Header.Set("X-Forwarded-For", "198.51.100.88")

	rr1 := httptest.NewRecorder()
	h(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("req1 expected 200, got %d", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	h(rr2, req2)
	// Same RemoteAddr IP, so should count against same bucket when not trusting proxy headers.
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("req2 expected 429, got %d", rr2.Code)
	}
}

func TestRateLimiter_TrustProxyHeadersUsesXForwardedFor(t *testing.T) {
	rl := newRateLimiterForTest(t, RateLimiterConfig{
		MaxReqs:           1,
		TimeFrameSecs:     2,
		TrustProxyHeaders: true,
	})
	h := rl.Exec(okHandler())

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "10.0.0.1:4444"
	req1.Header.Set("X-Forwarded-For", "203.0.113.1, 10.0.0.1")

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "10.0.0.1:5555"
	req2.Header.Set("X-Forwarded-For", "198.51.100.2, 10.0.0.1")

	rr1 := httptest.NewRecorder()
	h(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("req1 expected 200, got %d", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	h(rr2, req2)
	// Different XFF client IPs should be isolated when trust_proxy_headers is enabled.
	if rr2.Code != http.StatusOK {
		t.Fatalf("req2 expected 200, got %d", rr2.Code)
	}
}

func TestRateLimiter_EvictsStaleClients(t *testing.T) {
	rl := newRateLimiterForTest(t, RateLimiterConfig{
		MaxReqs:               2,
		TimeFrameSecs:         2,
		StaleClientTTLSeconds: 1,
	})

	// Seed two clients.
	rl.counterLock.Lock()
	rl.counter["client-old"] = &ClientRequestsCounter{
		reqsTimestamps: []time.Time{time.Now().Add(-10 * time.Second)},
		lastSeen:       time.Now().Add(-10 * time.Second),
	}
	rl.counter["client-fresh"] = &ClientRequestsCounter{
		reqsTimestamps: []time.Time{time.Now()},
		lastSeen:       time.Now(),
	}
	rl.counterLock.Unlock()

	rl.evictStaleClients()

	rl.counterLock.RLock()
	_, oldExists := rl.counter["client-old"]
	_, freshExists := rl.counter["client-fresh"]
	rl.counterLock.RUnlock()

	if oldExists {
		t.Fatalf("expected stale client to be evicted")
	}
	if !freshExists {
		t.Fatalf("expected fresh client to remain")
	}
}
