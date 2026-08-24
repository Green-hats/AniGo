package domain

import (
	"context"
	"time"
)

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

// ParsedTitle 是 AI 批量解析后返回的结构化标题信息。
type ParsedTitle struct {
	RawTitle   string  `json:"rawTitle"`
	Episode    float64 `json:"episode"`    // 集数（含 .5 特别篇）
	Resolution string  `json:"resolution"` // 1080P / 2160P / none
	Subgroup   string  `json:"subgroup"`   // 字幕组
	Title      string  `json:"title"`      // 去掉格式后的剧名
	IsSpecial  bool    `json:"isSpecial"`  // 是否 x.5 特别篇
}

// TitleParser 用 AI 批量解析 RSS 标题。
// 一次调用处理多条标题，减少请求数、省成本、降延迟。
type TitleParser interface {
	// Parse 解析一批标题，返回与输入等长、顺序一致的结果。
	// 对无法解析的条目返回 Episode=0。
	Parse(ctx context.Context, titles []string) ([]ParsedTitle, error)
}

// TitleFilter 用 AI 判断条目是否匹配订阅。
type TitleFilter interface {
	// Filter 判断每个条目是否保留（keep=true 保留）。
	// 输入标题与输出的 keep 数组顺序一致、长度相等。
	Filter(ctx context.Context, ani *Ani, titles []string) ([]bool, error)
}