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

func initRateLimiterForTest(t *testing.T, cfg *RateLimiter) {
	t.Helper()

	ctx := ContextWithLogger(context.Background(), testLogger())
	if err := cfg.Init(ctx); err != nil {
		t.Fatalf("init rate limiter: %v", err)
	}
	t.Cleanup(func() { _ = cfg.Close() })
}

func okHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}

func TestRateLimiter_DefaultsAppliedInInit(t *testing.T) {
	rl := RateLimiter{}
	initRateLimiterForTest(t, &rl)

	if rl.MaxRequests != defaultMaxRequests {
		t.Fatalf("expected default max requests %d, got %d", defaultMaxRequests, rl.MaxRequests)
	}
	if rl.WindowSizeSecs != defaultWindowSizeSeconds {
		t.Fatalf("expected default timeframe %d, got %d", defaultWindowSizeSeconds, rl.WindowSizeSecs)
	}
	if rl.StaleClientTTLSecs != defaultStaleClientTTLSeconds {
		t.Fatalf("expected default stale ttl %d, got %d", defaultStaleClientTTLSeconds, rl.StaleClientTTLSecs)
	}
	if rl.ProxyHeaderMaxForwards != 5 {
		t.Fatalf("expected default proxy max forwards 5, got %d", rl.ProxyHeaderMaxForwards)
	}
}

