package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/store"
)

// slowCloud 返回一个 AddOfflineTask 会阻塞等待 ctx 的驱动，用于验证取消传播。
type slowCloud struct{}

func (slowCloud) Get(cfg *domain.Config) domain.CloudDriver { return &slowDriver{} }

type slowDriver struct{ NoopDriver }

// AddOfflineTask 等待 ctx 取消或超时，模拟长时间卡住的网盘请求。
func (d *slowDriver) AddOfflineTask(ctx context.Context, cfg *domain.Config, magnet, destPath string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
	}
	return nil
}

func newTestDownloadService(t *testing.T, cloud CloudProvider) (*DownloadService, *ConfigService, *RssService) {
	t.Helper()
	s := store.NewJSONStore(t.TempDir())
	cfg, err := NewConfigService(s, store.NewTTLCache())
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}
	rss := NewRssService(cfg, nil)
	d := NewDownloadService(cfg, rss, cloud, store.NewTTLCache(), nil, nil, nil)
	return d, cfg, rss
}

// TestSyncDownloadStopsOnCancel 验证 ctx 取消后 SyncDownload 提前返回，
// 不会阻塞等待慢网盘请求（对应优雅停机已知问题）。
func TestSyncDownloadStopsOnCancel(t *testing.T) {
	d, cfg, rss := newTestDownloadService(t, slowCloud{})

	// 构建一个启用的订阅，其 RSS 解析返回空（避免真实网络），
	// 但下载流程仍需经过登录 → 条目循环的取消检查。
	ani := domain.DefaultAni()
	ani.Enable = true
	ani.URL = "http://example.invalid/rss.xml"
	cfg.SaveAniList([]*domain.Ani{ani})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		d.SyncDownload(ctx, cfg.AniList())
		close(done)
	}()

	// 让同步进入下载阶段后取消
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("ctx 取消后 SyncDownload 未提前返回")
	}
	_ = rss
}

// TestDownloadAniStopsOnCancel 验证单个订阅下载也感知 ctx。
func TestDownloadAniStopsOnCancel(t *testing.T) {
	d, _, _ := newTestDownloadService(t, slowCloud{})
	ani := domain.DefaultAni()
	ani.Enable = true
	ani.URL = "http://example.invalid/rss.xml"

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		d.DownloadAni(ctx, ani)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("ctx 取消后 DownloadAni 未提前返回")
	}
}

// completeCloud 记录已提交的离线任务，并在文件出现后 FileExists 返回 true。
type completeCloud struct{}

func (completeCloud) Get(cfg *domain.Config) domain.CloudDriver { return &completeDriver{} }

type completeDriver struct {
	NoopDriver
	complete map[string]bool
}

// FileExists 仅当路径在 complete 集合中出现时返回 true。
func (d *completeDriver) FileExists(ctx context.Context, cfg *domain.Config, path string) (bool, error) {
	return d.complete[path], nil
}

// TestCheckDownloadEndNotifiesOnCompletion 验证：
// 提交离线任务后文件仍不存在时不发通知；文件在云端出现后，
// checkDownloadEnd 触发 DOWNLOAD_END 通知并从 PendingDownload 移除。
func TestCheckDownloadEndNotifiesOnCompletion(t *testing.T) {
	s := store.NewJSONStore(t.TempDir())
	cfg, err := NewConfigService(s, store.NewTTLCache())
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}
	cfg.Get().NotificationConfigList = []domain.NotificationConfig{
		{
			Enable:           true,
			NotificationType: domain.NotifySystem,
			StatusList:       []domain.NotificationStatusEnum{domain.NotifyDownloadEnd},
		},
	}
	notifyMsg := make(chan string, 10)
	notifySvc := NewNotifyService(cfg, func(msg string) { notifyMsg <- msg })

	drv := &completeDriver{complete: map[string]bool{}}
	d := NewDownloadService(cfg, nil, cloudFn{drv}, store.NewTTLCache(), nil, notifySvc, nil)

	ani := domain.DefaultAni()
	ani.Enable = true
	ani.PendingDownload = map[string]domain.PendingDownload{
		"hash1": {Path: "/番剧/测试/Season 1/ep01.mkv", Master: true},
		"hash2": {Path: "/番剧/测试/Season 1/ep02.mkv", Master: false},
	}

	// 第一轮：文件尚未出现，不应有通知、不应移除
	d.checkDownloadEnd(context.Background(), ani)
	if len(ani.PendingDownload) != 2 {
		t.Fatalf("文件未完成不应移除, got %d", len(ani.PendingDownload))
	}

	// 云端转存完成 ep01
	drv.complete["/番剧/测试/Season 1/ep01.mkv"] = true
	d.checkDownloadEnd(context.Background(), ani)
	if len(ani.PendingDownload) != 1 {
		t.Fatalf("完成后应移除 1 条, got %d", len(ani.PendingDownload))
	}
	if _, ok := ani.PendingDownload["hash1"]; ok {
		t.Error("已完成的 hash1 应被移除")
	}
	select {
	case msg := <-notifyMsg:
		if !strings.Contains(msg, "ep01.mkv") {
			t.Errorf("通知应含文件名, got %q", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DOWNLOAD_END 通知未发送")
	}

	// ep02 完成，pending 清空
	drv.complete["/番剧/测试/Season 1/ep02.mkv"] = true
	d.checkDownloadEnd(context.Background(), ani)
	if len(ani.PendingDownload) != 0 {
		t.Fatalf("全部完成后应清空, got %d", len(ani.PendingDownload))
	}
	select {
	case <-notifyMsg:
	case <-time.After(2 * time.Second):
		t.Fatal("第二条 DOWNLOAD_END 通知未发送")
	}
}

// cloudFn 提供固定驱动的 CloudProvider 测试替身。
type cloudFn struct{ drv domain.CloudDriver }

func (c cloudFn) Get(cfg *domain.Config) domain.CloudDriver { return c.drv }
