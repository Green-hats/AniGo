package service

import (
	"context"
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

// cloudFn 提供固定驱动的 CloudProvider 测试替身。
type cloudFn struct{ drv domain.CloudDriver }

func (c cloudFn) Get(cfg *domain.Config) domain.CloudDriver { return c.drv }
