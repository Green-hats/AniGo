package rss

import (
	"net/url"
	"testing"
)

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