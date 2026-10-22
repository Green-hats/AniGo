package garden

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

type testConfig struct{}

func (testConfig) Get() *domain.Config { return &domain.Config{} }

func TestListSubjectsChoosesCurrentQuarter(t *testing.T) {
	for _, tc := range []struct {
		date, season string
	}{
		{"2026-01-01T00:00:00+08:00", "2026-01"},
		{"2026-03-31T23:59:59+08:00", "2026-01"},
		{"2026-04-01T00:00:00+08:00", "2026-04"},
		{"2026-06-30T23:59:59+08:00", "2026-04"},
		{"2026-07-01T00:00:00+08:00", "2026-07"},
		{"2026-09-30T15:59:59Z", "2026-07"},
		{"2026-09-30T16:00:00Z", "2026-10"},
		{"2026-12-31T15:59:59Z", "2026-10"},
		{"2026-12-31T16:00:00Z", "2027-01"},
	} {
		t.Run(tc.date, func(t *testing.T) {
			g := New(testConfig{})
			g.Req = func(_ context.Context, method, rawURL string, _ io.Reader, _ string) ([]byte, int, error) {
				want := "https://bgm.animes.garden/calendar?season=" + tc.season
				if method != "GET" || rawURL != want {
					t.Fatalf("request = %s %s, want GET %s", method, rawURL, want)
				}
				return []byte(fmt.Sprintf(`{"ok":true,"data":{"seasons":[%q],"calendar":[[],[],[],[],[],[],[]],"web":[]}}`, tc.season)), 200, nil
			}
			now, err := time.Parse(time.RFC3339, tc.date)
			if err != nil {
				t.Fatal(err)
			}
			list, err := g.listSubjects(context.Background(), now)
			if err != nil {
				t.Fatal(err)
			}
			if list == nil || len(list) != 0 {
				t.Fatalf("empty calendar = %#v, want empty JSON array", list)
			}
		})
	}
}

func TestListSubjectsUsesCalendarMembershipAndWeekdays(t *testing.T) {
	g := New(testConfig{})
	g.Req = func(_ context.Context, _, _ string, _ io.Reader, _ string) ([]byte, int, error) {
		return []byte(`{
			"ok": true,
			"subjects": [{"id":999,"name":"旧季度索引条目"}],
			"data": {
				"seasons": ["2026-10"],
				"calendar": [
					[
						{"id":101,"title":"日历内的跨月首播","onair_date":"2026-09-30","poster":"poster","alias":{"zh":["中文名"]},"search":{"include":["检索名"]}},
						{"id":"102","title":"较晚首播","onair_date":"2026-10-12","bangumi":{"images":{"large":"poster"}}},
						{"id":103,"title":"无封面"},
						{"id":104,"title":"国创","poster":"poster","bangumi":{"meta_tags":["国创"]}},
						{"id":105,"title":"国产","poster":"poster","bangumi":{"tags":["中国"]}}
					], [], [], [], [], [], []
				],
				"web": [{"id":201,"title":"WEB 批量发布","poster":"poster","bangumi":{"date":"2026-11-01"}}]
			}
		}`), 200, nil
	}
	list, err := g.listSubjects(context.Background(), time.Date(2026, 10, 23, 0, 0, 0, 0, domain.Loc()))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].WeekLabel != "星期一" || list[1].WeekLabel != "WEB" {
		t.Fatalf("calendar groups = %+v, want Monday and WEB", list)
	}
	if list[0].Season != "2026-10" || list[1].Season != "2026-10" {
		t.Fatalf("wrong season: %+v", list)
	}
	subs := list[0].Subjects
	if len(subs) != 2 || subs[0].ID != "102" || subs[1].ID != "101" {
		t.Fatalf("visible calendar subjects = %+v, want 102, 101", subs)
	}
	// 日历分组由上游确定，不能按日期过滤掉跨月首播，也不能据首播日期重排星期。
	if subs[1].Name != "中文名" || subs[1].WeekLabel != "星期一" || len(subs[1].Keywords) != 1 || subs[1].Keywords[0] != "检索名" {
		t.Fatalf("subject metadata = %+v", subs[1])
	}
	if subs[0].Name != "较晚首播" || !strings.Contains(subs[0].Cover, "/subject/102/poster.jpeg") {
		t.Fatalf("title fallback or cover = %+v", subs[0])
	}
	web := list[1].Subjects
	if len(web) != 1 || web[0].ID != "201" || web[0].WeekLabel != "WEB" || web[0].ActivedAt.Time().IsZero() {
		t.Fatalf("WEB group = %+v", web)
	}
}

func TestListSubjectsRejectsInvalidCalendar(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"upstream failure", `{"ok":false}`, 200},
		{"wrong quarter", `{"ok":true,"data":{"seasons":["2026-07"],"calendar":[[],[],[],[],[],[],[]]}}`, 200},
		{"unscoped quarters", `{"ok":true,"data":{"seasons":["2026-07","2026-10"],"calendar":[[],[],[],[],[],[],[]]}}`, 200},
		{"missing weekdays", `{"ok":true,"data":{"seasons":["2026-10"],"calendar":[]}}`, 200},
		{"bad JSON", `<html>unavailable</html>`, 200},
		{"HTTP failure", `unavailable`, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := New(testConfig{})
			g.Req = func(_ context.Context, _, _ string, _ io.Reader, _ string) ([]byte, int, error) {
				return []byte(tc.body), tc.status, nil
			}
			if _, err := g.listSubjects(context.Background(), time.Date(2026, 10, 23, 0, 0, 0, 0, domain.Loc())); err == nil {
				t.Fatal("invalid upstream calendar must return an error, not a successful empty list")
			}
		})
	}
}
