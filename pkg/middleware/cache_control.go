package middleware

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	utils "github.com/gabrielopesantos/reverse-proxy/pkg/utilities/cache"
)

func init() {
	RegisterYAML(TypeCacheControl, PhaseCache, func() *CacheControl { return &CacheControl{} })
}

// defaultMaxCacheBodyBytes caps the per-response buffer when MaxBodyBytes is
// not configured. 1 MiB keeps a single oversize response from anchoring tens
// of MB of heap waiting for eviction.
const (
	defaultMaxCacheBodyBytes = 1 << 20
	defaultMaxCacheItems     = 200
	cacheKeyBufInitialCap    = 256
)

type CacheControl struct {
	Duration     string `yaml:"duration"`
	durationTime time.Duration

	MaxItems     uint `yaml:"max_items"`
	MaxBodyBytes int  `yaml:"max_body_bytes,omitempty"`

	cache  *utils.SizeLimitedCache
	logger *slog.Logger
}

// bufferPool recycles capture buffers so cache-eligible responses don't
// allocate a fresh bytes.Buffer per request.
var bufferPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// keyBufPool recycles []byte slices used to assemble the pre-hash cache key.
var keyBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, cacheKeyBufInitialCap)
		return &b
	},
}

func (cc *CacheControl) Init(ctx context.Context) error {
	cc.logger = LoggerFromContext(ctx)
	timeDuration, err := time.ParseDuration(cc.Duration)
	if err != nil {
		return fmt.Errorf("cache_control: invalid duration %q: %w", cc.Duration, err)
	}
	cc.durationTime = timeDuration

	maxItems := cc.MaxItems
	if maxItems == 0 {
		maxItems = defaultMaxCacheItems
	}
	cc.cache = utils.NewSizeLimitedCache(maxItems)

	if cc.MaxBodyBytes <= 0 {
		cc.MaxBodyBytes = defaultMaxCacheBodyBytes
	}

	return nil
}

func (cc *CacheControl) Exec(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}

		reqCC := parseRequestCacheControl(r.Header.Get("Cache-Control"))

		if reqCC.noStore {
			next.ServeHTTP(w, r)
			return
		}

		cacheKey := buildCacheKey(r)

		if !reqCC.noCache {
			if cached := cc.cache.GetResponse(cacheKey); cached != nil {
				writeCachedResponse(w, cached)
				return
			}
		}

		buf := bufferPool.Get().(*bytes.Buffer)
		buf.Reset()
		crw := &captureResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			body:           buf,
			max:            cc.MaxBodyBytes,
		}
		next.ServeHTTP(crw, r)

		respCC := parseResponseCacheControl(crw.Header().Get("Cache-Control"))
		if respCC.noStore || crw.overflow {
			// Buffer is unusable for caching; recycle and skip.
			bufferPool.Put(buf)
			return
		}

		ttl := cc.durationTime
		if respCC.maxAge > 0 {
			ttl = time.Duration(respCC.maxAge) * time.Second
		}

		// Snapshot the body into a right-sized slice so we can return the
		// pooled buffer immediately. Cached entries outlive this call.
		body := append([]byte(nil), crw.body.Bytes()...)
		bufferPool.Put(buf)

		cc.cache.CacheResponse(cacheKey, &utils.CachedResponse{
			StatusCode: crw.statusCode,
			Headers:    map[string][]string(crw.Header().Clone()),
			Body:       body,
			ExpiresAt:  time.Now().Add(ttl),
		})
	})
}

func (cc *CacheControl) Close() error { return nil }

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

// captureResponseWriter buffers the response body up to max bytes so it can
// be cached. Once the limit is exceeded, buffering stops, the overflow flag
// is set, and the entry is skipped at the end of the chain. Bytes still
// stream through to the client unchanged.
type captureResponseWriter struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
	max        int
	overflow   bool
}

func (c *captureResponseWriter) WriteHeader(code int) {
	c.statusCode = code
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureResponseWriter) Write(b []byte) (int, error) {
	if !c.overflow {
		if c.body.Len()+len(b) > c.max {
			c.overflow = true
			c.body.Reset()
		} else {
			c.body.Write(b)
		}
	}
	return c.ResponseWriter.Write(b)
}

// Unwrap exposes the underlying ResponseWriter so http.ResponseController can
// reach Hijacker/Flusher/Pusher implementations on the original writer.
func (c *captureResponseWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

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
	bp := keyBufPool.Get().(*[]byte)
	buf := (*bp)[:0]
	buf = append(buf, r.Host...)
	buf = append(buf, '-')
	buf = append(buf, r.Method...)
	buf = append(buf, '-')
	buf = append(buf, r.URL.Path...)
	buf = append(buf, '?')
	buf = append(buf, r.URL.RawQuery...)
	sum := md5.Sum(buf)
	*bp = buf[:0]
	keyBufPool.Put(bp)
	return sum
}
