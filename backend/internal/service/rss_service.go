package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
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
// AI 解析标题并缓存结果；失败显式返回错误。
type RssService struct {
	cfg    *ConfigService
	logger *log.Logger

	parseGate   chan struct{}
	cacheMu     sync.Mutex
	parsedCache map[string]titleCacheEntry
	failMu      sync.Mutex
	failCount   map[string]int       // 每个源连续 AI 失败次数
	failTime    map[string]time.Time // 每个源最近一次失败时间
}

// ai 连续失败退避参数：连续失败达到阈值后，在退避期内不再发起 AI 请求。
const (
	aiFailBackoffThreshold = 3
	aiFailBackoffDuration  = 5 * time.Minute
)

// NewRssService 创建 RSS 服务。
func NewRssService(cfg *ConfigService, logger *log.Logger) *RssService {
	return &RssService{
		cfg:         cfg,
		parseGate:   make(chan struct{}, 1),
		parsedCache: map[string]titleCacheEntry{},
		logger:      logger,
		failCount:   map[string]int{},
		failTime:    map[string]time.Time{},
	}
}

// aiKey 生成某源的失败状态 key。
func aiKey(ani *domain.Ani, rssURL string) string {
	return ani.ID + "|" + rssURL
}

// ReloadAI 在 AI 配置变更后重建客户端。
func (s *RssService) ReloadAI() {
	s.cacheMu.Lock()
	s.parsedCache = map[string]titleCacheEntry{}
	s.cacheMu.Unlock()
	s.failMu.Lock()
	s.failCount, s.failTime = map[string]int{}, map[string]time.Time{}
	s.failMu.Unlock()
}

func (s *RssService) AIPing(ctx context.Context) (string, error) {
	client := ai.New(s.cfg.Get())
	if client == nil {
		return "", errAINotConfigured
	}
	return client.Ping(ctx)
}

// errAINotConfigured 是 AI 未配置错误。
var errAINotConfigured = &aiNotConfiguredError{}

type aiNotConfiguredError struct{}

func (e *aiNotConfiguredError) Error() string { return "AI 未配置 apiKey" }

