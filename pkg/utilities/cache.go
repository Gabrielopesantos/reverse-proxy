package utilities

import (
	"container/list"
	"sync"
	"time"
)

// CachedResponse holds a stored HTTP response.
type CachedResponse struct {
	StatusCode int
	Headers    map[string][]string
	Body       []byte
	ExpiresAt  time.Time
}

// IsExpired reports whether the cached entry is past its expiry time.
func (cr *CachedResponse) IsExpired() bool {
	return time.Now().After(cr.ExpiresAt)
}

type cacheEntry struct {
	key   [16]byte
	value *CachedResponse
}

// SizeLimitedCache is an LRU cache bounded by a maximum number of entries.
type SizeLimitedCache struct {
	mu      sync.Mutex
	maxSize int
	items   map[[16]byte]*list.Element
	lru     *list.List
}

func NewSizeLimitedCache(maxSize uint) *SizeLimitedCache {
	return &SizeLimitedCache{
		maxSize: int(maxSize),
		items:   make(map[[16]byte]*list.Element),
		lru:     list.New(),
	}
}

// CacheResponse stores a response under the given key.
func (c *SizeLimitedCache) CacheResponse(key [16]byte, resp *CachedResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.lru.MoveToFront(elem)
		elem.Value.(*cacheEntry).value = resp
		return
	}

	if c.lru.Len() >= c.maxSize {
		c.evictOldest()
	}

	elem := c.lru.PushFront(&cacheEntry{key: key, value: resp})
	c.items[key] = elem
}

// GetResponse retrieves a non-expired response. Returns nil if not found or expired.
func (c *SizeLimitedCache) GetResponse(key [16]byte) *CachedResponse {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		return nil
	}
	entry := elem.Value.(*cacheEntry)
	if entry.value.IsExpired() {
		c.removeElement(elem)
		return nil
	}
	c.lru.MoveToFront(elem)
	return entry.value
}

// Evict removes the entry for the given key.
func (c *SizeLimitedCache) Evict(key [16]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.removeElement(elem)
	}
}

func (c *SizeLimitedCache) evictOldest() {
	back := c.lru.Back()
	if back != nil {
		c.removeElement(back)
	}
}

func (c *SizeLimitedCache) removeElement(elem *list.Element) {
	c.lru.Remove(elem)
	delete(c.items, elem.Value.(*cacheEntry).key)
}
