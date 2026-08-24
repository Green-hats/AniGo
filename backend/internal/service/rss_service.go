package service

import (
	"sort"
	"strings"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/rss"
)

// RssService 负责 RSS 聚合、过滤与去重。
type RssService struct {
	cfg *ConfigService
}

// NewRssService 创建 RSS 服务。
func NewRssService(cfg *ConfigService) *RssService {
	return &RssService{cfg: cfg}
}

// GetItems 聚合主 + 备用 RSS 条目，按剧集排序。
func (s *RssService) GetItems(ani *domain.Ani) []*domain.Item {
	cfg := s.cfg.Get()
	subgroup := ani.Subgroup
	if strings.TrimSpace(subgroup) == "" {
		subgroup = "未知字幕组"
	}
	var items []*domain.Item
	for _, it := range getItems(cfg, ani, ani.URL, subgroup) {
		it.Master = true
		items = append(items, it)
	}

	if !cfg.StandbyRss {
		sort.SliceStable(items, func(i, j int) bool { return items[i].Episode < items[j].Episode })
		return items
	}

	for _, sr := range ani.StandbyRssList {
		time.Sleep(time.Second)
		subgroup = sr.Label
		if strings.TrimSpace(subgroup) == "" {
			subgroup = "未知字幕组"
		}
		clone := ani.Clone()
		clone.Offset = sr.Offset
		for _, it := range getItems(cfg, clone, sr.URL, subgroup) {
			it.Master = false
			items = append(items, it)
		}
	}

	items = rss.DistinctItems(items, cfg.Coexist)
	sort.SliceStable(items, func(i, j int) bool { return items[i].Episode < items[j].Episode })
	return items
}

// CurrentEpisodeNumber 计算当前集数。
func (s *RssService) CurrentEpisodeNumber(ani *domain.Ani, items []*domain.Item) int {
	cfg := s.cfg.Get()
	if cfg.StandbyRss && cfg.Coexist {
		var master []*domain.Item
		for _, it := range items {
			if it.Master {
				master = append(master, it)
			}
		}
		items = master
	}
	var cleaned []*domain.Item
	for _, it := range items {
		if it.Episode == float64(int(it.Episode)) {
			cleaned = append(cleaned, it)
		}
	}
	if len(cleaned) == 0 {
		return 0
	}
	if ani.DownloadNew {
		max := 0
		for _, it := range cleaned {
			if int(it.Episode) > max {
				max = int(it.Episode)
			}
		}
		return max
	}
	return len(cleaned)
}

// getItems 解析单个 RSS 源为条目（最新在前），过滤并重命名。
func getItems(cfg *domain.Config, ani *domain.Ani, rssURL, subgroupName string) []*domain.Item {
	xmlBody, err := rss.GetRSS(cfg, rssURL)
	if err != nil {
		return nil
	}
	return rss.Parse(cfg, ani, rssURL, subgroupName, xmlBody)
}