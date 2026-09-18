package store

import (
	"sync"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

const cacheMaxEntries = 4096
const cacheMaxBytes = 16 << 20

type cacheEntry struct {
	value  string
	expiry time.Time
}
type TTLCache struct {
	mu    sync.Mutex
	m     map[string]cacheEntry
	bytes int
}

func NewTTLCache() *TTLCache { return &TTLCache{m: map[string]cacheEntry{}} }
func (c *TTLCache) removeLocked(key string) {
	if e, ok := c.m[key]; ok {
		c.bytes -= len(key) + len(e.value)
		delete(c.m, key)
	}
}
func (c *TTLCache) pruneLocked() {
	now := time.Now()
	for k, e := range c.m {
		if !now.Before(e.expiry) {
			c.removeLocked(k)
		}
	}
}
func (c *TTLCache) Prune() { c.mu.Lock(); defer c.mu.Unlock(); c.pruneLocked() }
func (c *TTLCache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok {
		return "", false
	}
	if !time.Now().Before(e.expiry) {
		c.removeLocked(key)
		return "", false
	}
	return e.value, true
}
func (c *TTLCache) Put(key, val string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeLocked(key)
	size := len(key) + len(val)
	if ttl <= 0 || size > cacheMaxBytes {
		return
	}
	if len(c.m) >= cacheMaxEntries || c.bytes+size > cacheMaxBytes {
		c.pruneLocked()
	}
	for len(c.m) >= cacheMaxEntries || c.bytes+size > cacheMaxBytes {
		oldest := ""
		var expiry time.Time
		for k, e := range c.m {
			if expiry.IsZero() || e.expiry.Before(expiry) {
				oldest, expiry = k, e.expiry
			}
		}
		if expiry.IsZero() {
			break
		}
		c.removeLocked(oldest)
	}
	c.m[key] = cacheEntry{val, time.Now().Add(ttl)}
	c.bytes += size
}
func (c *TTLCache) Contains(key string) bool { _, ok := c.Get(key); return ok }
func (c *TTLCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m = map[string]cacheEntry{}
	c.bytes = 0
}
func (c *TTLCache) Size() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked()
	return len(c.m), c.bytes
}

var _ domain.Cache = (*TTLCache)(nil)
