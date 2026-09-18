package task

import (
	"context"
	"sync"
	"time"

	"github.com/greenhats/anigo/internal/domain"
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
	t.download.StartBackground(t.ctx)
	t.wg.Add(3)
	go t.runCacheLoop()
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
	t.download.StopBackground()
	t.wg.Wait()
	if t.logger != nil {
		t.logger.Info("task", "后台任务已停止")
	}
}

// runRSSLoop 每 N 分钟执行一轮下载同步。
func (t *TaskManager) runRSSLoop() {
	defer t.wg.Done()
	for {
		cfg, changed := t.cfg.Watch()
		if cfg.Rss {
			if err := t.download.EnqueueAll(); err != nil && t.logger != nil {
				t.logger.Error("task", err.Error())
			}
		}
		interval := time.Duration(cfg.RssSleepMinutes) * time.Minute
		if interval <= 0 {
			interval = 15 * time.Minute
		}
		if !t.waitConfig(interval, cfg, changed, false) {
			return
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
		cfg, changed := t.cfg.Watch()
		if t.meta != nil && t.ctx.Err() == nil {
			t.meta.RefreshAll(t.ctx, t.cfg.AniList())
		}
		interval := time.Duration(cfg.BgmRefreshHours) * time.Hour
		if interval <= 0 {
			interval = bgmRefreshInterval
		}
		if !t.waitConfig(interval, cfg, changed, true) {
			return
		}
	}
}

// bgmRefreshInterval 是 BGM 元数据刷新周期的默认值（配置未设置时使用）。
const bgmRefreshInterval = 6 * time.Hour

// Only scheduling changes interrupt a timer; unrelated settings do not trigger requests.
func (t *TaskManager) waitConfig(interval time.Duration, cfg *domain.Config, changed <-chan struct{}, bgm bool) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-t.ctx.Done():
			return false
		case <-timer.C:
			return true
		case <-changed:
			next, signal := t.cfg.Watch()
			if (bgm && cfg.BgmRefreshHours != next.BgmRefreshHours) || (!bgm && (cfg.Rss != next.Rss || cfg.RssSleepMinutes != next.RssSleepMinutes)) {
				return true
			}
			changed = signal
		}
	}
}
func (t *TaskManager) runCacheLoop() {
	defer t.wg.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.cfg.PruneCache()
		}
	}
}
