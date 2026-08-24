package service

import (
	"context"
	"runtime"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

// ServiceStatus 汇总各服务连接状态与系统资源。
type ServiceStatus struct {
	AI      *AIStatus       `json:"ai"`
	Cloud   domain.LoginStatus `json:"cloud"`
	Memory  *MemoryStatus   `json:"memory"`
	Cache   *CacheStatus    `json:"cache"`
	Uptime  int64           `json:"uptimeSeconds"`
}

// AIStatus 是 AI 服务连接状态。
type AIStatus struct {
	Configured bool   `json:"configured"`
	OK         bool   `json:"ok"`
	Reply      string `json:"reply"`
	Message    string `json:"message"`
}

// MemoryStatus 是进程内存占用。
type MemoryStatus struct {
	AllocMB     float64 `json:"allocMB"`
	TotalAllocMB float64 `json:"totalAllocMB"`
	SysMB       float64 `json:"sysMB"`
	NumGC       uint32  `json:"numGC"`
}

// CacheStatus 是缓存占用。
type CacheStatus struct {
	Count    int   `json:"count"`
	Bytes    int   `json:"bytes"`
	SizeKB   float64 `json:"sizeKB"`
}

// StatusService 汇总服务状态，供前端状态页/日志页展示。
type StatusService struct {
	cfg     *ConfigService
	rss     *RssService
	download *DownloadService
	cache   domain.Cache
	startAt time.Time
}

// NewStatusService 创建状态服务。
func NewStatusService(cfg *ConfigService, rss *RssService, download *DownloadService, cache domain.Cache) *StatusService {
	return &StatusService{
		cfg:      cfg,
		rss:      rss,
		download: download,
		cache:    cache,
		startAt:  time.Now(),
	}
}

// Get 收集当前状态（AI/115 ping 为实时探测）。
func (s *StatusService) Get(ctx context.Context) *ServiceStatus {
	st := &ServiceStatus{
		Cloud:  s.download.DownloadLoginStatus(),
		Uptime: int64(time.Since(s.startAt).Seconds()),
	}
	// 内存
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	st.Memory = &MemoryStatus{
		AllocMB:      float64(m.Alloc) / 1024 / 1024,
		TotalAllocMB: float64(m.TotalAlloc) / 1024 / 1024,
		SysMB:        float64(m.Sys) / 1024 / 1024,
		NumGC:        m.NumGC,
	}
	// 缓存
	if s.cache != nil {
		count, bytes := s.cache.Size()
		st.Cache = &CacheStatus{Count: count, Bytes: bytes, SizeKB: float64(bytes) / 1024}
	}
	// AI 实时探测（短超时）
	ai := &AIStatus{Configured: s.cfg.Get().AiApiKey != ""}
	if ai.Configured {
		ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		reply, err := s.rss.AIPing(ctx2)
		if err == nil {
			ai.OK = true
			ai.Reply = reply
		} else {
			ai.Message = err.Error()
		}
	}
	st.AI = ai
	return st
}