package middleware

// Let's start with a dummy implementation where each counter is reset at the second
// FIXME: Eventually upgrade implementation (Working on it)

import (
	"context"
	"net/http"
	"sync"
	"time"
)

const (
	DEFAULT_MAX_REQUESTS       = 100
	DEFAULT_TIME_FRAME_SECONDS = 20
)

type RateLimiterConfig struct {
	MaxRequests      uint `json:"max_requests"`
	TimeFrameSeconds uint `json:"time_frame_seconds"` // Timeframe?

	counter map[string]*ClientRequestsCounter
}

type ClientRequestsCounter struct {
	requests           []int64
	oldestReqTimestamp uint64
	sync.Mutex
}

func NewClientRequestsCounter() *ClientRequestsCounter {
	return &ClientRequestsCounter{
		requests: make([]int64, 0),
	}
}

func (c *ClientRequestsCounter) incr(requestTimestamp int64) {
	c.Lock()
	defer c.Unlock()
	c.requests = append(c.requests, requestTimestamp)
}

func (c *ClientRequestsCounter) NumReqsInFrame(requestTime time.Time, timeframe time.Duration) int {
	c.Lock()
	defer c.Unlock()
	timeFrameStart := requestTime.Add(-timeframe).Unix()
	var outOfTimeFrame uint64
	for {
		if len(c.requests) > int(outOfTimeFrame) && c.requests[outOfTimeFrame] < timeFrameStart {
			outOfTimeFrame += 1
		} else {
			c.requests = c.requests[outOfTimeFrame:]
			break
		}
	}

	return len(c.requests)
}

func (rl *RateLimiterConfig) Initialize(context context.Context) {
	if rl.MaxRequests == 0 {
		rl.MaxRequests = DEFAULT_MAX_REQUESTS
	}
	if rl.TimeFrameSeconds == 0 {
		rl.TimeFrameSeconds = DEFAULT_TIME_FRAME_SECONDS
	}
	rl.counter = make(map[string]*ClientRequestsCounter)
}

func (rl *RateLimiterConfig) Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestTime := time.Now()
		clientAddr := readClientIpAddr(r)
		clientCounter, insert := rl.counter[clientAddr]
		if !insert {
			clientCounter = NewClientRequestsCounter()
			rl.counter[clientAddr] = clientCounter
		} else {
			if clientCounter.NumReqsInFrame(requestTime, time.Duration(rl.TimeFrameSeconds)*time.Second) >= int(rl.MaxRequests) {
				http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				return
			}
		}
		clientCounter.incr(requestTime.Unix())

		next.ServeHTTP(w, r)
	}
}

// NOTE: Helper function
// Temporary: From: https://stackoverflow.com/questions/27234861/correct-way-of-getting-clients-ip-addresses-from-http-request
func readClientIpAddr(r *http.Request) string {
	IPAddress := r.Header.Get("X-Real-Ip")
	if IPAddress == "" {
		IPAddress = r.Header.Get("X-Forwarded-For")
	}
	if IPAddress == "" {
		IPAddress = r.RemoteAddr
	}
	return IPAddress
}
