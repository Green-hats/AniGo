package service

import (
	"context"
	"testing"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/store"
)

func TestSwitchingCloudDoesNotReconcileOrOverwriteOldTasks(t *testing.T) {
	cfg := reviewConfig(t)
	ani := domain.DefaultAni()
	ani.URL = reviewFeed(t, cfg)
	ani.TotalEpisodeNumber = 1
	ani.DownloadTasks = []domain.DownloadTask{{Hash: "aaaa", Episode: 1, State: "submitted"}}
	if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetConfigRaw([]byte(`{"downloadToolType":"pikpak"}`)); err != nil {
		t.Fatal(err)
	}
	driver := &trackingDriver{remote: []domain.OfflineTaskStatus{{Hash: "aaaa", State: "completed"}}}
	d := NewDownloadService(cfg, NewRssService(cfg, nil), cloudFn{driver}, store.NewTTLCache(), nil, nil, nil)
	if err := d.DownloadAni(context.Background(), ani); err != nil {
		t.Fatal(err)
	}
	saved := cfg.AniList()[0]
	if !saved.Enable || len(saved.Downloaded) != 0 || len(saved.DownloadTasks) != 2 {
		t.Fatalf("clouds mixed: %+v", saved)
	}
	if saved.DownloadTasks[0].State != "submitted" || saved.DownloadTasks[1].Provider != "pikpak" {
		t.Fatalf("old task overwritten: %+v", saved.DownloadTasks)
	}
	view := NewAniService(cfg, nil, nil).ListAni()
	for _, week := range view.WeekList {
		for _, a := range week.Items {
			if len(a.DownloadTasks) != 1 || a.DownloadTasks[0].Provider != "pikpak" {
				t.Fatal("wrong cloud tasks displayed")
			}
		}
	}
	// Switching back reuses the 115 record without resubmitting PikPak's task.
	if err := cfg.SetConfigRaw([]byte(`{"downloadToolType":"115"}`)); err != nil {
		t.Fatal(err)
	}
	driver.remote = nil
	if err := d.DownloadAni(context.Background(), ani); err != nil {
		t.Fatal(err)
	}
	if len(cfg.AniList()[0].DownloadTasks) != 2 || driver.adds != 1 {
		t.Fatal("switch resubmitted an existing task")
	}
}
