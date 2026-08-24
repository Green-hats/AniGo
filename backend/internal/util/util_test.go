package util

import (
	"testing"
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