package middleware

// Let's start with a dummy implementation where each counter is reset at the second
// FIXME: Eventually upgrade implementation

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type RateLimiterConfig struct {
	// Starting with a fixed time window of a second for now
	Rqs uint `json:"rqs"`

	counter map[string]uint
}

// FIXME: Context
func (rl *RateLimiterConfig) Initialize(context context.Context) {
	go rl.resetter(time.NewTicker(time.Second))
	rl.counter = make(map[string]uint)
}

func (rl *RateLimiterConfig) Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientId := readUserIP(r)
		fmt.Println(clientId)
		if rl.exceedes(clientId) {
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}

		rl.counter[clientId] += 1

		next.ServeHTTP(w, r)
	}
}

func (rl *RateLimiterConfig) exceedes(client string) bool {
	numRequests, ok := rl.counter[client]
	if !ok {
		return false
	}
	return numRequests > rl.Rqs
}

func (rl *RateLimiterConfig) resetter(ticker *time.Ticker) {
	// Each counter is going to need a lock
	for range ticker.C {
		for key := range rl.counter {
			rl.counter[key] = 0
		}
	}
}

// Not related
// Temporary: From: https://stackoverflow.com/questions/27234861/correct-way-of-getting-clients-ip-addresses-from-http-request
func readUserIP(r *http.Request) string {
	IPAddress := r.Header.Get("X-Real-Ip")
	if IPAddress == "" {
		IPAddress = r.Header.Get("X-Forwarded-For")
	}
	if IPAddress == "" {
		IPAddress = r.RemoteAddr
	}
	return IPAddress
}
