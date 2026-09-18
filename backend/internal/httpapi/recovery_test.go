package httpapi

import (
	"github.com/greenhats/anigo/internal/domain"
	"testing"
)

func TestTaskRecoveryAPIValidatesAndPersists(t *testing.T) {
	s := newTestServer(t)
	a := domain.DefaultAni()
	a.DownloadTasks = []domain.DownloadTask{{Hash: "hash", Episode: 1, State: "exhausted", Attempts: 99}}
	if err := s.cfg.SaveAniList([]*domain.Ani{a}); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"bad", "retry"} {
		w := doReq(t, s, "POST", "/api/recoverTask", map[string]any{"id": a.ID, "hash": "hash", "episode": 1, "action": action})
		if action == "bad" {
			if s.cfg.AniByID(a.ID).DownloadTasks[0].Attempts != 99 {
				t.Fatal("invalid action mutated task")
			}
		} else {
			if w.Code != 200 || s.cfg.AniByID(a.ID).DownloadTasks[0].Attempts != 0 {
				t.Fatalf("recovery failed: %s", w.Body.String())
			}
		}
	}
}
