package driverpikpak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

func testDriver(t *testing.T, handler http.HandlerFunc) (*PikPak, *domain.Config) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	p := newPikPak()
	p.userBase, p.driveBase = server.URL, server.URL
	cfg := domain.DefaultConfig()
	cfg.DownloadToolType, cfg.PikpakEmail, cfg.PikpakPassword = "pikpak", "test@example.com", "password"
	return p, cfg
}
func reply(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func authReply(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/v1/shield/captcha/init":
		reply(w, map[string]any{"captcha_token": "captcha"})
		return true
	case "/v1/auth/signin", "/v1/auth/token":
		reply(w, map[string]any{"access_token": "access", "refresh_token": "refresh", "sub": "user", "expires_in": 3600})
		return true
	}
	return false
}
func TestSessionRefreshCredentialChangeAndConcurrency(t *testing.T) {
	var logins, refreshes atomic.Int32
	p, cfg := testDriver(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/signin":
			logins.Add(1)
		case "/v1/auth/token":
			refreshes.Add(1)
		}
		if authReply(w, r) {
			return
		}
		if r.Header.Get("Authorization") != "Bearer access" {
			t.Error("missing session")
		}
		if r.Header.Get("User-Agent") != UserAgent {
			t.Error("wrong user agent")
		}
		reply(w, map[string]any{"files": []any{}})
	})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, err := p.Login(context.Background(), true, cfg); !ok || err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if logins.Load() != 1 {
		t.Fatalf("login not reused: %d", logins.Load())
	}
	p.expiry = time.Time{}
	if _, err := p.ListDir(context.Background(), cfg, "/"); err != nil {
		t.Fatal(err)
	}
	if refreshes.Load() != 1 {
		t.Fatal("expired session not refreshed")
	}
	changed := cfg.Clone()
	changed.PikpakPassword = "changed"
	if ok, err := p.Login(context.Background(), true, changed); !ok || err != nil {
		t.Fatal(err)
	}
	if logins.Load() != 2 {
		t.Fatal("changed credentials reused old session")
	}
}
func TestAuthAndCaptchaRetriesAreBounded(t *testing.T) {
	for _, kind := range []string{"captcha", "auth"} {
		t.Run(kind, func(t *testing.T) {
			attempts := 0
			p, cfg := testDriver(t, func(w http.ResponseWriter, r *http.Request) {
				if authReply(w, r) {
					return
				}
				attempts++
				if kind == "captcha" {
					reply(w, map[string]any{"error_code": 9, "error": "captcha_invalid"})
				} else {
					w.WriteHeader(401)
					reply(w, map[string]any{"error_code": 16, "error": "unauthenticated"})
				}
			})
			if _, err := p.ListDir(context.Background(), cfg, "/"); err == nil {
				t.Fatal("rejected API reported success")
			}
			if attempts != 2 {
				t.Fatalf("unbounded or absent retry: %d", attempts)
			}
		})
	}
}
func TestHumanVerificationAndMalformedResponses(t *testing.T) {
	for _, kind := range []string{"human", "html", "empty"} {
		t.Run(kind, func(t *testing.T) {
			p, cfg := testDriver(t, func(w http.ResponseWriter, r *http.Request) {
				if kind == "human" {
					reply(w, map[string]any{"url": "https://verify.example", "captcha_token": "pending"})
					return
				}
				if authReply(w, r) {
					return
				}
				if kind == "html" {
					w.WriteHeader(502)
					io.WriteString(w, "<html>bad gateway</html>")
				} else {
					reply(w, map[string]any{})
				}
			})
			if ok, err := p.Login(context.Background(), true, cfg); ok || err == nil {
				t.Fatal("invalid response accepted")
			}
			if p.GetLoginStatus().OK {
				t.Fatal("invalid login status")
			}
		})
	}
}
func TestFilesPaginationPlaybackAndMissingPaths(t *testing.T) {
	p, cfg := testDriver(t, func(w http.ResponseWriter, r *http.Request) {
		if authReply(w, r) {
			return
		}
		if r.URL.Path == filesPath+"/video" {
			reply(w, map[string]any{"links": map[string]any{"application/octet-stream": map[string]string{"url": "https://cdn.example/original"}}, "web_content_link": "https://cdn.example/fallback"})
			return
		}
		if r.URL.Query().Get("parent_id") == "folder" {
			reply(w, map[string]any{"files": []any{map[string]any{"id": "video", "name": "番剧 E01.mkv", "kind": "drive#file", "size": "123"}}})
			return
		}
		if r.URL.Query().Get("page_token") == "second" {
			reply(w, map[string]any{"files": []any{map[string]any{"id": "folder", "name": "番剧", "kind": "drive#folder"}}})
			return
		}
		reply(w, map[string]any{"files": []any{map[string]any{"id": "trash", "name": "trash", "trashed": true}}, "next_page_token": "second"})
	})
	files, err := p.ListDir(context.Background(), cfg, "/番剧")
	if err != nil || len(files) != 1 || files[0].PickCode != "video" || files[0].Size != 123 {
		t.Fatalf("bad files: %+v %v", files, err)
	}
	link, err := p.FileURL(context.Background(), cfg, "/番剧/番剧 E01.mkv")
	if err != nil || link != "https://cdn.example/original" {
		t.Fatalf("bad playback: %s %v", link, err)
	}
	if exists, err := p.FileExists(context.Background(), cfg, "/missing/file"); exists || err != nil {
		t.Fatalf("missing path: %v %v", exists, err)
	}
	if err := p.DeleteDir(context.Background(), cfg, "/"); err == nil {
		t.Fatal("root deletion accepted")
	}
}
func TestOfflineSubmitFolderLayoutDedupAndRetry(t *testing.T) {
	magnet := "magnet:?xt=urn:btih:" + strings.Repeat("a", 40)
	taskPhase := ""
	creates, retries := 0, 0
	folders := map[string]string{}
	p, cfg := testDriver(t, func(w http.ResponseWriter, r *http.Request) {
		if authReply(w, r) {
			return
		}
		if r.URL.Path == "/drive/v1/tasks" {
			tasks := []any{}
			if taskPhase != "" {
				tasks = append(tasks, map[string]any{"id": "task", "phase": taskPhase, "params": map[string]string{"url": magnet}})
			}
			reply(w, map[string]any{"tasks": tasks})
			return
		}
		if r.URL.Path == "/drive/v1/task" {
			if r.URL.Query().Get("id") != "task" || r.URL.Query().Get("create_type") != "RETRY" {
				t.Error("wrong retry query")
			}
			retries++
			reply(w, map[string]any{})
			return
		}
		if r.Method == "GET" {
			parent := r.URL.Query().Get("parent_id")
			files := []any{}
			for key, id := range folders {
				parts := strings.SplitN(key, "|", 2)
				if parts[0] == parent {
					files = append(files, map[string]string{"id": id, "name": parts[1], "kind": "drive#folder"})
				}
			}
			reply(w, map[string]any{"files": files})
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["kind"] == "drive#folder" {
			id := fmt.Sprintf("dir%d", len(folders))
			folders[body["parent_id"].(string)+"|"+body["name"].(string)] = id
			reply(w, map[string]any{"file": map[string]string{"id": id}})
			return
		}
		if body["parent_id"] != "dir1" || body["upload_type"] != "UPLOAD_TYPE_URL" {
			t.Errorf("bad destination: %v", body)
		}
		creates++
		taskPhase = "PHASE_TYPE_RUNNING"
		reply(w, map[string]any{"task": map[string]string{"id": "task", "phase": taskPhase}})
	})
	for i := 0; i < 2; i++ {
		if err := p.AddOfflineTask(context.Background(), cfg, magnet, "/番剧/E01"); err != nil {
			t.Fatal(err)
		}
	}
	if creates != 1 || len(folders) != 2 {
		t.Fatal("duplicate submission or wrong folders")
	}
	taskPhase = "PHASE_TYPE_ERROR"
	statuses, err := p.OfflineTasks(context.Background(), cfg)
	if err != nil || len(statuses) != 1 || statuses[0].State != "failed" || statuses[0].Hash != strings.Repeat("a", 40) {
		t.Fatalf("bad status: %+v %v", statuses, err)
	}
	if err := p.RetryOfflineTask(context.Background(), cfg, statuses[0].Hash, magnet, "/番剧/E01"); err != nil {
		t.Fatal(err)
	}
	if retries != 1 || creates != 1 {
		t.Fatal("retry recreated task")
	}
	taskPhase = "PHASE_TYPE_COMPLETE"
	if err := p.RetryOfflineTask(context.Background(), cfg, statuses[0].Hash, magnet, "/番剧/E01"); err != nil {
		t.Fatal(err)
	}
	statuses, err = p.OfflineTasks(context.Background(), cfg)
	if err != nil || statuses[0].State != "completed" || retries != 1 {
		t.Fatal("completed task was retried")
	}
}
func TestTaskPaginationAndBase32Hash(t *testing.T) {
	p, cfg := testDriver(t, func(w http.ResponseWriter, r *http.Request) {
		if authReply(w, r) {
			return
		}
		if r.URL.Query().Get("next_page_token") == "next" {
			reply(w, map[string]any{"tasks": []any{map[string]any{"phase": "PHASE_TYPE_COMPLETE", "params": map[string]string{"url": "magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}}}})
			return
		}
		reply(w, map[string]any{"tasks": []any{}, "next_page_token": "next"})
	})
	tasks, err := p.OfflineTasks(context.Background(), cfg)
	if err != nil || len(tasks) != 1 || tasks[0].Hash != strings.Repeat("0", 40) {
		t.Fatalf("pagination/hash failure: %+v %v", tasks, err)
	}
}
func TestCancellationStopsRequestAndGateWait(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	p, cfg := testDriver(t, func(w http.ResponseWriter, r *http.Request) {
		if authReply(w, r) {
			return
		}
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := p.ListDir(ctx, cfg, "/"); done <- err }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("request not started")
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := p.ListDir(cancelled, cfg, "/"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("request ignored cancellation")
	}
}

func TestFileCaptchaActionAnd401RefreshRecover(t *testing.T) {
	detailCalls, captchaCalls, refreshCalls := 0, 0, 0
	p, cfg := testDriver(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/shield/captcha/init" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if strings.HasPrefix(body["action"].(string), "GET:") {
				captchaCalls++
				if body["action"] != "GET:/drive/v1/files" {
					t.Errorf("wrong captcha action: %v", body["action"])
				}
				meta := body["meta"].(map[string]any)
				if meta["user_id"] != "user" || !strings.HasPrefix(meta["captcha_sign"].(string), "1.") {
					t.Error("missing signed captcha metadata")
				}
			}
		}
		if r.URL.Path == "/v1/auth/token" {
			refreshCalls++
		}
		if authReply(w, r) {
			return
		}
		detailCalls++
		if detailCalls == 1 {
			w.WriteHeader(401)
			reply(w, map[string]any{"error_code": 16, "error": "unauthenticated"})
			return
		}
		if detailCalls == 2 {
			reply(w, map[string]any{"error_code": 9, "error": "captcha_invalid"})
			return
		}
		reply(w, map[string]any{"web_content_link": "https://cdn.example/video"})
	})
	link, err := p.FileURLByPickCode(context.Background(), cfg, "video")
	if err != nil || link != "https://cdn.example/video" || detailCalls != 3 || captchaCalls != 1 || refreshCalls != 1 {
		t.Fatalf("recovery failed: %s %v (%d,%d,%d)", link, err, detailCalls, captchaCalls, refreshCalls)
	}
}

func TestListFailureDoesNotCreateFoldersOrRetryMutation(t *testing.T) {
	mutations := 0
	p, cfg := testDriver(t, func(w http.ResponseWriter, r *http.Request) {
		if authReply(w, r) {
			return
		}
		if r.URL.Path == "/drive/v1/tasks" {
			reply(w, map[string]any{"tasks": []any{}})
			return
		}
		if r.Method == "POST" {
			mutations++
		}
		w.WriteHeader(503)
		reply(w, map[string]any{"error": "unavailable"})
	})
	if err := p.AddOfflineTask(context.Background(), cfg, "magnet:?xt=urn:btih:"+strings.Repeat("b", 40), "/folder"); err == nil {
		t.Fatal("list failure accepted")
	}
	if mutations != 0 {
		t.Fatal("list error was treated as a missing folder")
	}
	// At root, a mutation error must be returned without an ambiguous retry.
	if err := p.AddOfflineTask(context.Background(), cfg, "magnet:?xt=urn:btih:"+strings.Repeat("b", 40), "/"); err == nil {
		t.Fatal("submit failure accepted")
	}
	if mutations != 1 {
		t.Fatalf("mutation retried: %d", mutations)
	}
}
