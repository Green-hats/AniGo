package cloud

import (
	"strings"
	"sync"

	"github.com/greenhats/anigo/internal/cloud/driver_115"
	"github.com/greenhats/anigo/internal/domain"
)

func init() {
	// 注册 115 网盘驱动（默认）
	Register("115", driver115.New)
	Register("pan115", driver115.New)
}

// DriverFactory 是网盘驱动的工厂函数。
type DriverFactory func() domain.CloudDriver

var (
	mu        sync.RWMutex
	registry  = map[string]DriverFactory{}
)

// Register 注册一个网盘驱动。name 不区分大小写。
func Register(name string, factory DriverFactory) {
	mu.Lock()
	defer mu.Unlock()
	registry[strings.ToLower(name)] = factory
}

// Registry 是网盘驱动注册表，根据配置的 DownloadToolType 选择驱动。
type Registry struct {
	mu       sync.Mutex
	current  domain.CloudDriver
	toolType string
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{}
}

// Get 返回当前配置对应的驱动实例。
// 当配置的 DownloadToolType 变化时自动重建。
func (r *Registry) Get(cfg *domain.Config) domain.CloudDriver {
	if cfg == nil {
		return nil
	}
	tt := strings.ToLower(strings.TrimSpace(cfg.DownloadToolType))
	if tt == "" {
		tt = "115"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current != nil && r.toolType == tt {
		return r.current
	}
	r.current = build(tt)
	r.toolType = tt
	return r.current
}

// Reload 强制重建当前驱动（配置变更后调用）。
func (r *Registry) Reload(cfg *domain.Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current = build(strings.ToLower(strings.TrimSpace(cfg.DownloadToolType)))
	r.toolType = strings.ToLower(strings.TrimSpace(cfg.DownloadToolType))
}

func build(t string) domain.CloudDriver {
	if t == "" {
		t = "115"
	}
	mu.RLock()
	factory, ok := registry[t]
	mu.RUnlock()
	if !ok || factory == nil {
		// 未注册的驱动类型回退到默认 115
		mu.RLock()
		factory, ok = registry["115"]
		mu.RUnlock()
		if !ok || factory == nil {
			return nil
		}
	}
	return factory()
}