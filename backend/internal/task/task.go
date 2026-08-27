package task

import (
	"context"
	"sync"
	"time"

	"github.com/greenhats/anigo/internal/log"
	"github.com/greenhats/anigo/internal/service"
)

// TaskManager 管理后台任务循环（RSS 轮询 + BGM 元数据刷新）。
// 用 context 控制生命周期，支持优雅退出。
type TaskManager struct {
	cfg      *service.ConfigService
	download *service.DownloadService
	meta     *service.MetadataService
	logger   *log.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.Mutex
	running  bool
}

// NewTaskManager 创建任务管理器。
func NewTaskManager(cfg *service.ConfigService, download *service.DownloadService, meta *service.MetadataService, logger *log.Logger) *TaskManager {
	return &TaskManager{cfg: cfg, download: download, meta: meta, logger: logger}
}

// Start 启动后台循环。
func (t *TaskManager) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running {
		return
	}
	t.running = true
	t.ctx, t.cancel = context.WithCancel(context.Background())
	t.wg.Add(2)
	go t.runRSSLoop()
	go t.runBgmLoop()
	if t.logger != nil {
		t.logger.Info("task", "后台任务已启动")
	}
}

// Stop 停止所有后台循环并等待退出。
func (t *TaskManager) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.running {
		return
	}
	t.running = false
	t.cancel()
	t.wg.Wait()
	if t.logger != nil {
		t.logger.Info("task", "后台任务已停止")
	}
}

// runRSSLoop 每 N 分钟执行一轮下载同步。
func (t *TaskManager) runRSSLoop() {
	defer t.wg.Done()
	for {
		cfg := t.cfg.Get()
		if cfg.Rss {
			t.download.SyncDownload(t.ctx, t.cfg.AniList())
		}
		interval := time.Duration(cfg.RssSleepMinutes) * time.Minute
		if interval <= 0 {
			interval = 15 * time.Minute
		}
		select {
		case <-t.ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// runBgmLoop 每 N 小时刷新一轮订阅的 Bangumi 元数据（评分/总集数/封面）。
// 周期取配置 bgmRefreshHours，未配置时回退到默认值。
// 与 runRSSLoop 独立，避免单次元数据请求阻塞下载同步。
// 启动即刷新一轮（元数据尽快就位），之后按周期循环。
func (t *TaskManager) runBgmLoop() {
	defer t.wg.Done()
	for {
		if t.meta != nil && t.ctx.Err() == nil {
			t.meta.RefreshAll(t.ctx, t.cfg.AniList())
		}
		interval := time.Duration(t.cfg.Get().BgmRefreshHours) * time.Hour
		if interval <= 0 {
			interval = bgmRefreshInterval
		}
		select {
		case <-t.ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// bgmRefreshInterval 是 BGM 元数据刷新周期的默认值（配置未设置时使用）。
const bgmRefreshInterval = 6 * time.Hour