package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/store"
)

type slowCloud struct{ driver *slowDriver }

func (s slowCloud) Get(cfg *domain.Config) domain.CloudDriver { return s.driver }

type slowDriver struct {
	NoopDriver
	entered chan struct{}
}

func (d *slowDriver) Login(context.Context, bool, *domain.Config) (bool, error) { return true, nil }
func (d *slowDriver) AddOfflineTask(ctx context.Context, cfg *domain.Config, magnet, destPath string) error {
	close(d.entered)
	<-ctx.Done()
	return ctx.Err()
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

func TestDownloadCancellationReachesCloud(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(fmt.Sprint(all), func(t *testing.T) {
			driver := &slowDriver{entered: make(chan struct{})}
			d, cfg, _ := newTestDownloadService(t, slowCloud{driver})
			ani := domain.DefaultAni()
			ani.Title, ani.URL = "Review", reviewFeed(t, cfg)
			if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			go func() {
				defer close(done)
				if all {
					d.SyncDownload(ctx, cfg.AniList())
				} else {
					_ = d.DownloadAni(ctx, ani)
				}
			}()
			select {
			case <-driver.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("未进入真实下载步骤")
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("取消未传到云端请求")
			}
		})
	}
}

// cloudFn 提供固定驱动的 CloudProvider 测试替身。
type cloudFn struct{ drv domain.CloudDriver }

func (c cloudFn) Get(cfg *domain.Config) domain.CloudDriver { return c.drv }
