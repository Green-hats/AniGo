package task

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/greenhats/anigo/internal/service"
)

// TaskManager 管理后台任务循环（RSS 轮询）。
// 用 context 控制生命周期，支持优雅退出。
type TaskManager struct {
	cfg      *service.ConfigService
	download *service.DownloadService
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.Mutex
	running  bool
}

// NewTaskManager 创建任务管理器。
func NewTaskManager(cfg *service.ConfigService, download *service.DownloadService) *TaskManager {
	return &TaskManager{cfg: cfg, download: download}
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
	t.wg.Add(1)
	go t.runRSSLoop()
	fmt.Println("[task] 后台任务已启动")
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
	fmt.Println("[task] 后台任务已停止")
}

// Restart 停止并重启（配置时间变化后调用）。
func (t *TaskManager) Restart() {
	t.Stop()
	t.Start()
}

// runRSSLoop 每 N 分钟执行一轮下载同步。
func (t *TaskManager) runRSSLoop() {
	defer t.wg.Done()
	for {
		cfg := t.cfg.Get()
		if cfg.Rss {
			t.download.SyncDownload(t.cfg.AniList())
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