package utilities

import "sync"

type SizeLimitedCache struct {
	maxSize uint
	size    uint
	cache   sync.Map
}

func NewSizeLimitedCache(maxSize uint) *SizeLimitedCache {
	return &SizeLimitedCache{
		maxSize: maxSize,
		size:    0,
		cache:   sync.Map{},
	}
}

func (c *SizeLimitedCache) CacheResponse() {
}

func (c *SizeLimitedCache) GetResponse() {
}

func (c *SizeLimitedCache) Evict() {
}
