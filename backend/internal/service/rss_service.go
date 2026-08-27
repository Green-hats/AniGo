package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/log"
	"github.com/greenhats/anigo/internal/provider/ai"
	"github.com/greenhats/anigo/internal/rename"
	"github.com/greenhats/anigo/internal/rss"
	"github.com/greenhats/anigo/internal/scoring"
)

// RssService 负责 RSS 聚合、过滤与去重。
// 当 AI 启用并配置了 apiKey 时，用 AI 解析标题集数，失败回退正则。
type RssService struct {
	cfg    *ConfigService
	ai     *ai.DeepSeek
	logger *log.Logger

	failMu    sync.Mutex
	failCount map[string]int       // 每个源连续 AI 失败次数
	failTime  map[string]time.Time // 每个源最近一次失败时间
}

// ai 连续失败退避参数：连续失败达到阈值后，在退避期内不再发起 AI 请求。
const (
	aiFailBackoffThreshold = 3
	aiFailBackoffDuration  = 5 * time.Minute
)

// NewRssService 创建 RSS 服务。
func NewRssService(cfg *ConfigService, logger *log.Logger) *RssService {
	return &RssService{
		cfg:       cfg,
		ai:        ai.New(cfg.Get()),
		logger:    logger,
		failCount: map[string]int{},
		failTime:  map[string]time.Time{},
	}
}

// aiKey 生成某源的失败状态 key。
func aiKey(ani *domain.Ani, rssURL string) string {
	return ani.ID + "|" + rssURL
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
	for _, it := range s.getItems(ani, ani.URL, subgroup) {
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
			for _, it := range s.getItems(clone, sr.URL, subgroup) {
				it.Master = false
				items = append(items, it)
			}
		}
		items = rss.DistinctItems(items, cfg.Coexist)
	}

	// 每集选一个最优版本
	items = scoring.PickBestPerEpisode(items, ani.Subgroup)
	sort.SliceStable(items, func(i, j int) bool { return items[i].Episode < items[j].Episode })
	return items
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
// 仅 AI 解析：先过滤出候选条目，再用 AI 批量重算集号并渲染 reName。
// AI 未启用/不可用/失败时无法确定集号，返回空（不再回退正则）。
func (s *RssService) getItems(ani *domain.Ani, rssURL, subgroupName string) []*domain.Item {
	cfg := s.cfg.Get()
	s.logf("INFO", "rss", "%s rss开始刷新 (%s)", ani.Title, rssURL)
	xmlBody, err := rss.GetRSS(cfg, rssURL)
	if err != nil {
		s.logf("WARN", "rss", "%s rss获取失败: %v", ani.Title, err)
		return nil
	}
	items := rss.Parse(ani, rssURL, subgroupName, xmlBody)
	s.logf("INFO", "rss", "%s rss解析到 %d 个原始条目", ani.Title, len(items))
	// 进 AI 前用固定硬规则粗筛明显不需要的条目，减少 AI 处理量
	pre := items[:0]
	for _, it := range items {
		if !scoring.HardFilterTitle(it.Title) {
			pre = append(pre, it)
		}
	}
	dropped := len(items) - len(pre)
	items = pre
	if dropped > 0 {
		s.logf("INFO", "rss", "%s 粗筛剔除 %d 条", ani.Title, dropped)
	}
	if len(items) == 0 || s.ai == nil || !cfg.AiEnabled {
		// 仅 AI 解析：无 AI 则无法确定集号
		s.logf("WARN", "rss", "%s ai未启用, 无集号来源, 丢弃", ani.Title)
		return nil
	}
	// 批量 AI 解析：提取每个条目的原始标题，让 AI 重算集号
	s.logf("INFO", "rss", "%s ai开始解析 (%d 条)", ani.Title, len(items))
	titles := make([]string, len(items))
	for i, it := range items {
		titles[i] = it.Title
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	key := aiKey(ani, rssURL)
	if s.aiInBackoff(key) {
		s.logf("WARN", "rss", "%s ai连续失败, 暂缓解析, 跳过本轮", ani.Title)
		return nil
	}
	parsed, err := s.ai.Parse(ctx, ani, titles)
	if err != nil {
		// AI 失败 → 无集号来源，丢弃；记录失败次数以触发退避
		s.aiRecordFail(key)
		s.logf("WARN", "rss", "%s ai解析失败: %v", ani.Title, err)
		return nil
	}
	s.aiResetFail(key)
	s.logf("INFO", "rss", "%s ai解析完成", ani.Title)
	var refined []*domain.Item
	for i, it := range items {
		pt := parsed[i]
		if pt.Episode <= 0 {
			// AI 无法判断该条 → 丢弃
			continue
		}
		clone := it.Clone()
		clone.Title = pt.Title
		clone.Subgroup = pt.Subgroup
		clone.Resolution = pt.Resolution
		clone.SubtitleEmbed = pt.SubtitleEmbed
		clone.VideoCodec = pt.VideoCodec
		clone.Source = pt.Source
		clone.ColorDepth = pt.ColorDepth
		clone.SubtitleLang = pt.SubtitleLang
		if rename.RenameWithEpisode(ani, clone, cfg, pt.Episode) {
			refined = append(refined, clone)
		}
	}
	if len(refined) == 0 {
		return nil
	}
	refined = rss.DistinctByEpisode(refined)
	s.logf("INFO", "rss", "%s rss结束刷新, 共 %d 个条目", ani.Title, len(refined))
	return refined
}

// logf 写入 RSS 日志（logger 未注入时静默跳过）。
func (s *RssService) logf(level, logger, format string, args ...interface{}) {
	if s.logger == nil {
		return
	}
	s.logger.Log(level, logger, fmt.Sprintf(format, args...))
}

// aiInBackoff 判断某源是否处于 AI 失败退避期。
func (s *RssService) aiInBackoff(key string) bool {
	s.failMu.Lock()
	defer s.failMu.Unlock()
	if s.failCount[key] < aiFailBackoffThreshold {
		return false
	}
	last, ok := s.failTime[key]
	if !ok {
		return false
	}
	return time.Since(last) < aiFailBackoffDuration
}

// aiRecordFail 记录一次 AI 失败（连续失败计数 + 时间）。
func (s *RssService) aiRecordFail(key string) {
	s.failMu.Lock()
	defer s.failMu.Unlock()
	s.failCount[key]++
	s.failTime[key] = time.Now()
}

// aiResetFail 清空某源的失败计数（AI 解析成功时调用）。
func (s *RssService) aiResetFail(key string) {
	s.failMu.Lock()
	defer s.failMu.Unlock()
	delete(s.failCount, key)
	delete(s.failTime, key)
}
