package bgm

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/store"
)

type progressConfig struct{}

func (progressConfig) Get() *domain.Config { return &domain.Config{} }

func TestAiredProgressUsesOnlyBroadcastMainEpisodes(t *testing.T) {
	now := time.Date(2026, 10, 23, 0, 0, 0, 0, domain.Loc())
	for _, tc := range []struct {
		name string
		body string
		want int
		err  bool
	}{
		{"aired count differs from episode labels", `[{"id":1,"type":0,"sort":2,"airdate":"2026-09-25"},{"id":2,"type":0,"sort":3,"airdate":"2026-10-02"},{"id":3,"type":0,"sort":4,"airdate":"2026-10-09"},{"id":4,"type":0,"sort":5,"airdate":"2026-10-16"},{"id":5,"type":0,"sort":6,"airdate":"2026-10-23"},{"id":6,"type":0,"sort":7,"airdate":"2026-10-30"}]`, 5, false},
		{"exclude specials and duplicates", `[{"id":1,"type":0,"airdate":"2026-10-01"},{"id":1,"type":0,"airdate":"2026-10-01"},{"id":2,"type":1,"airdate":"2026-10-01"},{"id":3,"type":2,"airdate":"2026-10-01"},{"id":4,"type":3,"airdate":"2026-10-01"},{"id":5,"type":0,"airdate":""}]`, 1, false},
		{"not aired yet is zero", `[{"id":1,"type":0,"airdate":"2026-10-24"}]`, 0, false},
		{"no dates is unknown", `[{"id":1,"type":0,"airdate":""}]`, 0, true},
		{"no episodes is unknown", `[]`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var episodes []*domain.BgmEpisode
			if err := json.Unmarshal([]byte(tc.body), &episodes); err != nil {
				t.Fatal(err)
			}
			got, err := countAiredEpisodes(episodes, now)
			if got != tc.want || (err != nil) != tc.err {
				t.Fatalf("got %d, %v; want %d, error=%v", got, err, tc.want, tc.err)
			}
		})
	}
}

func TestGetAiredEpsDoesNotFallBackToWikiCount(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantError  bool
	}{
		{"future episodes", `{"total":1,"data":[{"id":1,"type":0,"airdate":"2999-01-01"}]}`, 200, false},
		{"network failure", `unavailable`, 503, true},
		{"missing dates", `{"total":1,"data":[{"id":1,"type":0,"airdate":""}]}`, 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := New(progressConfig{}, nil)
			calls := 0
			b.Req = func(_ context.Context, _, rawURL string, _ io.Reader, _ string) ([]byte, int, error) {
				calls++
				if !containsQuery(rawURL, "type", "0") {
					t.Fatal("aired progress must request main episodes only")
				}
				return []byte(tc.body), tc.status, nil
			}
			got, err := b.GetAiredEps(context.Background(), &domain.BgmInfo{ID: "123", Eps: 11, TotalEpisodes: 13})
			if got != 0 || (err != nil) != tc.wantError || calls != 1 {
				t.Fatalf("got %d, %v (%d requests); must not substitute wiki count 11", got, err, calls)
			}
		})
	}
}

func containsQuery(rawURL, key, value string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && u.Query().Get(key) == value
}

func TestGetEpisodesPaginatesBeyondOneHundred(t *testing.T) {
	b := New(progressConfig{}, store.NewTTLCache())
	calls := 0
	b.Req = func(_ context.Context, _, rawURL string, _ io.Reader, _ string) ([]byte, int, error) {
		calls++
		u, _ := url.Parse(rawURL)
		if u.Query().Get("type") != "0" {
			t.Fatal("unexpected episode type")
		}
		offset, _ := strconv.Atoi(u.Query().Get("offset"))
		if offset != 0 && offset != 100 {
			t.Fatalf("unexpected offset: %d", offset)
		}
		data := []map[string]interface{}{}
		for id := offset + 1; id <= min(offset+100, 101); id++ {
			data = append(data, map[string]interface{}{"id": id, "type": 0, "airdate": "2000-01-01"})
		}
		body, _ := json.Marshal(map[string]interface{}{"total": 101, "data": data})
		return body, 200, nil
	}
	for i := 0; i < 2; i++ {
		got, err := b.GetAiredEps(context.Background(), &domain.BgmInfo{ID: "long-series", Eps: 200})
		if err != nil || got != 101 {
			t.Fatalf("got %d, %v; want 101 aired episodes", got, err)
		}
	}
	if calls != 2 {
		t.Fatalf("complete episode list should be cached, got %d requests", calls)
	}
}

func TestGetEpisodesRejectsPartialPagination(t *testing.T) {
	b := New(progressConfig{}, nil)
	b.Req = func(_ context.Context, _, rawURL string, _ io.Reader, _ string) ([]byte, int, error) {
		if containsQuery(rawURL, "offset", "0") {
			return []byte(`{"total":2,"data":[{"id":1,"type":0,"airdate":"2000-01-01"}]}`), 200, nil
		}
		return []byte(`{"total":2,"data":[]}`), 200, nil
	}
	if _, err := b.GetAiredEps(context.Background(), &domain.BgmInfo{ID: "123", Eps: 12}); err == nil {
		t.Fatal("partial episode list must not be accepted as a complete aired count")
	}
}
