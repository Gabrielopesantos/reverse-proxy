package middleware

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

// NOTE: Eventually accept string as TimeFrame, e.g. "15m", "1h"
type RateLimiterConfig struct {
	MaxReqs       uint `json:"max_requests"`
	TimeFrameSecs uint `json:"time_frame_seconds"`

	counter map[string]*ClientRequestsCounter
}

type ClientRequestsCounter struct {
	reqsTimestamps []int64
	sync.Mutex
}

func NewClientRequestsCounter() *ClientRequestsCounter {
	return &ClientRequestsCounter{
		reqsTimestamps: make([]int64, 0),
	}
}

func (c *ClientRequestsCounter) incr(reqTimestamp int64) {
	c.Lock()
	defer c.Unlock()
	c.reqsTimestamps = append(c.reqsTimestamps, reqTimestamp)
}

func (c *ClientRequestsCounter) ReqsInFrame(reqTime time.Time, timeframe time.Duration) int {
	c.Lock()
	defer c.Unlock()
	timeFrameStart := reqTime.Add(-timeframe).Unix()
	var outOfTimeFrame uint64
	for {
		if len(c.reqsTimestamps) > int(outOfTimeFrame) && c.reqsTimestamps[outOfTimeFrame] < timeFrameStart {
			outOfTimeFrame += 1
		} else {
			c.reqsTimestamps = c.reqsTimestamps[outOfTimeFrame:]
			break
		}
	}

	return len(c.reqsTimestamps)
}

func (rl *RateLimiterConfig) Init(ctx context.Context) error {
	if rl.MaxReqs == 0 {
		rl.MaxReqs = DEFAULT_MAX_REQUESTS
	}
	if rl.TimeFrameSecs == 0 {
		rl.TimeFrameSecs = DEFAULT_TIME_FRAME_SECONDS
	}
	rl.counter = make(map[string]*ClientRequestsCounter)
	return nil
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
			if clientCounter.ReqsInFrame(requestTime, time.Duration(rl.TimeFrameSecs)*time.Second) >= int(rl.MaxReqs) {
				http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				return
			}
		}
		clientCounter.incr(requestTime.Unix())

		next.ServeHTTP(w, r)
	}
}

// NOTE: Temporary helper function
// From: https://stackoverflow.com/questions/27234861/correct-way-of-getting-clients-ip-addresses-from-http-request
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
