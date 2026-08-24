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
// 每集只保留一个最优版本：分辨率优先（2160p>1080p>720p），
// 再优先订阅的字幕组，再优先主源（Master）。
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

	if cfg.StandbyRss {
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
	}

	// 每集选一个最优版本
	items = pickBestPerEpisode(items, ani.Subgroup)
	sort.SliceStable(items, func(i, j int) bool { return items[i].Episode < items[j].Episode })
	return items
}

// resolutionScore 将分辨率转为优先级（数值越大越优先）。
func resolutionScore(res string) int {
	switch strings.ToLower(strings.TrimSpace(res)) {
	case "2160p", "4k":
		return 3
	case "1080p":
		return 2
	case "720p":
		return 1
	default:
		return 0
	}
}

// pickBestPerEpisode 按集分组，每集只保留一个最优条目。
// 比较顺序：分辨率 > 是否匹配订阅字幕组 > 是否主源。
func pickBestPerEpisode(items []*domain.Item, subscribedSubgroup string) []*domain.Item {
	best := map[int]*domain.Item{}
	// 保持稳定：遇到更优的才替换
	better := func(cur, cand *domain.Item) bool {
		cs := resolutionScore(cur.Resolution)
		ds := resolutionScore(cand.Resolution)
		if ds != cs {
			return ds > cs
		}
		curSub := cur.Subgroup != "" && cur.Subgroup == subscribedSubgroup
		candSub := cand.Subgroup != "" && cand.Subgroup == subscribedSubgroup
		if candSub != curSub {
			return candSub
		}
		return cand.Master && !cur.Master
	}
	for _, it := range items {
		ep := int(it.Episode)
		cur, ok := best[ep]
		if !ok || better(cur, it) {
			best[ep] = it
		}
	}
	var out []*domain.Item
	for _, it := range best {
		out = append(out, it)
	}
	return out
}

// CurrentEpisodeNumber 计算当前集数（按集去重，修复多版本重复计数）。
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
	// 按集去重计数
	seen := map[int]bool{}
	for _, it := range items {
		if it.Episode == float64(int(it.Episode)) {
			seen[int(it.Episode)] = true
		}
	}
	if len(seen) == 0 {
		return 0
	}
	if ani.DownloadNew {
		max := 0
		for ep := range seen {
			if ep > max {
				max = ep
			}
		}
		return max
	}
	return len(seen)
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
			// AI 无法判断该条 → 回退正则解析结果，不丢弃
			refined = append(refined, it)
			continue
		}
		clone := it.Clone()
		clone.Title = pt.Title
		clone.Subgroup = pt.Subgroup
		clone.Resolution = pt.Resolution
		if rename.RenameWithEpisode(ani, clone, cfg, pt.Episode) {
			refined = append(refined, clone)
		}
	}
	if len(refined) > 0 {
		return filterByAI(cfg, aiclient, ani, refined)
	}
	return items
}

// filterByAI 用 AI 筛选器剔除不符合订阅规则的条目（尽力而为，失败不阻断）。
func filterByAI(cfg *domain.Config, aiclient *ai.DeepSeek, ani *domain.Ani, items []*domain.Item) []*domain.Item {
	// 无匹配/排除规则时无需 AI 筛选
	if len(ani.Match) == 0 && len(ani.Exclude) == 0 {
		return items
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	titles := make([]string, len(items))
	for i, it := range items {
		titles[i] = it.Title
	}
	keep, err := aiclient.Filter(ctx, ani, titles)
	if err != nil {
		return items // AI 筛选失败 → 全部保留
	}
	var out []*domain.Item
	for i, it := range items {
		if i < len(keep) && keep[i] {
			out = append(out, it)
		}
	}
	if len(out) == 0 {
		return items // 避免全删，回退
	}
	return out
}