func TestRateLimiter_AllowsUnderLimitThenBlocksAtLimit(t *testing.T) {
	rl := RateLimiter{
		MaxRequests:    2,
		WindowSizeSecs: 1,
	}
	initRateLimiterForTest(t, &rl)
	h := rl.Exec(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:1234"

	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req)
	if rr1.Code != http.StatusOK {
		t.Fatalf("request 1 expected 200, got %d", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("request 2 expected 200, got %d", rr2.Code)
	}

	rr3 := httptest.NewRecorder()
	h.ServeHTTP(rr3, req)
	if rr3.Code != http.StatusTooManyRequests {
		t.Fatalf("request 3 expected 429, got %d", rr3.Code)
	}
}

func TestRateLimiter_ResetsAfterTimeframe(t *testing.T) {
	rl := RateLimiter{
		MaxRequests:    1,
		WindowSizeSecs: 1,
	}
	initRateLimiterForTest(t, &rl)
	h := rl.Exec(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.11:1234"

	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req)
	if rr1.Code != http.StatusOK {
		t.Fatalf("request 1 expected 200, got %d", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("request 2 expected 429, got %d", rr2.Code)
	}

	time.Sleep(1100 * time.Millisecond)

	rr3 := httptest.NewRecorder()
	h.ServeHTTP(rr3, req)
	if rr3.Code != http.StatusOK {
		t.Fatalf("request 3 after timeframe expected 200, got %d", rr3.Code)
	}
}

func TestRateLimiter_PerClientIsolation(t *testing.T) {
	rl := RateLimiter{
		MaxRequests:    1,
		WindowSizeSecs: 2,
	}
	initRateLimiterForTest(t, &rl)
	h := rl.Exec(okHandler())

	reqA := httptest.NewRequest(http.MethodGet, "/", nil)
	reqA.RemoteAddr = "198.51.100.1:1111"

	reqB := httptest.NewRequest(http.MethodGet, "/", nil)
	reqB.RemoteAddr = "198.51.100.2:2222"

	rrA1 := httptest.NewRecorder()
	h.ServeHTTP(rrA1, reqA)
	if rrA1.Code != http.StatusOK {
		t.Fatalf("client A req1 expected 200, got %d", rrA1.Code)
	}

	rrA2 := httptest.NewRecorder()
	h.ServeHTTP(rrA2, reqA)
	if rrA2.Code != http.StatusTooManyRequests {
		t.Fatalf("client A req2 expected 429, got %d", rrA2.Code)
	}

	rrB1 := httptest.NewRecorder()
	h.ServeHTTP(rrB1, reqB)
	if rrB1.Code != http.StatusOK {
		t.Fatalf("client B req1 expected 200, got %d", rrB1.Code)
	}
}

func TestRateLimiter_ConcurrentBurstDoesNotExceedMax(t *testing.T) {
	const (
		maxReqs      = 10
		totalWorkers = 200
	)

	rl := RateLimiter{
		MaxRequests:    maxReqs,
		WindowSizeSecs: 2,
	}
	initRateLimiterForTest(t, &rl)
	h := rl.Exec(okHandler())

	var okCount int64
	var tooManyCount int64
	var wg sync.WaitGroup
	wg.Add(totalWorkers)

	for range totalWorkers {
		go func() {
			defer wg.Done()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "192.0.2.55:9999"

			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

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
	rl := RateLimiter{
		MaxRequests:    1,
		WindowSizeSecs: 2,
	}
	initRateLimiterForTest(t, &rl)
	h := rl.Exec(okHandler())

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "10.0.0.1:4444"
	req1.Header.Set("X-Forwarded-For", "203.0.113.99")

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "10.0.0.1:5555"
	req2.Header.Set("X-Forwarded-For", "198.51.100.88")

	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("req1 expected 200, got %d", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	// Same RemoteAddr IP, so should count against same bucket when not trusting proxy headers.
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("req2 expected 429, got %d", rr2.Code)
	}
}

func TestRateLimiter_TrustProxyHeadersUsesXForwardedFor(t *testing.T) {
	rl := RateLimiter{
		MaxRequests:       1,
		WindowSizeSecs:    2,
		TrustProxyHeaders: true,
	}
	initRateLimiterForTest(t, &rl)
	h := rl.Exec(okHandler())

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "10.0.0.1:4444"
	req1.Header.Set("X-Forwarded-For", "203.0.113.1, 10.0.0.1")

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "10.0.0.1:5555"
	req2.Header.Set("X-Forwarded-For", "198.51.100.2, 10.0.0.1")

	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("req1 expected 200, got %d", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	// Different XFF client IPs should be isolated when trust_proxy_headers is enabled.
	if rr2.Code != http.StatusOK {
		t.Fatalf("req2 expected 200, got %d", rr2.Code)
	}
}

func TestRateLimiter_EvictsStaleClients(t *testing.T) {
	rl := RateLimiter{
		MaxRequests:        2,
		WindowSizeSecs:     2,
		StaleClientTTLSecs: 1,
	}
	initRateLimiterForTest(t, &rl)

	// Seed two clients.
	rl.counters.Store("client-old", &clientCounter{
		windowStart: time.Now().Add(-10 * time.Second),
		lastSeen:    time.Now().Add(-10 * time.Second),
	})
	rl.clientCount.Add(1)
	rl.counters.Store("client-fresh", &clientCounter{
		windowStart: time.Now(),
		lastSeen:    time.Now(),
	})
	rl.clientCount.Add(1)

	rl.evictStaleClients()

	_, oldExists := rl.counters.Load("client-old")
	_, freshExists := rl.counters.Load("client-fresh")

	if oldExists {
		t.Fatalf("expected stale client to be evicted")
	}
	if !freshExists {
		t.Fatalf("expected fresh client to remain")
	}
}

// TestRateLimiter_PrevWindowWeighting verifies that, after the first window
// fills up, the second window admits a number of requests proportional to how
// far we are into it (i.e. the previous window's count is weighted down by the
// fraction of the current window already elapsed).
func TestRateLimiter_PrevWindowWeighting(t *testing.T) {
	const (
		maxReqs = 10
		window  = 1 * time.Second
	)

	rl := RateLimiter{
		MaxRequests:    maxReqs,
		WindowSizeSecs: uint(window / time.Second),
	}
	initRateLimiterForTest(t, &rl)
	h := rl.Exec(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.42:1234"

	// Fill window 1.
	for i := range maxReqs {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("W1 req %d expected 200, got %d", i+1, rr.Code)
		}
	}

	// Confirm the next request in window 1 is blocked.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("W1 over-limit expected 429, got %d", rr.Code)
	}

	// Land roughly halfway into window 2 so the weighted prev count is ~5,
	// leaving room for ~5 more before the rolling estimate hits the limit.
	time.Sleep(window + window/2)

	var allowedW2 int
	for range maxReqs * 2 {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		switch rr.Code {
		case http.StatusOK:
			allowedW2++
		case http.StatusTooManyRequests:
			// Expected once the weighted estimate catches up.
		default:
			t.Fatalf("unexpected status %d in W2", rr.Code)
		}
	}

	// Allow generous bounds for sleep jitter; the point is that admissions are
	// neither full (max) nor zero, but proportional to remaining capacity.
	if allowedW2 < 3 || allowedW2 > 7 {
		t.Fatalf("expected ~5 admissions into W2, got %d", allowedW2)
	}
}
