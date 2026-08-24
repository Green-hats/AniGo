package domain

import "time"

// ConfigStore 是配置与订阅的持久化端口。
// 由 store 适配器（JSON 文件）实现。
type ConfigStore interface {
	// LoadConfig 读取配置，对缺失字段应用默认值。
	LoadConfig() (*Config, error)
	// SaveConfig 持久化配置。
	SaveConfig(c *Config) error
	// LoadAnis 读取订阅列表。
	LoadAnis() ([]*Ani, error)
	// SaveAnis 持久化订阅列表。
	SaveAnis(list []*Ani) error
}

// Cache 是轻量级内存 TTL 缓存，用于去重（缺集/摸鱼通知）等短时记忆。
type Cache interface {
	Get(key string) (string, bool)
	Put(key, val string, ttl time.Duration)
	Contains(key string) bool
	Clear()
}