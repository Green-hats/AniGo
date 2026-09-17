package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/store"
)

type failingStore struct{ domain.ConfigStore }

func (f failingStore) SaveConfig(*domain.Config) error { return errors.New("disk full") }
func (f failingStore) SaveAnis([]*domain.Ani) error    { return errors.New("disk full") }

func TestSnapshotsAndFailedWritesAreIsolated(t *testing.T) {
	cfg := reviewConfig(t)
	ani := domain.DefaultAni()
	ani.Title = "original"
	if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
		t.Fatal(err)
	}
	ani.Title = "caller mutation"
	read := cfg.AniList()
	read[0].Title = "reader mutation"
	snapshot := cfg.Get()
	snapshot.Exclude = append(snapshot.Exclude, "x")
	if cfg.AniList()[0].Title != "original" {
		t.Fatal("subscription snapshot aliases internal data")
	}
	if len(cfg.Get().Exclude) == len(snapshot.Exclude) {
		t.Fatal("config snapshot aliases internal data")
	}
	before := cfg.Get().RssSleepMinutes
	cfg.store = failingStore{cfg.store}
	if err := cfg.SetConfigRaw([]byte(`{"rssSleepMinutes":123}`)); err == nil {
		t.Fatal("expected write failure")
	}
	if cfg.Get().RssSleepMinutes != before {
		t.Fatal("failed config write changed memory")
	}
	if err := cfg.UpdateAni(ani.ID, func(a *domain.Ani) error { a.Title = "changed"; return nil }); err == nil {
		t.Fatal("expected write failure")
	}
	if cfg.AniList()[0].Title != "original" {
		t.Fatal("failed subscription write changed memory")
	}
}

func TestStandbySourceStillWorksWhenMainFails(t *testing.T) {
	cfg := reviewConfig(t)
	feed := reviewFeed(t, cfg)
	if err := cfg.SetConfigRaw([]byte(`{"standbyRss":true}`)); err != nil {
		t.Fatal(err)
	}
	ani := domain.DefaultAni()
	ani.URL = feed + "/missing"
	ani.StandbyRssList = []domain.StandbyRss{{URL: feed, Label: "backup"}}
	items, err := NewRssService(cfg, nil).GetItems(context.Background(), ani)
	if err != nil || len(items) != 1 || items[0].Resolution != "2160p" || items[0].Master {
		t.Fatalf("backup failed: %+v %v", items, err)
	}
}

func TestConcurrentSubscriptionAddsDoNotLoseUpdates(t *testing.T) {
	cfg := reviewConfig(t)
	service := NewAniService(cfg, nil, nil)
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ani := domain.DefaultAni()
			ani.Title = fmt.Sprint(i)
			if err := service.AddAni(ani); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if len(cfg.AniList()) != 30 {
		t.Fatalf("lost additions: %d", len(cfg.AniList()))
	}
	disk, err := cfg.store.LoadAnis()
	if err != nil {
		t.Fatal(err)
	}
	if len(disk) != 30 {
		t.Fatal("disk differs from memory")
	}
}

