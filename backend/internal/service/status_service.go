package service

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

// aiStatusRefreshInterval 是 AI 连接状态两次真实探测的最小间隔。
// 状态接口会被前端高频轮询/刷新，若不节流会对付费大模型 API 产生不必要的调用。
const aiStatusRefreshInterval = time.Minute

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
	cfg      *ConfigService
	rss      *RssService
	download *DownloadService
	cache    domain.Cache
	startAt  time.Time

	aiMu     sync.Mutex
	aiAt     time.Time
	aiStatus *AIStatus
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
	// AI 状态（带节流：仅在配置变化或超过探测间隔时才真实调用大模型 API）
	st.AI = s.getAIStatus(ctx)
	return st
}

// getAIStatus 返回 AI 连接状态。
// 结果缓存 aiStatusRefreshInterval，缓存期内直接返回，不发起真实请求，
// 避免前端高频轮询状态时反复调用付费大模型 API。
func (s *StatusService) getAIStatus(ctx context.Context) *AIStatus {
	ai := &AIStatus{Configured: s.cfg.Get().AiApiKey != ""}
	if !ai.Configured {
		return ai
	}
	s.aiMu.Lock()
	defer s.aiMu.Unlock()
	if s.aiStatus != nil && time.Since(s.aiAt) < aiStatusRefreshInterval {
		return s.aiStatus
	}
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	reply, err := s.rss.AIPing(ctx2)
	if err == nil {
		s.aiStatus = &AIStatus{Configured: true, OK: true, Reply: reply}
	} else {
		s.aiStatus = &AIStatus{Configured: true, Message: err.Error()}
	}
	s.aiAt = time.Now()
	return s.aiStatus
}