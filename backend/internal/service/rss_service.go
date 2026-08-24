package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/provider/ai"
	"github.com/greenhats/anigo/internal/rename"
	"github.com/greenhats/anigo/internal/rss"
)

// RssService 负责 RSS 聚合、过滤与去重。
// 当 AI 启用并配置了 apiKey 时，用 AI 解析标题集数，失败回退正则。
type RssService struct {
	cfg *ConfigService
	ai  *ai.DeepSeek
}

// NewRssService 创建 RSS 服务。
func NewRssService(cfg *ConfigService) *RssService {
	return &RssService{
		cfg: cfg,
		ai:  ai.New(cfg.Get()),
	}
}

// ReloadAI 在 AI 配置变更后重建客户端。
func (s *RssService) ReloadAI() {
	s.ai = ai.New(s.cfg.Get())
}

// AIPing 测试 AI 连通性，返回模型回复（未配置时返回错误）。
func (s *RssService) AIPing(ctx context.Context) (string, error) {
	if s.ai == nil {
		return "", errAINotConfigured
	}
	return s.ai.Ping(ctx)
}

// errAINotConfigured 是 AI 未配置错误。
var errAINotConfigured = &aiNotConfiguredError{}

type aiNotConfiguredError struct{}

func (e *aiNotConfiguredError) Error() string { return "AI 未配置 apiKey" }

// GetItems 聚合主 + 备用 RSS 条目，按剧集排序。
func (s *RssService) GetItems(ani *domain.Ani) []*domain.Item {
	cfg := s.cfg.Get()
	subgroup := ani.Subgroup
	if strings.TrimSpace(subgroup) == "" {
		subgroup = "未知字幕组"
	}
	var items []*domain.Item
	for _, it := range getItems(cfg, s.ai, ani, ani.URL, subgroup) {
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
		for _, it := range getItems(cfg, s.ai, clone, sr.URL, subgroup) {
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
// 先正则解析；若 AI 可用，再批量用 AI 精化集号。
func getItems(cfg *domain.Config, aiclient *ai.DeepSeek, ani *domain.Ani, rssURL, subgroupName string) []*domain.Item {
	xmlBody, err := rss.GetRSS(cfg, rssURL)
	if err != nil {
		return nil
	}
	items := rss.Parse(cfg, ani, rssURL, subgroupName, xmlBody)
	if len(items) == 0 || aiclient == nil || !cfg.AiEnabled {
		return items
	}
	// 批量 AI 解析：提取每个条目的原始标题，让 AI 重算集号
	titles := make([]string, len(items))
	for i, it := range items {
		titles[i] = it.Title
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	parsed, err := aiclient.Parse(ctx, titles)
	if err != nil {
		// AI 失败 → 回退正则结果
		return items
	}
	var refined []*domain.Item
	for i, it := range items {
		pt := parsed[i]
		if pt.Episode <= 0 {
			continue // AI 也无法判断，丢弃
		}
		clone := it.Clone()
		clone.Title = pt.Title
		clone.Subgroup = pt.Subgroup
		if rename.RenameWithEpisode(ani, clone, cfg, pt.Episode) {
			refined = append(refined, clone)
		}
	}
	if len(refined) > 0 {
		return refined
	}
	return items
}