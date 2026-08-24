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

func TestPickBestPerEpisodePrefersEmbed(t *testing.T) {
	items := []*domain.Item{
		{Episode: 1, Resolution: "1080p", SubtitleEmbed: "内嵌", Subgroup: "ANi", Master: true},
		{Episode: 1, Resolution: "1080p", SubtitleEmbed: "内封", Subgroup: "ANi", Master: true},
	}
	got := pickBestPerEpisode(items, "ANi")
	if len(got) != 1 || got[0].SubtitleEmbed != "内封" {
		t.Errorf("应选内封, got %+v", got[0])
	}
}

func TestPickBestPerEpisodePrefersCodec(t *testing.T) {
	items := []*domain.Item{
		{Episode: 1, Resolution: "1080p", VideoCodec: "AVC", Subgroup: "ANi", Master: true},
		{Episode: 1, Resolution: "1080p", VideoCodec: "HEVC", Subgroup: "ANi", Master: true},
	}
	got := pickBestPerEpisode(items, "ANi")
	if len(got) != 1 || got[0].VideoCodec != "HEVC" {
		t.Errorf("应选 HEVC, got %+v", got[0])
	}
}

func TestPickBestPerEpisodePrefersSource(t *testing.T) {
	items := []*domain.Item{
		{Episode: 1, Resolution: "1080p", Source: "WebRip", Subgroup: "ANi", Master: true},
		{Episode: 1, Resolution: "1080p", Source: "BDRip", Subgroup: "ANi", Master: true},
	}
	got := pickBestPerEpisode(items, "ANi")
	if len(got) != 1 || got[0].Source != "BDRip" {
		t.Errorf("应选 BDRip, got %+v", got[0])
	}
}

func TestPickBestPerEpisodePrefersColorDepth(t *testing.T) {
	items := []*domain.Item{
		{Episode: 1, Resolution: "1080p", ColorDepth: "8bit", Subgroup: "ANi", Master: true},
		{Episode: 1, Resolution: "1080p", ColorDepth: "10bit", Subgroup: "ANi", Master: true},
	}
	got := pickBestPerEpisode(items, "ANi")
	if len(got) != 1 || got[0].ColorDepth != "10bit" {
		t.Errorf("应选 10bit, got %+v", got[0])
	}
}

func TestPickBestPerEpisodePrefersSubtitleLang(t *testing.T) {
	items := []*domain.Item{
		{Episode: 1, Resolution: "1080p", SubtitleLang: "简日", Subgroup: "ANi", Master: true},
		{Episode: 1, Resolution: "1080p", SubtitleLang: "简繁日", Subgroup: "ANi", Master: true},
	}
	got := pickBestPerEpisode(items, "ANi")
	if len(got) != 1 || got[0].SubtitleLang != "简繁日" {
		t.Errorf("应选字幕更全的 简繁日, got %+v", got[0])
	}
}

func TestPickBestPerEpisodeMissingSignalFallsBack(t *testing.T) {
	// 内封但分辨率低，应选分辨率高者（分辨率优先级更高）
	items := []*domain.Item{
		{Episode: 1, Resolution: "720p", SubtitleEmbed: "内封", Subgroup: "ANi", Master: true},
		{Episode: 1, Resolution: "1080p", SubtitleEmbed: "", Subgroup: "ANi", Master: true},
	}
	got := pickBestPerEpisode(items, "ANi")
	if len(got) != 1 || got[0].Resolution != "1080p" {
		t.Errorf("分辨率应优先, got %+v", got[0])
	}
	// 信号缺失（内封 vs 空）应选内封
	items2 := []*domain.Item{
		{Episode: 2, Resolution: "1080p", SubtitleEmbed: "", Subgroup: "ANi", Master: true},
		{Episode: 2, Resolution: "1080p", SubtitleEmbed: "内嵌", Subgroup: "ANi", Master: true},
	}
	got2 := pickBestPerEpisode(items2, "ANi")
	if len(got2) != 1 || got2[0].SubtitleEmbed != "内嵌" {
		t.Errorf("缺失信号视为最低, 应选内嵌, got %+v", got2[0])
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