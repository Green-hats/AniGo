package httpapi

import (
	"net/http"
	"testing"

	"github.com/greenhats/anigo/internal/domain"
)

func TestArchiveAPILifecycle(t *testing.T) {
	s := newTestServer(t)
	ani := domain.DefaultAni()
	ani.ID = "archive-test"
	ani.Title = "完结番剧"
	if err := s.cfg.SaveAniList([]*domain.Ani{ani}); err != nil {
		t.Fatal(err)
	}
	for _, archived := range []bool{true, true, false} {
		w := doReq(t, s, http.MethodPost, "/api/archiveAni", map[string]interface{}{"id": ani.ID, "archived": archived})
		if res := decodeResult(t, w); res.Code != 200 {
			t.Fatalf("archive=%v: %s", archived, res.Message)
		}
		list := decodeResult(t, doReq(t, s, http.MethodPost, "/api/listAni", nil)).Data.(map[string]interface{})
		archives := list["archivedList"].([]interface{})
		active := 0
		for _, w := range list["weekList"].([]interface{}) {
			active += len(w.(map[string]interface{})["items"].([]interface{}))
		}
		if list["total"].(float64) != 1 || archived && (len(archives) != 1 || active != 0) || !archived && (len(archives) != 0 || active != 1) {
			t.Fatalf("wrong list after archive=%v: %+v", archived, list)
		}
		if archived {
			res := decodeResult(t, doReq(t, s, http.MethodPost, "/api/refreshAni", map[string]interface{}{"id": ani.ID}))
			if res.Code == 200 {
				t.Fatal("manual refresh accepted an archived subscription")
			}
		}
	}
}

func TestArchiveAPIRejectsInvalidInput(t *testing.T) {
	s := newTestServer(t)
	for _, body := range []interface{}{
		nil,
		map[string]interface{}{"id": "missing"},
		map[string]interface{}{"archived": true},
		map[string]interface{}{"id": "missing", "archived": true},
		map[string]interface{}{"id": "missing", "archived": "false"},
	} {
		if res := decodeResult(t, doReq(t, s, http.MethodPost, "/api/archiveAni", body)); res.Code == 200 {
			t.Fatalf("invalid body accepted: %+v", body)
		}
	}
}
