package service

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/store"
)

func TestBatchQueueDrainsBeyondCapacityAndDeduplicatesWaiting(t *testing.T) {
	entered := make(chan string, 400)
	release := make(chan struct{})
	q := NewRefreshQueue(128, func(ctx context.Context, id string) error {
		entered <- id
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	q.Start(context.Background())
	defer q.Stop()
	ids := make([]string, 300)
	for i := range ids {
		ids[i] = fmt.Sprint(i)
	}
	if err := q.EnqueueBatch(ids...); err != nil {
		t.Fatal(err)
	}
	if err := q.EnqueueBatch(ids...); err != nil {
		t.Fatal(err)
	}
	active, waiting := 0, 0
	for _, job := range q.Snapshot() {
		switch job.State {
		case "queued", "running":
			active++
		case "waiting":
			waiting++
		}
	}
	if active != 128 || waiting != 172 {
		t.Fatalf("active=%d waiting=%d", active, waiting)
	}
	close(release)
	seen := map[string]bool{}
	for range ids {
		select {
		case id := <-entered:
			if seen[id] {
				t.Fatalf("duplicate %s", id)
			}
			seen[id] = true
		case <-time.After(3 * time.Second):
			t.Fatalf("only drained %d", len(seen))
		}
	}
	q.Stop()
	if len(entered) != 0 {
		t.Fatal("duplicate work")
	}
}

func TestBatchQueueCancelWaiting(t *testing.T) {
	q := NewRefreshQueue(1, func(ctx context.Context, id string) error { <-ctx.Done(); return ctx.Err() })
	q.Start(context.Background())
	if err := q.EnqueueBatch("a", "b", "c"); err != nil {
		t.Fatal(err)
	}
	q.Stop()
	for _, job := range q.Snapshot() {
		if job.State != "cancelled" {
			t.Fatal(job)
		}
	}
}

func TestAccountSwitchAndLegacyBindingSurviveRestart(t *testing.T) {
	cfg := reviewConfig(t)
	if err := cfg.SetConfigRaw([]byte(`{"downloadToolType":"pikpak","pikpakEmail":"a@example.com"}`)); err != nil {
		t.Fatal(err)
	}
	ani := domain.DefaultAni()
	ani.URL = reviewFeed(t, cfg)
	ani.DownloadTasks = []domain.DownloadTask{{Provider: "pikpak", Hash: "aaaa", Episode: 1, State: "failed", Torrent: "magnet:?xt=urn:btih:aaaa", Attempts: 1}}
	if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
		t.Fatal(err)
	}
	old := cfg.AniByID(ani.ID).DownloadTasks[0]
	if old.AccountID == "" {
		t.Fatal("legacy task unbound")
	}
	if err := cfg.SetConfigRaw([]byte(`{"pikpakEmail":"b@example.com"}`)); err != nil {
		t.Fatal(err)
	}
	cfg, err := NewConfigService(store.NewJSONStore(cfg.Dir()), store.NewTTLCache())
	if err != nil {
		t.Fatal(err)
	}
	driver := &trackingDriver{remote: []domain.OfflineTaskStatus{{Hash: "aaaa", State: "completed"}}}
	d := NewDownloadService(cfg, NewRssService(cfg, nil), cloudFn{driver}, store.NewTTLCache(), nil, nil, nil)
	if err := d.DownloadAni(context.Background(), ani); err != nil {
		t.Fatal(err)
	}
	saved := cfg.AniByID(ani.ID)
	if saved.DownloadTasks[0] != old || len(saved.Downloaded) != 0 || len(saved.DownloadTasks) != 2 || driver.adds != 1 {
		t.Fatalf("accounts mixed: %+v", saved.DownloadTasks)
	}
	if saved.DownloadTasks[1].AccountID == old.AccountID {
		t.Fatal("new task has old account")
	}
	if err := d.RecoverTask(context.Background(), ani.ID, old.Hash, 1, "retry"); err == nil {
		t.Fatal("recovery crossed account or active task")
	}
	if err := cfg.SetConfigRaw([]byte(`{"pikpakEmail":"a@example.com"}`)); err != nil {
		t.Fatal(err)
	}
	if err := d.DownloadAni(context.Background(), ani); err != nil {
		t.Fatal(err)
	}
	if cfg.AniByID(ani.ID).DownloadTasks[0].State != "completed" {
		t.Fatal("switch back did not resume")
	}
}

func TestManualRetryAndReplacement(t *testing.T) {
	for _, action := range []string{"retry", "replace"} {
		t.Run(action, func(t *testing.T) {
			cfg := reviewConfig(t)
			ani := domain.DefaultAni()
			ani.URL = reviewFeed(t, cfg)
			ani.DownloadTasks = []domain.DownloadTask{{Hash: "aaaa", Episode: 1, State: "exhausted", Attempts: 99, Torrent: "magnet:?xt=urn:btih:aaaa"}}
			if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
				t.Fatal(err)
			}
			driver := &trackingDriver{}
			d := NewDownloadService(cfg, NewRssService(cfg, nil), cloudFn{driver}, store.NewTTLCache(), nil, nil, nil)
			if err := d.RecoverTask(context.Background(), ani.ID, "aaaa", 1, action); err != nil {
				t.Fatal(err)
			}
			if err := d.DownloadAni(context.Background(), ani); err != nil {
				t.Fatal(err)
			}
			tasks := cfg.AniByID(ani.ID).DownloadTasks
			if driver.adds != 1 {
				t.Fatalf("adds=%d", driver.adds)
			}
			if action == "retry" && (len(tasks) != 1 || tasks[0].Attempts != 1 || tasks[0].State != "submitted") {
				t.Fatalf("retry=%+v", tasks)
			}
			if action == "replace" && (len(tasks) != 2 || tasks[0].State != "abandoned" || tasks[1].Hash != "bbbb") {
				t.Fatalf("replace=%+v", tasks)
			}
		})
	}
}