func writeTestBackup(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backup.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBackupValidationAndRoundTrip(t *testing.T) {
	cfg := reviewConfig(t)
	ani := domain.DefaultAni()
	ani.Title = "backup"
	if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.Dir(), "files"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Dir(), "files", "cover.png"), []byte("cover"), 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(cfg.Dir(), "config.v2.json"))
	for _, files := range []map[string]string{
		{"config.v2.json": "{}", "ani.v2.json": "invalid"},
		{"config.v2.json": "null", "ani.v2.json": "[]"},
		{"config.v2.json": "{}", "ani.v2.json": "[null]"},
		{"config.v2.json": "{}", "ani.v2.json": "[]", "../escape": "x"},
	} {
		if err := cfg.ImportConfig(writeTestBackup(t, files)); err == nil {
			t.Fatal("invalid backup accepted")
		}
		after, _ := os.ReadFile(filepath.Join(cfg.Dir(), "config.v2.json"))
		if !bytes.Equal(before, after) {
			t.Fatal("invalid backup altered original")
		}
	}
	var buf bytes.Buffer
	if err := cfg.ExportConfig(&buf); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "good.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	target := reviewConfig(t)
	if err := target.ImportConfig(path); err != nil {
		t.Fatal(err)
	}
	cover, err := os.ReadFile(filepath.Join(target.Dir(), "files", "cover.png"))
	if err != nil || string(cover) != "cover" {
		t.Fatalf("cover restore: %q %v", cover, err)
	}
	if target.AniList()[0].Title != "backup" {
		t.Fatal("subscriptions not restored")
	}
	restarted, err := NewConfigService(store.NewJSONStore(target.Dir()), store.NewTTLCache())
	if err != nil {
		t.Fatal(err)
	}
	if restarted.AniList()[0].ID != ani.ID {
		t.Fatal("restart lost restored data")
	}
}

func TestAIOnlyParsesNewOrChangedRules(t *testing.T) {
	cfg := reviewConfig(t)
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		var titles []string
		if err := json.Unmarshal([]byte(req.Messages[1].Content), &titles); err != nil {
			t.Error(err)
			return
		}
		count.Add(int32(len(titles)))
		results := []domain.ParsedTitle{}
		for _, title := range titles {
			results = append(results, domain.ParsedTitle{RawTitle: title, Title: title, Episode: 1})
		}
		raw, _ := json.Marshal(results)
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(raw)}}}})
	}))
	defer server.Close()
	raw, _ := json.Marshal(map[string]any{"aiApiKey": "test", "aiBaseURL": server.URL, "aiEnabled": true})
	if err := cfg.SetConfigRaw(raw); err != nil {
		t.Fatal(err)
	}
	rss := NewRssService(cfg, nil)
	ani := domain.DefaultAni()
	for _, titles := range [][]string{{"A", "B"}, {"B", "A"}, {"A", "B", "C"}} {
		if _, err := rss.parseCached(context.Background(), cfg.Get(), ani, titles); err != nil {
			t.Fatal(err)
		}
	}
	if count.Load() != 3 {
		t.Fatalf("expected 3 new titles, got %d", count.Load())
	}
	ani.Match = []string{"new rule"}
	if _, err := rss.parseCached(context.Background(), cfg.Get(), ani, []string{"A"}); err != nil {
		t.Fatal(err)
	}
	if count.Load() != 4 {
		t.Fatal("rule changes did not invalidate cache")
	}
}

type trackingDriver struct {
	NoopDriver
	adds   int
	remote []domain.OfflineTaskStatus
}

func (d *trackingDriver) Login(context.Context, bool, *domain.Config) (bool, error) { return true, nil }
func (d *trackingDriver) AddOfflineTask(context.Context, *domain.Config, string, string) error {
	d.adds++
	return nil
}
func (d *trackingDriver) OfflineTasks(context.Context, *domain.Config) ([]domain.OfflineTaskStatus, error) {
	return d.remote, nil
}
func (d *trackingDriver) RetryOfflineTask(context.Context, *domain.Config, string, string, string) error {
	d.adds++
	return nil
}

