package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

func TestRSSAndAIRequestsCancelWithTask(t *testing.T) {
	for _, stage := range []string{"rss", "ai"} {
		t.Run(stage, func(t *testing.T) {
			cfg := reviewConfig(t)
			feed := reviewFeed(t, cfg)
			entered := make(chan struct{})
			exited := make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				close(entered)
				select {
				case <-r.Context().Done():
					close(exited)
				case <-release:
				}
			}))
			defer func() {
				close(release)
				server.Close()
			}()
			ani := domain.DefaultAni()
			ani.URL = feed
			if stage == "rss" {
				ani.URL = server.URL
			} else {
				if err := cfg.SetConfigRaw([]byte(`{"aiBaseURL":"` + server.URL + `"}`)); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := NewRssService(cfg, nil).GetItems(ctx, ani); done <- err }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("request did not enter slow stage")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation error lost: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("task did not stop")
			}
			select {
			case <-exited:
			case <-time.After(time.Second):
				t.Fatal("upstream request did not stop")
			}
		})
	}
}

func TestDownloadLockWaitCanBeCancelled(t *testing.T) {
	d, _, _ := newTestDownloadService(t, cloudFn{&NoopDriver{}})
	d.gate <- struct{}{}
	defer func() { <-d.gate }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- d.DownloadAni(ctx, domain.DefaultAni()) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled job waited on download lock")
	}
}
