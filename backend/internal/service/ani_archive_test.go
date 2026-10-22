package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/store"
)

func TestArchivePreservesSubscriptionAndSurvivesReload(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[enabled], func(t *testing.T) {
			st := store.NewJSONStore(t.TempDir())
			cfg, err := NewConfigService(st, store.NewTTLCache())
			if err != nil {
				t.Fatal(err)
			}
			ani := domain.DefaultAni()
			ani.Enable = enabled
			ani.Title = "已完结的订阅"
			ani.ReleaseDate = domain.Date(time.Date(2026, 7, 1, 0, 0, 0, 0, domain.Loc()))
			ani.CustomEpisodeStr = domain.RENAME_REG_STR()
			ani.Downloaded = []float64{1, 2, 3}
			ani.DownloadedHash = []string{"saved-hash"}
			ani.DownloadedEps = 3
			ani.CurrentEpisodeNumber = 3
			ani.LastDownloadTime = 123456789
			if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
				t.Fatal(err)
			}
			svc := NewAniService(cfg, nil, nil)
			if err := svc.SetArchived(ani.ID, true); err != nil {
				t.Fatal(err)
			}
			list := svc.ListAni()
			if list.Total != 1 || len(list.ArchivedList) != 1 || len(list.ReleaseDateList) != 0 {
				t.Fatalf("archived subscription not separated: %+v", list)
			}
			for _, week := range list.WeekList {
				if len(week.Items) != 0 {
					t.Fatal("archived subscription remained in weekday list")
				}
			}
			svc.BatchEnable([]string{ani.ID}, !enabled)
			if svc.FindAniByID(ani.ID).Enable != enabled {
				t.Fatal("batch enable changed the archived subscription's previous state")
			}
			loaded, err := NewConfigService(st, store.NewTTLCache())
			if err != nil {
				t.Fatal(err)
			}
			if !loaded.AniList()[0].Archived {
				t.Fatal("archive did not survive reload")
			}
			restored := NewAniService(loaded, nil, nil)
			if err := restored.SetArchived(ani.ID, false); err != nil {
				t.Fatal(err)
			}
			got := restored.FindAniByID(ani.ID)
			if got.Archived || got.Enable != enabled || got.Title != ani.Title ||
				got.DownloadedEps != 3 || got.CurrentEpisodeNumber != 3 || got.LastDownloadTime != 123456789 ||
				!reflect.DeepEqual(got.Downloaded, []float64{1, 2, 3}) || !reflect.DeepEqual(got.DownloadedHash, []string{"saved-hash"}) {
				t.Fatalf("archive/restore changed subscription state or download history: %+v", got)
			}
			active := restored.ListAni()
			count := 0
			for _, week := range active.WeekList {
				count += len(week.Items)
			}
			if count != 1 || len(active.ArchivedList) != 0 {
				t.Fatal("restored subscription did not return to weekday list")
			}
		})
	}
}

type failedArchiveStore struct{ domain.ConfigStore }

func (failedArchiveStore) SaveAnis([]*domain.Ani) error { return errors.New("disk unavailable") }

func TestArchiveSaveFailureDoesNotChangeMemory(t *testing.T) {
	_, cfg, _ := newTestDownloadService(t, slowCloud{})
	ani := domain.DefaultAni()
	if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
		t.Fatal(err)
	}
	cfg.store = failedArchiveStore{cfg.store}
	svc := NewAniService(cfg, nil, nil)
	if err := svc.SetArchived(ani.ID, true); err == nil {
		t.Fatal("expected persistence failure")
	}
	if svc.FindAniByID(ani.ID).Archived {
		t.Fatal("failed save left subscription archived in memory")
	}
}

// 归档后不应访问网盘、RSS 或元数据服务，包括单项刷新和刷新全部。
type unexpectedArchiveCloud struct{ t *testing.T }

func (c unexpectedArchiveCloud) Get(*domain.Config) domain.CloudDriver {
	c.t.Fatal("archived subscription accessed cloud driver")
	return nil
}

func TestArchivedSubscriptionsSkipBackgroundWork(t *testing.T) {
	download, cfg, _ := newTestDownloadService(t, unexpectedArchiveCloud{t})
	archived := domain.DefaultAni()
	archived.Archived = true
	archived.BgmUrl = "https://bgm.tv/subject/123"
	disabled := domain.DefaultAni()
	disabled.Enable = false
	if err := cfg.SaveAniList([]*domain.Ani{archived, disabled}); err != nil {
		t.Fatal(err)
	}
	download.SyncDownload(context.Background(), []*domain.Ani{nil, archived, disabled})
	// 已入队或已取得旧快照的任务，也必须在执行时读取最新归档状态。
	stale := archived.Clone()
	stale.Archived = false
	if err := download.DownloadAni(context.Background(), stale); err == nil {
		t.Fatal("download accepted a stale unarchived snapshot")
	}
	if err := download.queue.run(context.Background(), archived.ID); err == nil {
		t.Fatal("queued refresh accepted an archived subscription")
	}
	if err := download.EnqueueAll(); err != nil {
		t.Fatalf("refresh all should skip archived and disabled subscriptions: %v", err)
	}
	// nil provider would panic if metadata processing were attempted.
	meta := &MetadataService{cfg: cfg}
	meta.RefreshAll(context.Background(), []*domain.Ani{archived})
}
