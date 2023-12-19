package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"
)

const (
	ASSET_CACHING_DEFAULT_VALUE = "0s"
)

type CacheControlConfig struct {
	Duration string `json:"duration"`

	cache        sync.Map
	durationTime time.Duration
}

type cacheControlHeader struct {
	NoCache       bool
	NoStore       bool
	MaxAgeSeconds uint
}

func (cc *CacheControlConfig) Init(ctx context.Context) error {
	timeDuration, err := time.ParseDuration(cc.Duration)
	if err != nil {
		return err
	}
	cc.durationTime = timeDuration
	cc.cache = sync.Map{}

	return nil
}

func (cc *CacheControlConfig) Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// cacheControlHeaderValue := r.Header.Get("Cache-Control")
		next.ServeHTTP(w, r)
	}
}
