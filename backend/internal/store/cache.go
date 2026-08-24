package store

import (
	"sync"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

type cacheEntry struct {
	value  string
	expiry time.Time
}

// TTLCache 是内存 TTL 缓存，实现 domain.Cache。
type TTLCache struct {
	mu sync.Mutex
	m  map[string]cacheEntry
}

// NewTTLCache 创建空缓存。
func NewTTLCache() *TTLCache {
	return &TTLCache{m: map[string]cacheEntry{}}
}

// Get 返回值及其是否仍然有效。
func (c *TTLCache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok {
		return "", false
	}
	if time.Now().After(e.expiry) {
		delete(c.m, key)
		return "", false
	}
	return e.value, true
}

// Put 存储带 TTL 的值。
func (c *TTLCache) Put(key, val string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = cacheEntry{value: val, expiry: time.Now().Add(ttl)}
}

// Contains 报告键是否存在且未过期。
func (c *TTLCache) Contains(key string) bool {
	_, ok := c.Get(key)
	return ok
}

// Clear 清空缓存。
func (c *TTLCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m = map[string]cacheEntry{}
}

// Size 返回条目数与占用的字节数（未过期的）。
func (c *TTLCache) Size() (count, bytes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, e := range c.m {
		if now.After(e.expiry) {
			delete(c.m, k)
			continue
		}
		count++
		bytes += len(k) + len(e.value)
	}
	return count, bytes
}

var _ domain.Cache = (*TTLCache)(nil)