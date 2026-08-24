package base

import (
	"encoding/json"
	"time"
)

// Cacher 提供结构化的带 TTL 缓存（内部把值序列化为 JSON 存入 string 缓存）。
// 供各 provider 复用，避免每个 provider 重复实现缓存逻辑。
type Cacher struct {
	cache Cache
}

// NewCacher 创建缓存器。
func NewCacher(cache Cache) *Cacher {
	return &Cacher{cache: cache}
}

// Get 读取并反序列化缓存值；不存在或类型不符时返回 false。
func (c *Cacher) Get(key string, out interface{}) bool {
	if c == nil || c.cache == nil {
		return false
	}
	raw, ok := c.cache.Get(key)
	if !ok {
		return false
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return false
	}
	return true
}

// Put 序列化并写入缓存。
func (c *Cacher) Put(key string, v interface{}, ttl time.Duration) {
	if c == nil || c.cache == nil {
		return
	}
	if b, err := json.Marshal(v); err == nil {
		c.cache.Put(key, string(b), ttl)
	}
}