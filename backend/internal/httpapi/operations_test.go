package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/greenhats/anigo/internal/domain"
)

func TestTaskHistoryPaginationAndAccountIsolation(t *testing.T) {
	s := newTestServer(t)
	a := domain.DefaultAni()
	for i := 0; i < 25; i++ {
		a.DownloadTasks = append(a.DownloadTasks, domain.DownloadTask{Hash: "old", Episode: float64(i + 1), State: "completed"})
	}
	a.DownloadTasks = append(a.DownloadTasks, domain.DownloadTask{AccountID: "other-account", Hash: "private", Episode: 99, State: "failed"})
	if err := s.cfg.SaveAniList([]*domain.Ani{a}); err != nil {
		t.Fatal(err)
	}
	res := doReq(t, s, "POST", "/api/taskHistory", map[string]any{"id": a.ID, "offset": 20, "limit": 20})
	var data struct {
		Code int `json:"code"`
		Data struct {
			Total int                   `json:"total"`
			Items []domain.DownloadTask `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data.Code != 200 || data.Data.Total != 25 || len(data.Data.Items) != 5 {
		t.Fatalf("bad page: %s", res.Body.String())
	}
	for _, task := range data.Data.Items {
		if task.AccountID == "other-account" {
			t.Fatal("foreign task leaked")
		}
	}
}
func TestClearCacheEndpointSucceeds(t *testing.T) {
	s := newTestServer(t)
	// Verify the endpoint completes without making a network request to the no-op driver.
	res := decodeResult(t, doReq(t, s, "POST", "/api/clearCache", nil))
	if res.Code != 200 {
		t.Fatal(res)
	}
}