type countingStore struct {
	domain.ConfigStore
	writes atomic.Int32
}

func (s *countingStore) SaveAnis(list []*domain.Ani) error {
	s.writes.Add(1)
	return s.ConfigStore.SaveAnis(list)
}

func TestReconciliationBatchesWritesAndSkipsUnchanged(t *testing.T) {
	cfg := reviewConfig(t)
	ani := domain.DefaultAni()
	ani.DownloadTasks = []domain.DownloadTask{{Hash: "a", Episode: 1, State: "submitted", SubmittedAt: domain.NowMillis()}, {Hash: "b", Episode: 2, State: "submitted", SubmittedAt: domain.NowMillis()}}
	if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
		t.Fatal(err)
	}
	counted := &countingStore{ConfigStore: cfg.store}
	cfg.store = counted
	driver := &trackingDriver{remote: []domain.OfflineTaskStatus{{Hash: "a", State: "submitted"}, {Hash: "b", State: "submitted"}}}
	d := NewDownloadService(cfg, nil, cloudFn{driver}, store.NewTTLCache(), nil, nil, nil)
	if err := d.reconcileTasks(context.Background(), cfg.Get(), driver, cfg.AniByID(ani.ID)); err != nil {
		t.Fatal(err)
	}
	if counted.writes.Load() != 0 {
		t.Fatal("unchanged reconciliation wrote disk")
	}
	driver.remote[0].State = "completed"
	driver.remote[1].State = "completed"
	if err := d.reconcileTasks(context.Background(), cfg.Get(), driver, cfg.AniByID(ani.ID)); err != nil {
		t.Fatal(err)
	}
	if counted.writes.Load() != 1 || cfg.AniByID(ani.ID).DownloadedEps != 2 {
		t.Fatal("completion writes not batched")
	}
	if err := cfg.UpdateAni(ani.ID, func(a *domain.Ani) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if counted.writes.Load() != 1 {
		t.Fatal("no-op wrote disk")
	}
}

func TestCloudTimeoutWaitsForConfirmationAndUsesRemoteID(t *testing.T) {
	cfg := reviewConfig(t)
	ani := domain.DefaultAni()
	ani.URL = reviewFeed(t, cfg)
	ani.DownloadTasks = []domain.DownloadTask{{Hash: "aaaa", RemoteID: "original", Episode: 1, State: "submitted", SubmittedAt: time.Now().Add(-2 * time.Hour).UnixMilli()}}
	if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
		t.Fatal(err)
	}
	driver := &trackingDriver{remote: []domain.OfflineTaskStatus{{ID: "different", Hash: "aaaa", State: "completed"}}}
	d := NewDownloadService(cfg, NewRssService(cfg, nil), cloudFn{driver}, store.NewTTLCache(), nil, nil, nil)
	if err := d.DownloadAni(context.Background(), ani); err != nil {
		t.Fatal(err)
	}
	if driver.adds != 0 || cfg.AniByID(ani.ID).DownloadTasks[0].State != "unknown" {
		t.Fatal("timeout resubmitted or matched wrong remote ID")
	}
	if err := d.RecoverTask(context.Background(), ani.ID, "aaaa", 1, "retry"); err == nil {
		t.Fatal("unknown task can be resubmitted")
	}
	driver.remote = []domain.OfflineTaskStatus{{ID: "original", Hash: "aaaa", State: "completed"}}
	if err := d.DownloadAni(context.Background(), ani); err != nil {
		t.Fatal(err)
	}
	if cfg.AniByID(ani.ID).DownloadedEps != 1 {
		t.Fatal("late completion lost")
	}
}

