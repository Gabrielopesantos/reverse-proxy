package middleware

// Let's start with a dummy implementation where each counter is reset at the second
// FIXME: Eventually upgrade implementation

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type RateLimiterConfig struct {
	// Starting with a fixed time window of a second for now
	Rqs uint `json:"rqs"`

	counter map[string]clientCount
}

type clientCount struct {
	numRequests uint
	*sync.Mutex
}

func (c *clientCount) reset() {
	c.Lock()
	defer c.Unlock()
	c.numRequests = 0
}

func (c *clientCount) incr() {
	c.Lock()
	defer c.Unlock()
	c.numRequests++
}

// FIXME: Context
func (rl *RateLimiterConfig) Initialize(context context.Context) {
	go rl.resetter(time.NewTicker(time.Second))
	rl.counter = make(map[string]clientCount)
}

func (rl *RateLimiterConfig) Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userId := readClientIpAddr(r)
		if rl.clientExceedsLimit(userId) {
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	}
}

func (rl *RateLimiterConfig) clientExceedsLimit(user string) bool {
	count, ok := rl.counter[user]
	if !ok {
		return false
	}
	count.Lock()
	defer count.Unlock()

	return count.numRequests > rl.Rqs
}

func (rl *RateLimiterConfig) clientCountIncr(user string) {
	count := rl.counter[user]
	count.incr()
}

func (rl *RateLimiterConfig) resetter(ticker *time.Ticker) {
	// Each counter is going to need a lock
	for range ticker.C {
		for _, cc := range rl.counter {
			cc.reset()
		}
	}
}

// Not related
// Temporary: From: https://stackoverflow.com/questions/27234861/correct-way-of-getting-clients-ip-addresses-from-http-request
func readClientIpAddr(r *http.Request) string {
	// IPAddress := r.Header.Get("X-Real-Ip")
	// if IPAddress == "" {
	// 	IPAddress = r.Header.Get("X-Forwarded-For")
	// }
	// if IPAddress == "" {
	// 	IPAddress = r.RemoteAddr
	// }
	return r.RemoteAddr
}
