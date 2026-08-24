package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/log"
	"github.com/greenhats/anigo/internal/provider/ai"
	"github.com/greenhats/anigo/internal/rename"
	"github.com/greenhats/anigo/internal/rss"
)

// RssService 负责 RSS 聚合、过滤与去重。
// 当 AI 启用并配置了 apiKey 时，用 AI 解析标题集数，失败回退正则。
type RssService struct {
	cfg    *ConfigService
	ai     *ai.DeepSeek
	logger *log.Logger
}

// NewRssService 创建 RSS 服务。
func NewRssService(cfg *ConfigService, logger *log.Logger) *RssService {
	return &RssService{
		cfg:    cfg,
		ai:     ai.New(cfg.Get()),
		logger: logger,
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
// 比较链（从高到低）：分辨率 > 压制源 > 视频编码 > 色深 > 字幕嵌入方式
// > 字幕语言丰富度 > 是否匹配订阅字幕组 > 是否主源。
// 缺失的信号视为该档最低分（不影响后续比较）。
func pickBestPerEpisode(items []*domain.Item, subscribedSubgroup string) []*domain.Item {
	best := map[int]*domain.Item{}
	// 保持稳定：遇到更优的才替换
	better := func(cur, cand *domain.Item) bool {
		if cs, ds := resolutionScore(cur.Resolution), resolutionScore(cand.Resolution); ds > cs {
			return true
		} else if ds < cs {
			return false
		}
		if ds := sourceScore(cand.Source); ds > sourceScore(cur.Source) {
			return true
		} else if ds < sourceScore(cur.Source) {
			return false
		}
		if dc := codecScore(cand.VideoCodec); dc > codecScore(cur.VideoCodec) {
			return true
		} else if dc < codecScore(cur.VideoCodec) {
			return false
		}
		if db := colorScore(cand.ColorDepth); db > colorScore(cur.ColorDepth) {
			return true
		} else if db < colorScore(cur.ColorDepth) {
			return false
		}
		if de := embedScore(cand.SubtitleEmbed); de > embedScore(cur.SubtitleEmbed) {
			return true
		} else if de < embedScore(cur.SubtitleEmbed) {
			return false
		}
		if dl := langScore(cand.SubtitleLang); dl > langScore(cur.SubtitleLang) {
			return true
		} else if dl < langScore(cur.SubtitleLang) {
			return false
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

// sourceScore 压制源质量分：BD > BDRip > WebRip > Web > 其他(0)。
func sourceScore(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "bd", "bdrip":
		return 4
	case "webrip":
		return 3
	case "web":
		return 2
	default:
		return 0
	}
}

// codecScore 视频编码分：HEVC/x265 > AVC/x264 > 其他(0)。
func codecScore(c string) int {
	switch strings.ToLower(strings.TrimSpace(c)) {
	case "hevc", "x265", "h265":
		return 2
	case "avc", "x264", "h264":
		return 1
	default:
		return 0
	}
}

// colorScore 色深分：10bit > 8bit > 其他(0)。
func colorScore(c string) int {
	switch strings.ToLower(strings.TrimSpace(c)) {
	case "10bit", "10bit+":
		return 2
	case "8bit":
		return 1
	default:
		return 0
	}
}

// embedScore 字幕嵌入方式分：内封 > 内嵌 > 外挂 > 其他(0)。
func embedScore(e string) int {
	switch strings.ToLower(strings.TrimSpace(e)) {
	case "内封", "内封字幕", "软字幕":
		return 3
	case "内嵌", "硬字幕":
		return 2
	case "外挂", "外挂字幕":
		return 1
	default:
		return 0
	}
}

// langScore 字幕语言丰富度分：按字幕语言标记长度/种类粗略计分。
// 简繁日 > 简日 > 简 > 繁 > 其他(0)。缺失视为 0。
func langScore(l string) int {
	s := strings.ToLower(strings.TrimSpace(l))
	if s == "" {
		return 0
	}
	if strings.Contains(s, "简") && strings.Contains(s, "繁") {
		return 4 // 简繁（双语最全）
	}
	if strings.Contains(s, "简") {
		return 3 // 含简中
	}
	if strings.Contains(s, "繁") {
		return 2 // 繁中
	}
	return 1 // 有语言标记但非简繁
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
	items := rss.Parse(cfg, ani, rssURL, subgroupName, xmlBody)
	s.logf("INFO", "rss", "%s rss解析到 %d 个原始条目", ani.Title, len(items))
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
	parsed, err := s.ai.Parse(ctx, titles)
	if err != nil {
		// AI 失败 → 无集号来源，丢弃
		s.logf("WARN", "rss", "%s ai解析失败: %v", ani.Title, err)
		return nil
	}
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
	refined = s.filterByAI(cfg, ani, refined)
	refined = rss.DistinctByEpisode(refined)
	s.logf("INFO", "rss", "%s rss结束刷新, 共 %d 个条目", ani.Title, len(refined))
	return refined
}

// filterByAI 用 AI 筛选器剔除不符合订阅规则的条目（尽力而为，失败不阻断）。
func (s *RssService) filterByAI(cfg *domain.Config, ani *domain.Ani, items []*domain.Item) []*domain.Item {
	// 无匹配/排除规则且未开启简中字幕筛选时无需 AI 筛选
	if len(ani.Match) == 0 && len(ani.Exclude) == 0 && !cfg.AiSubtitleSC {
		return items
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	titles := make([]string, len(items))
	for i, it := range items {
		titles[i] = it.Title
	}
	keep, err := s.ai.Filter(ctx, ani, titles)
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

// logf 写入 RSS 日志（logger 未注入时静默跳过）。
func (s *RssService) logf(level, logger, format string, args ...interface{}) {
	if s.logger == nil {
		return
	}
	s.logger.Log(level, logger, fmt.Sprintf(format, args...))
}