func TestSubmittedTaskSurvivesRestartAndCompletesOnlyAfterConfirmation(t *testing.T) {
	cfg := reviewConfig(t)
	ani := domain.DefaultAni()
	ani.Title, ani.URL, ani.TotalEpisodeNumber = "Review", reviewFeed(t, cfg), 1
	if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
		t.Fatal(err)
	}
	driver := &trackingDriver{}
	makeDownloader := func(c *ConfigService) *DownloadService {
		return NewDownloadService(c, NewRssService(c, nil), cloudFn{driver}, store.NewTTLCache(), nil, nil, nil)
	}
	if err := makeDownloader(cfg).DownloadAni(context.Background(), ani); err != nil {
		t.Fatal(err)
	}
	saved := cfg.AniList()[0]
	if len(saved.Downloaded) != 0 || !saved.Enable || saved.DownloadTasks[0].State != "submitted" {
		t.Fatalf("premature completion: %+v", saved)
	}
	restarted, err := NewConfigService(store.NewJSONStore(cfg.Dir()), store.NewTTLCache())
	if err != nil {
		t.Fatal(err)
	}
	d := makeDownloader(restarted)
	if err := d.DownloadAni(context.Background(), ani); err != nil {
		t.Fatal(err)
	}
	if driver.adds != 1 {
		t.Fatal("restart resubmitted pending task")
	}
	driver.remote = []domain.OfflineTaskStatus{{Hash: saved.DownloadTasks[0].Hash, State: "completed"}}
	if err := d.DownloadAni(context.Background(), ani); err != nil {
		t.Fatal(err)
	}
	saved = restarted.AniList()[0]
	if saved.Enable || saved.DownloadedEps != 1 || len(saved.Downloaded) != 1 {
		t.Fatalf("completion not persisted: %+v", saved)
	}
}

func TestFailedTaskBackoffAndRetryLimit(t *testing.T) {
	cfg := reviewConfig(t)
	ani := domain.DefaultAni()
	ani.Title, ani.URL = "Review", reviewFeed(t, cfg)
	if err := cfg.SetConfigRaw([]byte(`{"downloadRetry":1}`)); err != nil {
		t.Fatal(err)
	}
	ani.DownloadTasks = []domain.DownloadTask{{Hash: "aaaa", Episode: 1, Torrent: "magnet:?xt=urn:btih:aaaa", Path: "Review/file.mkv", State: "submitted", Attempts: 1, RetryAt: time.Now().Add(time.Hour).UnixMilli()}}
	if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
		t.Fatal(err)
	}
	driver := &trackingDriver{remote: []domain.OfflineTaskStatus{{Hash: "aaaa", State: "failed", Error: "remote failed"}}}
	d := NewDownloadService(cfg, NewRssService(cfg, nil), cloudFn{driver}, store.NewTTLCache(), nil, nil, nil)
	if err := d.DownloadAni(context.Background(), ani); err != nil {
		t.Fatal(err)
	}
	if driver.adds != 0 {
		t.Fatal("retried during backoff")
	}
	expire := func() {
		if err := cfg.UpdateAni(ani.ID, func(a *domain.Ani) error { a.DownloadTasks[0].RetryAt = 0; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	expire()
	if err := d.DownloadAni(context.Background(), ani); err != nil {
		t.Fatal(err)
	}
	if driver.adds != 1 {
		t.Fatal("failed task not retried")
	}
	expire()
	if err := d.DownloadAni(context.Background(), ani); err != nil {
		t.Fatal(err)
	}
	if driver.adds != 1 || cfg.AniList()[0].DownloadedEps != 0 {
		t.Fatal("retry limit or completion count incorrect")
	}
}

func TestRefreshQueueDeduplicatesBoundsAndCancels(t *testing.T) {
	entered := make(chan struct{})
	var calls atomic.Int32
	q := NewRefreshQueue(2, func(ctx context.Context, id string) error {
		calls.Add(1)
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	})
	q.Start(context.Background())
	if err := q.Enqueue("a"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("worker not started")
	}
	if err := q.Enqueue("a", "a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue("c"); err == nil {
		t.Fatal("queue capacity not enforced")
	}
	done := make(chan struct{})
	go func() { q.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("queue did not cancel")
	}
	if calls.Load() != 1 {
		t.Fatal("duplicate task ran")
	}
	for _, job := range q.Snapshot() {
		if job.State != "cancelled" {
			t.Fatalf("job still active: %+v", job)
		}
	}
	if err := q.Enqueue("c"); err == nil {
		t.Fatal("stopped queue accepted work")
	}
}
