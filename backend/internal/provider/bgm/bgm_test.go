package bgm

import (
	"testing"

	"github.com/greenhats/anigo/internal/domain"
)

func TestGetSeasonByName(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"间谍过家家 第二季", 2},
		{"SPY×FAMILY Season 2", 2},
		{"某番剧 第三期", 3},
		{"某番剧 第四季", 4},
		{"无季标识", 1},
		{"Re:从零 第二季", 2},
		{"某番剧 Season 12", 12},
	}
	for _, c := range cases {
		if got := GetSeasonByName(c.in); got != c.want {
			t.Errorf("GetSeasonByName(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestChineseNum(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"十", 10},
		{"十五", 15},
		{"二", 2},
		{"二十三", 23},
	}
	for _, c := range cases {
		if got := chineseNum(c.in); got != c.want {
			t.Errorf("chineseNum(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestGetSubjectIdByURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://bgm.tv/subject/477825", "477825"},
		{"https://bgm.tv/subject/123456/other", "123456"},
		{"https://example.com", ""},
	}
	for _, c := range cases {
		if got := GetSubjectIdByURL(c.in); got != c.want {
			t.Errorf("GetSubjectIdByURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGetFinalName(t *testing.T) {
	info := &domain.BgmInfo{Name: "SPY×FAMILY", NameCn: "间谍过家家"}
	if got := GetFinalName(info, false); got != "间谍过家家" {
		t.Errorf("中文名优先: %q", got)
	}
	if got := GetFinalName(info, true); got != "SPY×FAMILY" {
		t.Errorf("BgmJpName 用日文名: %q", got)
	}
	if got := GetFinalName(nil, false); got != "无标题" {
		t.Errorf("nil 返回无标题: %q", got)
	}
}

func TestGetSeasonFromTags(t *testing.T) {
	info := &domain.BgmInfo{
		NameCn: "某番剧",
		Tags:   []domain.BgmTag{{Name: "第二季", Count: 1}},
	}
	if got := GetSeason(info); got != 2 {
		t.Errorf("标签推断季数: got %d", got)
	}
}