type deadlineDriver struct {
	NoopDriver
	deadline time.Time
}

func (d *deadlineDriver) Login(ctx context.Context, _ bool, _ *domain.Config) (bool, error) {
	d.deadline, _ = ctx.Deadline()
	return false, errors.New("stop before network")
}
func TestRefreshDeadlineAndGateCancellation(t *testing.T) {
	cfg := reviewConfig(t)
	ani := domain.DefaultAni()
	if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetConfigRaw([]byte(`{"refreshTimeout":2}`)); err != nil {
		t.Fatal(err)
	}
	driver := &deadlineDriver{}
	d := NewDownloadService(cfg, nil, cloudFn{driver}, store.NewTTLCache(), nil, nil, nil)
	_ = d.DownloadAni(context.Background(), ani)
	if remaining := time.Until(driver.deadline); remaining < 119*time.Second || remaining > 120*time.Second {
		t.Fatalf("deadline=%v", remaining)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := d.DownloadAni(ctx, ani); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if len(d.gate) != 0 {
		t.Fatal("gate leaked")
	}
}

type timeoutSubmitter struct{ trackingDriver }

func (d *timeoutSubmitter) SubmitOfflineTask(ctx context.Context, cfg *domain.Config, magnet, path string, retry bool) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}
func TestSubmissionTimeoutKeepsRemoteIDAndDoesNotResubmit(t *testing.T) {
	cfg := reviewConfig(t)
	ani := domain.DefaultAni()
	ani.URL = reviewFeed(t, cfg)
	ani.DownloadTasks = []domain.DownloadTask{{Hash: "aaaa", RemoteID: "old-id", Episode: 1, State: "failed", Torrent: "magnet:?xt=urn:btih:aaaa"}}
	if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
		t.Fatal(err)
	}
	driver := &timeoutSubmitter{}
	d := NewDownloadService(cfg, NewRssService(cfg, nil), cloudFn{driver}, store.NewTTLCache(), nil, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := d.DownloadAni(ctx, ani)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	task := cfg.AniByID(ani.ID).DownloadTasks[0]
	if task.State != "unknown" || task.RemoteID != "old-id" || len(d.gate) != 0 {
		t.Fatalf("timeout corrupted task: %+v", task)
	}
	// A normal subsequent refresh must not enter SubmitOfflineTask again.
	next, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := d.DownloadAni(next, ani); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyBindingPersistsBeforeSettingsChange(t *testing.T) {
	cfg := reviewConfig(t)
	if err := cfg.SetConfigRaw([]byte(`{"pan115Cookie":"UID=123_A1_session"}`)); err != nil {
		t.Fatal(err)
	}
	ani := domain.DefaultAni()
	ani.DownloadTasks = []domain.DownloadTask{{Hash: "old", Episode: 1, State: "failed"}}
	// Simulate a pre-upgrade disk record, bypassing the new service migration.
	if err := cfg.store.SaveAnis([]*domain.Ani{ani}); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewConfigService(cfg.store, store.NewTTLCache())
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.SetConfigRaw([]byte(`{"pan115Cookie":"UID=456_A1_session"}`)); err != nil {
		t.Fatal(err)
	}
	again, err := NewConfigService(cfg.store, store.NewTTLCache())
	if err != nil {
		t.Fatal(err)
	}
	task := again.AniByID(ani.ID).DownloadTasks[0]
	if task.AccountID != domain.CloudAccountKey(cfg.Get(), "115") || taskBelongs(task, again.Get()) {
		t.Fatal("legacy record rebound to new account")
	}
}
