package rss

import (
	"net/url"
	"testing"
)

// 回归: 默认排除规则 \d-\d 会误伤标题里的编码格式(H265-10bit 中的 5-1),
// 导致字幕组 MKV 版本被整条排除, 订阅显示"共 0 个条目"。
func TestRegexpMatchExcludeEncodingFormat(t *testing.T) {
	for _, title := range []string{
		"H265-10bit 1080p AAC",
		"AV1-10bit 1080p AAC",
	} {
		if !regexpMatch(`\d-\d`, title) {
			t.Errorf("\\d-\\d 应匹配 %q (编码格式里的数字-数字)", title)
		}
	}
	// HEVC 的 E-C 不是数字-数字, 不应误伤
	if regexpMatch(`\d-\d`, "HEVC-10bit 1080p") {
		t.Error("HEVC-10bit 不应匹配 \\d-\\d")
	}
	// 单集标题不应被误判为集合并集
	if regexpMatch(`\d-\d`, "S01E04") {
		t.Error("S01E04 不应匹配 \\d-\\d")
	}
}

// 无效正则回退为字符串包含匹配(不 panic)。
func TestRegexpMatchFallbackToContains(t *testing.T) {
	if !regexpMatch("1080p[", "x1080p[y") {
		t.Error("无效正则应回退为 contains 匹配")
	}
	if regexpMatch("(", "无特殊字符") {
		t.Error("无效正则可编译时不应误匹配")
	}
}

func TestNormalizeURLEncoding(t *testing.T) {
	// animegarden 生成(已用 QueryEscape)和用户手动粘贴(未编码)都应被规范化
	cases := []string{
		"https://api.animes.garden/feed.xml?subject=545008&fansub=TSDM字幕组",
		"https://api.animes.garden/feed.xml?subject=545008&fansub=TSDM%E5%AD%97%E5%B9%95%E7%BB%84",
		"https://mikanani.me/RSS/Bangumi?bangumiId=100&subgroupid=1",
	}
	for _, c := range cases {
		got := normalizeURL(c)
		u, err := url.Parse(got)
		if err != nil {
			t.Fatalf("parse %q: %v", got, err)
		}
		q := u.Query()
		if f := q.Get("fansub"); f != "" && f != "TSDM字幕组" {
			t.Errorf("fansub 被错误解码: %q", f)
		}
	}
}