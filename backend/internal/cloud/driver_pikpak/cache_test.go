package driverpikpak

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

func TestRefreshSnapshotRequestCountsAndInvalidation(t *testing.T) {
	lists, dirs, creates := 0, 0, 0
	p, cfg := testDriver(t, func(w http.ResponseWriter, r *http.Request) {
		if authReply(w, r) {
			return
		}
		switch {
		case r.URL.Path == "/drive/v1/tasks":
			lists++
			reply(w, map[string]any{"tasks": []any{}})
		case r.Method == "DELETE":
			reply(w, map[string]any{})
		case r.Method == "POST":
			creates++
			reply(w, map[string]any{"task": map[string]string{"id": fmt.Sprint(creates), "phase": "PHASE_TYPE_RUNNING"}})
		default:
			dirs++
			reply(w, map[string]any{"files": []any{map[string]string{"id": "folder", "name": "shows", "kind": "drive#folder"}}})
		}
	})
	ctx := domain.WithCloudCache(context.Background())
	for i := 0; i < 20; i++ {
		magnet := fmt.Sprintf("magnet:?xt=urn:btih:%040x", i)
		id, err := p.SubmitOfflineTask(ctx, cfg, magnet, "/shows", false)
		if err != nil || id == "" {
			t.Fatal(id, err)
		}
	}
	if lists != 1 || dirs != 1 || creates != 20 {
		t.Fatalf("requests tasks=%d folders=%d creates=%d", lists, dirs, creates)
	}
	// A submitted task must immediately enter the snapshot for deduplication.
	if err := p.AddOfflineTask(ctx, cfg, "magnet:?xt=urn:btih:"+strings.Repeat("0", 40), "/shows"); err != nil {
		t.Fatal(err)
	}
	if creates != 20 {
		t.Fatal("cached submission duplicated")
	}
	if _, err := p.OfflineTasks(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if lists != 1 {
		t.Fatal("snapshot not shared by reconciliation")
	}
	// Expiry and an interactive check both demand fresh upstream state.
	p.taskCacheUntil = time.Time{}
	if _, err := p.OfflineTasks(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := p.OfflineTasks(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if lists != 3 {
		t.Fatal("snapshot did not expire/refresh")
	}
	changed := cfg.Clone()
	changed.PikpakEmail = "other@example.com"
	if err := p.AddOfflineTask(ctx, changed, "magnet:?xt=urn:btih:"+strings.Repeat("b", 40), "/shows"); err != nil {
		t.Fatal(err)
	}
	if lists != 4 || dirs != 2 {
		t.Fatal("account switch reused old cache")
	}
	if err := p.DeleteDir(ctx, changed, "/shows"); err != nil {
		t.Fatal(err)
	}
	if err := p.AddOfflineTask(ctx, changed, "magnet:?xt=urn:btih:"+strings.Repeat("c", 40), "/shows"); err != nil {
		t.Fatal(err)
	}
	if lists != 5 || dirs != 4 {
		t.Fatalf("delete did not invalidate caches: %d %d", lists, dirs)
	}
	t.Logf("20 submissions: task listings=1, parent lookups=1; dedup, TTL, account and delete invalidation verified")
}

func TestFolderCacheSurvivesAuthenticationRefresh(t *testing.T) {
	calls := 0
	p, cfg := testDriver(t, func(w http.ResponseWriter, r *http.Request) {
		if authReply(w, r) {
			return
		}
		calls++
		if calls == 1 {
			w.WriteHeader(401)
			reply(w, map[string]any{"error": "unauthenticated", "error_code": 16})
			return
		}
		if r.URL.Query().Get("parent_id") == "folder" {
			reply(w, map[string]any{"files": []any{}})
			return
		}
		reply(w, map[string]any{"files": []any{map[string]string{"id": "folder", "name": "shows", "kind": "drive#folder"}}})
	})
	if _, err := p.ListDir(context.Background(), cfg, "/shows"); err != nil {
		t.Fatal(err)
	}
	if p.folders["/shows"].id != "folder" {
		t.Fatal("folder was not recached after auth refresh")
	}
}
