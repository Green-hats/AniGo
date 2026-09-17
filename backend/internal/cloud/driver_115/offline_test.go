package driver115

import (
	"context"
	"net/url"
	"testing"

	"github.com/greenhats/anigo/internal/domain"
)

func TestOfflineStatusPaginationAndSafeRetry(t *testing.T) {
	p := newPan115()
	deletes, adds, pages := 0, 0, 0
	p.reqFn = func(ctx context.Context, cfg *domain.Config, method, rawURL string, form url.Values) (map[string]interface{}, error) {
		u, _ := url.Parse(rawURL)
		switch u.Query().Get("ac") {
		case "task_lists":
			pages++
			status, hash := float64(2), "done"
			if form.Get("page") == "2" {
				status, hash = -1, "failed"
			}
			return map[string]interface{}{"state": true, "page_count": float64(2), "tasks": []interface{}{map[string]interface{}{"info_hash": hash, "status": status}}}, nil
		case "task_del":
			deletes++
			if form.Get("hash[0]") != "failed" || form.Get("flag") != "0" {
				t.Fatal("unsafe task deletion")
			}
		case "add_task_url":
			adds++
		default:
			t.Fatalf("unexpected request %s", rawURL)
		}
		return map[string]interface{}{"state": true}, nil
	}
	cfg := &domain.Config{}
	tasks, err := p.OfflineTasks(context.Background(), cfg)
	if err != nil || len(tasks) != 2 || tasks[0].State != "completed" || tasks[1].State != "failed" || pages != 2 {
		t.Fatalf("bad tasks: %+v %v", tasks, err)
	}
	if err := p.RetryOfflineTask(context.Background(), cfg, "done", "magnet:done", "file.mkv"); err != nil {
		t.Fatal(err)
	}
	if deletes != 0 || adds != 0 {
		t.Fatal("completed task retried")
	}
	if err := p.RetryOfflineTask(context.Background(), cfg, "failed", "magnet:failed", "file.mkv"); err != nil {
		t.Fatal(err)
	}
	if deletes != 1 || adds != 1 {
		t.Fatalf("retry not submitted: delete=%d add=%d", deletes, adds)
	}
}
