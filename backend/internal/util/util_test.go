package util

import (
	"testing"

	"github.com/greenhats/anigo/internal/domain"
)

func TestFormatSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{-1, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1.00 KiB"},
		{1536, "1.50 KiB"},
		{1024 * 1024, "1.00 MiB"},
		{5 * 1024 * 1024, "5.00 MiB"},
		{1024 * 1024 * 1024, "1.00 GiB"},
		{2 * 1024 * 1024 * 1024, "2.00 GiB"},
		{1536 * 1024 * 1024, "1.50 GiB"},
		{1024 * 1024 * 1024 * 1024, "1.00 TiB"},
	}
	for _, c := range cases {
		if got := FormatSize(c.in); got != c.want {
			t.Errorf("FormatSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsVideo(t *testing.T) {
	videos := []string{"a.mp4", "b.MKV", "c.webm", "d.flv", "e.rmvb", "f.ts", "g.mov", "h.wmv", "i.avi"}
	for _, v := range videos {
		if !IsVideo(v) {
			t.Errorf("IsVideo(%q) = false, want true", v)
		}
	}
	notVideos := []string{"a.txt", "b.ass", "c.mp3", "d", "noext"}
	for _, v := range notVideos {
		if IsVideo(v) {
			t.Errorf("IsVideo(%q) = true, want false", v)
		}
	}
}

func TestIsSubtitle(t *testing.T) {
	subs := []string{"a.ass", "b.SRT", "c.ssa", "d.mks", "e.sup", "f.pgs"}
	for _, s := range subs {
		if !IsSubtitle(s) {
			t.Errorf("IsSubtitle(%q) = false, want true", s)
		}
	}
	if IsSubtitle("a.mp4") {
		t.Error("IsSubtitle(a.mp4) = true, want false")
	}
}

func TestVideoMimeType(t *testing.T) {
	cases := map[string]string{
		"a.mp4": "video/mp4",
		"b.MKV": "video/x-matroska",
		"c.avi": "video/x-msvideo",
		"d.webm": "video/webm",
		"e.unk": "",
		"noext": "",
	}
	for in, want := range cases {
		if got := VideoMimeType(in); got != want {
			t.Errorf("VideoMimeType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGetPinyin(t *testing.T) {
	cases := []struct{ in, want string }{
		{"中文", "zhongwen"},
		{"简单", "jiandan"},
		{"abc", "abc"},
		{"", ""},
		{"a中", "azhong"},
	}
	for _, c := range cases {
		if got := GetPinyin(c.in); got != c.want {
			t.Errorf("GetPinyin(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGetPinyinInitials(t *testing.T) {
	cases := []struct{ in, want string }{
		{"中文", "zw"},
		{"简单", "jd"},
		{"abc", "abc"},
		{"a中", "az"},
	}
	for _, c := range cases {
		if got := GetPinyinInitials(c.in); got != c.want {
			t.Errorf("GetPinyinInitials(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestWeekSortIndex(t *testing.T) {
	labels := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	// 排序从今天开始倒排循环：今天的索引为 0，昨天的为 1，……，明天的为 6
	today := int(domain.Now().Weekday())
	for i, l := range labels {
		want := (today - i + 7) % 7
		if got := WeekSortIndex(l); got != want {
			t.Errorf("%s 索引 = %d, want %d (today=%d)", l, got, want, today)
		}
	}
	// 未知标签返回大值
	if got := WeekSortIndex("不存在的标签"); got != 1<<30 {
		t.Errorf("未知标签索引 = %d, want %d", got, 1<<30)
	}
}