package service

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/store"
)

func reviewConfig(t *testing.T) *ConfigService {
	t.Helper()
	cfg, err := NewConfigService(store.NewJSONStore(t.TempDir()), store.NewTTLCache())
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func reviewFeed(t *testing.T, cfg *ConfigService) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<rss><channel><item><title>Review 01 1080p</title><enclosure url="magnet:?xt=urn:btih:bbbb" /></item><item><title>Review 01 2160p</title><enclosure url="magnet:?xt=urn:btih:aaaa" /></item></channel></rss>`)
		case "/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			content := `[{"rawTitle":"Review 01 2160p","title":"Review","episode":1,"resolution":"2160p"},{"rawTitle":"Review 01 1080p","title":"Review","episode":1,"resolution":"1080p"}]`
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	raw, _ := json.Marshal(map[string]any{"aiEnabled": true, "aiApiKey": "local-test", "aiBaseURL": server.URL, "autoDisabled": true, "standbyRss": false, "delayedDownload": 0})
	if err := cfg.SetConfigRaw(raw); err != nil {
		t.Fatal(err)
	}
	return server.URL + "/feed"
}

func TestReviewBestVersionSurvivesPipeline(t *testing.T) {
	cfg := reviewConfig(t)
	ani := domain.DefaultAni()
	ani.Title, ani.URL = "Review", reviewFeed(t, cfg)
	items, err := NewRssService(cfg, nil).GetItems(context.Background(), ani)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items", len(items))
	}
	if items[0].Resolution != "2160p" {
		t.Fatalf("best version discarded: got %s, want 2160p", items[0].Resolution)
	}
}

type reviewFailDriver struct{ NoopDriver }

func (*reviewFailDriver) Login(context.Context, bool, *domain.Config) (bool, error) { return true, nil }
func (*reviewFailDriver) AddOfflineTask(context.Context, *domain.Config, string, string) error {
	return errors.New("submission rejected")
}

func TestReviewFailedSubmissionKeepsSubscriptionEnabled(t *testing.T) {
	cfg := reviewConfig(t)
	ani := domain.DefaultAni()
	ani.Title, ani.URL, ani.TotalEpisodeNumber = "Review", reviewFeed(t, cfg), 1
	if err := cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
		t.Fatal(err)
	}
	rss := NewRssService(cfg, nil)
	downloader := NewDownloadService(cfg, rss, cloudFn{drv: &reviewFailDriver{}}, store.NewTTLCache(), nil, nil, nil)
	downloader.DownloadAni(context.Background(), ani)
	ani = cfg.AniList()[0]
	if !ani.Enable {
		t.Fatalf("subscription disabled even though every submission failed; downloaded=%v", ani.Downloaded)
	}
}

func TestReviewConfigConcurrentReadAndUpdate(t *testing.T) {
	cfg := reviewConfig(t)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			_ = fmt.Sprint(cfg.Get().RssSleepMinutes)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 1; i < 30; i++ {
			if err := cfg.SetConfigRaw([]byte(fmt.Sprintf(`{"rssSleepMinutes":%d}`, i))); err != nil {
				t.Error(err)
			}
		}
	}()
	wg.Wait()
}

func TestReviewInvalidBackupPreservesOriginalFile(t *testing.T) {
	cfg := reviewConfig(t)
	path := filepath.Join(cfg.Dir(), "config.v2.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "invalid.zip")
	out, err := os.Create(backup)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	entry, err := zw.Create("config.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprint(entry, "invalid JSON")
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	out.Close()
	if err := cfg.ImportConfig(backup); err == nil {
		t.Fatal("invalid backup accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("invalid backup overwrote valid config: %q", after)
	}
}
