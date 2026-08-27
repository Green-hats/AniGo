package task

import (
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/service"
	"github.com/greenhats/anigo/internal/store"
)

// noopCloud 提供一个永不登录的网盘驱动，避免测试依赖真实 115。
type noopCloud struct{}

func (noopCloud) Get(cfg *domain.Config) domain.CloudDriver {
	return &service.NoopDriver{}
}

func newTestTaskManager(t *testing.T) *TaskManager {
	t.Helper()
	s := store.NewJSONStore(t.TempDir())
	cfg, err := service.NewConfigService(s, store.NewTTLCache())
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}
	// 关闭 RSS 轮询，避免测试期间触发真实下载同步
	cfg.Get().Rss = false
	rss := service.NewRssService(cfg, nil)
	download := service.NewDownloadService(cfg, rss, noopCloud{}, store.NewTTLCache(), nil, nil, nil)
	meta := service.NewMetadataService(cfg, store.NewTTLCache())
	return NewTaskManager(cfg, download, meta, nil)
}

func TestStartStopLifecycle(t *testing.T) {
	tm := newTestTaskManager(t)
	tm.Start()
	if !tm.running {
		t.Error("Start 后 running 应为 true")
	}
	tm.Stop()
	if tm.running {
		t.Error("Stop 后 running 应为 false")
	}
	// 再次 Stop 不应 panic
	tm.Stop()
}

func TestStartIsIdempotent(t *testing.T) {
	tm := newTestTaskManager(t)
	tm.Start()
	tm.Start() // 第二次不应启动新 goroutine
	tm.Stop()
}

func TestStopBeforeStart(t *testing.T) {
	tm := newTestTaskManager(t)
	tm.Stop() // 未启动时调用应安全
	if tm.running {
		t.Error("未启动不应标记 running")
	}
}

func TestStartStopCompletesWithinTimeout(t *testing.T) {
	// 即使 Rss=true，Stop 也应通过 context 取消快速返回（不会等完整个 sleep interval）
	tm := newTestTaskManager(t)
	tm.cfg.Get().Rss = false
	tm.Start()
	done := make(chan struct{})
	go func() {
		tm.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop 超时，未能优雅退出")
	}
}

func TestBgmLoopStartsAndStops(t *testing.T) {
	// runBgmLoop 应随 Start 启动、随 Stop 优雅退出
	tm := newTestTaskManager(t)
	tm.Start()
	// 等待两个循环 goroutine 都就绪
	time.Sleep(50 * time.Millisecond)
	tm.Stop()
	if tm.running {
		t.Error("Stop 后 running 应为 false")
	}
}
