package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/store"
)

func TestStatusReadsDoNotCallAIAndConfigChangesInvalidate(t *testing.T) {
	cfg := reviewConfig(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()
	raw, _ := json.Marshal(map[string]any{"aiEnabled": true, "aiApiKey": "local-test", "aiBaseURL": server.URL})
	if err := cfg.SetConfigRaw(raw); err != nil {
		t.Fatal(err)
	}
	rss := NewRssService(cfg, nil)
	d := NewDownloadService(cfg, rss, cloudFn{&NoopDriver{}}, store.NewTTLCache(), nil, nil, nil)
	status := NewStatusService(cfg, rss, d, store.NewTTLCache())
	for i := 0; i < 20; i++ {
		if status.Get(context.Background()).AI.CheckedAt != 0 {
			t.Fatal("untested status fabricated")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("page viewing calls paid AI")
	}
	if _, err := rss.AIPing(context.Background()); err != nil {
		t.Fatal(err)
	}
	observed := status.Get(context.Background()).AI
	if calls.Load() != 1 || !observed.OK || observed.Source != "test" || observed.CheckedAt == 0 {
		t.Fatalf("observation missing: %+v", observed)
	}
	if err := cfg.SetConfigRaw([]byte(`{"aiModel":"different"}`)); err != nil {
		t.Fatal(err)
	}
	if status.Get(context.Background()).AI.CheckedAt != 0 || calls.Load() != 1 {
		t.Fatal("old model status reused or automatically tested")
	}
	rss.observeAI(cfg.Get(), "parse", "", errors.New("upstream failed"))
	if status.Get(context.Background()).AI.Source != "parse" || status.Get(context.Background()).AI.OK {
		t.Fatal("real failure not reflected")
	}
}
func TestSummaryOmitsCompletedTasksButKeepsRecoveryAndHistory(t *testing.T) {
	cfg := reviewConfig(t)
	a := domain.DefaultAni()
	a.DownloadTasks = []domain.DownloadTask{{Hash: "done", Episode: 1, State: "completed"}, {Hash: "failed", Episode: 2, State: "exhausted"}}
	if err := cfg.SaveAniList([]*domain.Ani{a}); err != nil {
		t.Fatal(err)
	}
	svc := NewAniService(cfg, nil, nil)
	view := svc.ListAniView(true)
	visibleCount := 0
	for _, w := range view.WeekList {
		for _, item := range w.Items {
			visibleCount++
			if len(item.DownloadTasks) != 1 || item.TaskSummary["completed"] != 1 {
				t.Fatal(item)
			}
		}
	}
	if visibleCount != 1 {
		t.Fatal("undated subscription invisible")
	}
	if len(cfg.AniByID(a.ID).DownloadTasks) != 2 {
		t.Fatal("summary destroyed history")
	}
}
func TestDownloadFailureEmitsOneDeduplicatedNotification(t *testing.T) {
	cfg := reviewConfig(t)
	if err := cfg.SetConfigRaw([]byte(`{"notificationConfigList":[{"enable":true,"notificationType":"SYSTEM","retry":1,"statusList":["ERROR"]}]}`)); err != nil {
		t.Fatal(err)
	}
	var received atomic.Int32
	notify := NewNotifyService(cfg, func(string) { received.Add(1) })
	d := NewDownloadService(cfg, nil, cloudFn{&NoopDriver{}}, store.NewTTLCache(), nil, notify, nil)
	d.StartBackground(context.Background())
	defer d.StopBackground()
	a := domain.DefaultAni()
	if err := cfg.SaveAniList([]*domain.Ani{a}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := d.DownloadAni(context.Background(), a); err == nil {
			t.Fatal("expected login failure")
		}
	}
	deadline := time.After(time.Second)
	for received.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("error notification missing")
		case <-time.After(time.Millisecond):
		}
	}
	if len(notify.Records()) != 1 {
		t.Fatal("same error notified repeatedly")
	}
}
func TestConfigWatchOnlySignalsSuccessfulWrites(t *testing.T) {
	cfg := reviewConfig(t)
	_, changed := cfg.Watch()
	cfg.store = failingStore{cfg.store}
	if err := cfg.SetConfigRaw([]byte(`{"rssSleepMinutes":3}`)); err == nil {
		t.Fatal("expected failed write")
	}
	select {
	case <-changed:
		t.Fatal("failed config announced")
	default:
	}
}

func TestClearCachesDropsPlaybackSnapshot(t *testing.T) {
	cfg := reviewConfig(t)
	d := NewDownloadService(cfg, nil, cloudFn{&NoopDriver{}}, store.NewTTLCache(), nil, nil, nil)
	d.playCache["old"] = &playCacheEntry{expire: time.Now().Add(time.Minute)}
	before := d.playGeneration
	if err := d.ClearCaches(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(d.playCache) != 0 || d.playGeneration == before {
		t.Fatal("old playback cache retained")
	}
}
