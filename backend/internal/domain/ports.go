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
	// Size 返回条目数与占用的字节数。
	Size() (count, bytes int)
}

// ParsedTitle 是 AI 批量解析后返回的结构化标题信息。
type ParsedTitle struct {
	RawTitle   string  `json:"rawTitle"`
	Episode    float64 `json:"episode"`    // 集数（含 .5 特别篇）
	Resolution string  `json:"resolution"` // 1080P / 2160P / none
	Subgroup   string  `json:"subgroup"`   // 字幕组
	Title      string  `json:"title"`      // 去掉格式后的剧名
	IsSpecial  bool    `json:"isSpecial"`  // 是否 x.5 特别篇

	// 选版信号（同集多版本竞争时用于挑选最优版本；无法判断则为空）
	SubtitleEmbed string `json:"subtitleEmbed"` // 字幕嵌入方式：内封 / 内嵌 / 外挂 / 空
	VideoCodec    string `json:"videoCodec"`    // 视频编码：HEVC / x265 / AVC / x264 / 空
	Source        string `json:"source"`        // 压制源：BD / BDRip / WebRip / Web / 空
	ColorDepth    string `json:"colorDepth"`    // 色深：10bit / 8bit / 空
	SubtitleLang  string `json:"subtitleLang"`  // 字幕语言：如 简繁日 / 简日 / 简 / 繁 / 空
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

// CloudFile 是一个云端文件条目。
type CloudFile struct {
	Name     string
	Size     int64
	IsDir    bool
	ID       string // 提供方特定的目录 id（115 cid）
	PickCode string // 提供方特定的下载码（115 pc）
}

// CloudDriver 是统一网盘驱动接口。目前实现 driver_115，后续扩展其他网盘。
// 网盘驱动只做"路径/磁力 → 云端文件"的原子操作，不感知订阅。
// 需要网盘凭据的方法接收 cfg 作为参数，保证始终拿到最新配置。
type CloudDriver interface {
	// Name 返回网盘名称。
	Name() string
	// Login 验证网盘凭据（如 115 Cookie）是否有效。
	Login(ctx context.Context, test bool, cfg *Config) (bool, error)
	// AddOfflineTask 离线下载一个磁力/ed2k 任务到指定云端目录。
	AddOfflineTask(ctx context.Context, cfg *Config, magnet, destPath string) error
	// FileExists 检查云端路径上的文件是否存在。
	FileExists(ctx context.Context, cfg *Config, path string) (bool, error)
	// FileURL 返回云端文件的可播放/下载 URL。
	FileURL(ctx context.Context, cfg *Config, path string) (string, error)
	// ListDir 列出云端目录的文件。
	ListDir(ctx context.Context, cfg *Config, path string) ([]CloudFile, error)
	// DeleteDir 递归删除云端目录。
	DeleteDir(ctx context.Context, cfg *Config, path string) error
	// GetLoginStatus 返回最近的登录状态。
	GetLoginStatus() LoginStatus
}

// LoginStatus 描述最近一次网盘登录结果。
type LoginStatus struct {
	Configured bool   `json:"configured"`
	OK         bool   `json:"loginOK"`
	Message    string `json:"message"`
}

// Notification 是一条待发送的通知。
type Notification struct {
	Text   string
	Status NotificationStatusEnum
	Ani    *Ani
}

// Notifier 是通知渠道实现（Telegram/Bark/ServerChan/WebHook/Shell/System...）。
type Notifier interface {
	// Type 返回通知类型（对应 NotificationTypeEnum）。
	Type() NotificationTypeEnum
	// Send 发送一条通知。cfg 是该渠道的配置。
	Send(ctx context.Context, cfg *NotificationConfig, n *Notification) error
}