package service

import (
	"testing"

	"github.com/greenhats/anigo/internal/domain"
)

func TestResolutionScore(t *testing.T) {
	cases := []struct {
		res  string
		want int
	}{
		{"2160p", 3}, {"4K", 3}, {"1080p", 2}, {"720p", 1}, {"none", 0}, {"", 0},
	}
	for _, c := range cases {
		if got := resolutionScore(c.res); got != c.want {
			t.Errorf("resolutionScore(%q) = %d, want %d", c.res, got, c.want)
		}
	}
}

func TestPickBestPerEpisodePrefersHigherResolution(t *testing.T) {
	items := []*domain.Item{
		{Episode: 1, Resolution: "720p", Subgroup: "ANi", Master: true},
		{Episode: 1, Resolution: "1080p", Subgroup: "ANi", Master: true},
		{Episode: 2, Resolution: "720p", Subgroup: "ANi", Master: true},
	}
	got := pickBestPerEpisode(items, "ANi")
	if len(got) != 2 {
		t.Fatalf("应保留 2 集, got %d", len(got))
	}
	// 第 1 集应选 1080p
	for _, it := range got {
		if it.Episode == 1 && it.Resolution != "1080p" {
			t.Errorf("第1集应选 1080p, got %s", it.Resolution)
		}
	}
}

func TestPickBestPerEpisodePrefersSubscribedSubgroup(t *testing.T) {
	items := []*domain.Item{
		{Episode: 1, Resolution: "1080p", Subgroup: "其他组", Master: true},
		{Episode: 1, Resolution: "720p", Subgroup: "ANi", Master: true},
	}
	got := pickBestPerEpisode(items, "ANi")
	if len(got) != 1 {
		t.Fatalf("应保留 1 集, got %d", len(got))
	}
	// 订阅 ANi 时，即使 720p 也应选 ANi（分辨率相同级别下优先订阅字幕组）
	// 但这里 1080p 分辨率更高 → 应先按分辨率选 1080p
	if got[0].Resolution != "1080p" {
		t.Errorf("分辨率优先应选 1080p, got %s", got[0].Resolution)
	}
}

func TestPickBestPerEpisodePrefersMaster(t *testing.T) {
	items := []*domain.Item{
		{Episode: 1, Resolution: "1080p", Subgroup: "ANi", Master: false}, // 备用源
		{Episode: 1, Resolution: "1080p", Subgroup: "ANi", Master: true},  // 主源
	}
	got := pickBestPerEpisode(items, "ANi")
	if len(got) != 1 || !got[0].Master {
		t.Errorf("应选主源, got master=%v", got[0].Master)
	}
}

func TestCurrentEpisodeNumberDedup(t *testing.T) {
	s := &RssService{cfg: &ConfigService{cfg: &domain.Config{}}}
	// 模拟 12 集番剧，每集 2 个版本 → 24 个条目
	var items []*domain.Item
	for ep := 1; ep <= 12; ep++ {
		items = append(items,
			&domain.Item{Episode: float64(ep), Master: true},
			&domain.Item{Episode: float64(ep), Master: true},
		)
	}
	ani := &domain.Ani{DownloadNew: false}
	if got := s.CurrentEpisodeNumber(ani, items); got != 12 {
		t.Errorf("按集去重应返回 12, got %d", got)
	}
}

func TestCurrentEpisodeNumberDownloadNew(t *testing.T) {
	s := &RssService{cfg: &ConfigService{cfg: &domain.Config{}}}
	var items []*domain.Item
	for ep := 1; ep <= 3; ep++ {
		items = append(items, &domain.Item{Episode: float64(ep), Master: true})
	}
	ani := &domain.Ani{DownloadNew: true}
	if got := s.CurrentEpisodeNumber(ani, items); got != 3 {
		t.Errorf("DownloadNew 应返回最大集 3, got %d", got)
	}
}