// GetItems 聚合主 + 备用 RSS 条目，按剧集排序。
// 每集只保留一个最优版本：分辨率优先（2160p>1080p>720p），
// 再优先订阅的字幕组，再优先主源（Master）。
func (s *RssService) GetItems(ctx context.Context, ani *domain.Ani) ([]*domain.Item, error) {
	cfg := s.cfg.Get()
	subgroup := ani.Subgroup
	if strings.TrimSpace(subgroup) == "" {
		subgroup = "未知字幕组"
	}
	var items []*domain.Item
	var sourceErrors []error
	mainItems, err := s.getItems(ctx, ani, ani.URL, subgroup)
	if err != nil {
		sourceErrors = append(sourceErrors, err)
	}
	for _, it := range mainItems {
		it.Master = true
		items = append(items, it)
	}

	if cfg.StandbyRss {
		for _, sr := range ani.StandbyRssList {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Second):
			}
			subgroup = sr.Label
			if strings.TrimSpace(subgroup) == "" {
				subgroup = "未知字幕组"
			}
			clone := ani.Clone()
			clone.Offset = sr.Offset
			standby, err := s.getItems(ctx, clone, sr.URL, subgroup)
			if err != nil {
				sourceErrors = append(sourceErrors, err)
				continue
			}
			for _, it := range standby {
				it.Master = false
				items = append(items, it)
			}
		}
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if len(items) == 0 && len(sourceErrors) > 0 {
		return nil, errors.Join(sourceErrors...)
	}
	// 每集选一个最优版本
	items = scoring.PickBestPerEpisode(items, ani.Subgroup)
	sort.SliceStable(items, func(i, j int) bool { return items[i].Episode < items[j].Episode })
	return items, nil
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
// AI 未启用/不可用/失败时返回错误，不将失败当作空订阅。
func (s *RssService) getItems(ctx context.Context, ani *domain.Ani, rssURL, subgroupName string) ([]*domain.Item, error) {
	cfg := s.cfg.Get()
	s.logf("INFO", "rss", "%s rss开始刷新 (%s)", ani.Title, rssURL)
	xmlBody, err := rss.GetRSS(ctx, cfg, rssURL)
	if err != nil {
		s.logf("WARN", "rss", "%s rss获取失败: %v", ani.Title, err)
		return nil, err
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
	if len(items) == 0 {
		return nil, nil
	}
	if !cfg.AiEnabled || ai.New(cfg) == nil {
		return nil, errAINotConfigured
	}
	// 批量 AI 解析：提取每个条目的原始标题，让 AI 重算集号
	s.logf("INFO", "rss", "%s ai开始解析 (%d 条)", ani.Title, len(items))
	titles := make([]string, len(items))
	for i, it := range items {
		titles[i] = it.Title
	}
	parentCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	key := aiKey(ani, rssURL) + fmt.Sprintf("%x", sha256.Sum256([]byte(cfg.AiBaseURL+"\x00"+cfg.AiApiKey+"\x00"+cfg.AiModel)))
	if s.aiInBackoff(key) {
		s.logf("WARN", "rss", "%s ai连续失败, 暂缓解析, 跳过本轮", ani.Title)
		return nil, fmt.Errorf("AI 连续失败，暂缓解析")
	}
	parsed, err := s.parseCached(ctx, cfg, ani, titles)
	if err != nil {
		// AI 失败 → 无集号来源，丢弃；记录失败次数以触发退避
		if parentCtx.Err() == nil {
			s.aiRecordFail(key)
		}
		s.logf("WARN", "rss", "%s ai解析失败: %v", ani.Title, err)
		return nil, err
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

	s.logf("INFO", "rss", "%s rss结束刷新, 共 %d 个条目", ani.Title, len(refined))
	return refined, nil
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

type titleCacheEntry struct {
	value   domain.ParsedTitle
	expires time.Time
}

// parseCached 仅调用新增标题；规则、提示词、模型或凭据变化自动使用新缓存键。
func (s *RssService) parseCached(ctx context.Context, cfg *domain.Config, ani *domain.Ani, titles []string) ([]domain.ParsedTitle, error) {
	select {
	case s.parseGate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-s.parseGate }()
	rules := ani.Clone()
	if cfg.EnabledExclude && ani.GlobalExclude {
		rules.Exclude = append(rules.Exclude, cfg.Exclude...)
	}
	signature, _ := json.Marshal([]any{cfg.AiProvider, cfg.AiBaseURL, cfg.AiModel, cfg.AiApiKey, cfg.AiSubtitleSC, rules.Match, rules.Exclude, domain.DEFAULT_AI_PROMPT()})
	keyFor := func(title string) string {
		return fmt.Sprintf("%x", sha256.Sum256(append(append([]byte(nil), signature...), []byte(title)...)))
	}
	results := make([]domain.ParsedTitle, len(titles))
	missing := []string{}
	positions := map[string][]int{}
	now := time.Now()
	s.cacheMu.Lock()
	for key, entry := range s.parsedCache {
		if !now.Before(entry.expires) {
			delete(s.parsedCache, key)
		}
	}
	for i, title := range titles {
		if entry, ok := s.parsedCache[keyFor(title)]; ok {
			results[i] = entry.value
			continue
		}
		if len(positions[title]) == 0 {
			missing = append(missing, title)
		}
		positions[title] = append(positions[title], i)
	}
	s.cacheMu.Unlock()
	client := ai.New(cfg)
	for start := 0; start < len(missing); start += 32 {
		end := min(start+32, len(missing))
		batch := missing[start:end]
		parsed, err := client.Parse(ctx, rules, batch)
		if err != nil {
			return nil, err
		}
		s.cacheMu.Lock()
		for i, title := range batch {
			if len(s.parsedCache) >= 10000 {
				for k := range s.parsedCache {
					delete(s.parsedCache, k)
					break
				}
			}
			s.parsedCache[keyFor(title)] = titleCacheEntry{parsed[i], time.Now().Add(24 * time.Hour)}
			for _, pos := range positions[title] {
				results[pos] = parsed[i]
			}
		}
		s.cacheMu.Unlock()
	}
	return results, nil
}
