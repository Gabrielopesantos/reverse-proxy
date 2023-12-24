package middleware

import (
	"context"
	"crypto/md5"
	"fmt"
	"net/http"
	"time"

	utils "github.com/gabrielopesantos/reverse-proxy/pkg/utilities"
)

const (
	ASSET_CACHING_DEFAULT_VALUE = "0s"
)

type CacheControlConfig struct {
	Duration string `json:"duration"`

	cache        *utils.SizeLimitedCache
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
	// NOTE: Size still doesn't have any meaning
	cc.cache = utils.NewSizeLimitedCache(200)

	return nil
}

func (cc *CacheControlConfig) Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cacheControlHeaderValue := r.Header.Get("Cache-Control")
		if cacheControlHeaderValue == "no-cache" {
			next.ServeHTTP(w, r)
		}

		// Check for cached value
		cacheResponseKey := buildCacheKey(r)
	}
}

func buildCacheKey(r *http.Request) [16]byte {
	unhashedKey := []byte(fmt.Sprintf("%s-%s-%s", r.Host, r.Method, r.URL.Path))
	return md5.Sum(unhashedKey)
}
