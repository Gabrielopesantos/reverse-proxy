package middleware

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	utils "github.com/gabrielopesantos/reverse-proxy/pkg/utilities/cache"
)

type CacheControlConfig struct {
	Duration     string `json:"duration"`
	durationTime time.Duration

	MaxItems uint `json:"max_items"`

	cache *utils.SizeLimitedCache
}

func (cc *CacheControlConfig) Init(ctx context.Context) error {
	timeDuration, err := time.ParseDuration(cc.Duration)
	if err != nil {
		return fmt.Errorf("cache_control: invalid duration %q: %w", cc.Duration, err)
	}
	cc.durationTime = timeDuration

	maxItems := cc.MaxItems
	if maxItems == 0 {
		maxItems = 200
	}
	cc.cache = utils.NewSizeLimitedCache(maxItems)

	return nil
}

func (cc *CacheControlConfig) Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only cache GET/HEAD responses.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}

		reqCC := parseRequestCacheControl(r.Header.Get("Cache-Control"))

		// no-store: never read or write cache.
		if reqCC.noStore {
			next.ServeHTTP(w, r)
			return
		}

		cacheKey := buildCacheKey(r)

		// no-cache: bypass cache read but still store the fresh response.
		if !reqCC.noCache {
			if cached := cc.cache.GetResponse(cacheKey); cached != nil {
				writeCachedResponse(w, cached)
				return
			}
		}

		// Capture the upstream response.
		crw := newCaptureResponseWriter(w)
		next.ServeHTTP(crw, r)

		// Respect upstream Cache-Control: no-store.
		respCC := parseResponseCacheControl(crw.Header().Get("Cache-Control"))
		if respCC.noStore {
			return
		}

		ttl := cc.durationTime
		if respCC.maxAge > 0 {
			ttl = time.Duration(respCC.maxAge) * time.Second
		}

		cc.cache.CacheResponse(cacheKey, &utils.CachedResponse{
			StatusCode: crw.statusCode,
			Headers:    map[string][]string(crw.Header().Clone()),
			Body:       crw.body.Bytes(),
			ExpiresAt:  time.Now().Add(ttl),
		})
	}
}

func writeCachedResponse(w http.ResponseWriter, cached *utils.CachedResponse) {
	for key, vals := range cached.Headers {
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}
	w.Header().Set("X-Cache", "HIT")
	w.WriteHeader(cached.StatusCode)
	_, _ = w.Write(cached.Body)
}

// captureResponseWriter buffers the response so it can be cached.
type captureResponseWriter struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func newCaptureResponseWriter(w http.ResponseWriter) *captureResponseWriter {
	return &captureResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (c *captureResponseWriter) WriteHeader(code int) {
	c.statusCode = code
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureResponseWriter) Write(b []byte) (int, error) {
	c.body.Write(b)
	return c.ResponseWriter.Write(b)
}

type cacheControlDirectives struct {
	noCache bool
	noStore bool
	maxAge  int // seconds; -1 means not set
}

func parseRequestCacheControl(header string) cacheControlDirectives {
	return parseCacheControlHeader(header)
}

func parseResponseCacheControl(header string) cacheControlDirectives {
	return parseCacheControlHeader(header)
}

func parseCacheControlHeader(header string) cacheControlDirectives {
	d := cacheControlDirectives{maxAge: -1}
	for _, directive := range strings.Split(header, ",") {
		directive = strings.TrimSpace(strings.ToLower(directive))
		switch {
		case directive == "no-cache":
			d.noCache = true
		case directive == "no-store":
			d.noStore = true
		case strings.HasPrefix(directive, "max-age="):
			if v, err := strconv.Atoi(strings.TrimPrefix(directive, "max-age=")); err == nil {
				d.maxAge = v
			}
		}
	}
	return d
}

func buildCacheKey(r *http.Request) [16]byte {
	key := fmt.Appendf(nil, "%s-%s-%s?%s", r.Host, r.Method, r.URL.Path, r.URL.RawQuery)
	return md5.Sum(key)
}
