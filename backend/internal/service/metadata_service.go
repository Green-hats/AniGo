package service

import (
	"context"
	"errors"
	"strings"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/provider/bgm"
	"github.com/greenhats/anigo/internal/provider/garden"
	"github.com/greenhats/anigo/internal/provider/tmdb"
)

// MetadataService 聚合元数据 provider（BGM/TMDB/animes.garden），
// 提供订阅创建（RssToAni）、bgmId 解析、剧集标题等能力。
type MetadataService struct {
	cfg  *ConfigService
	bgm  *bgm.BGM
	tmdb *tmdb.TMDB
	gr   *garden.Garden
	cache domain.Cache
}

// NewMetadataService 创建元数据服务。
func NewMetadataService(cfg *ConfigService, cache domain.Cache) *MetadataService {
	return &MetadataService{
		cfg:   cfg,
		bgm:   bgm.New(cfg, cache),
		tmdb:  tmdb.New(cfg, cache),
		gr:    garden.New(cfg),
		cache: cache,
	}
}

// BGM 返回 BGM 客户端。
func (s *MetadataService) BGM() *bgm.BGM { return s.bgm }

// TMDB 返回 TMDB 客户端。
func (s *MetadataService) TMDB() *tmdb.TMDB { return s.tmdb }

// Garden 返回 animes.garden 客户端。
func (s *MetadataService) Garden() *garden.Garden { return s.gr }

// Reload 在配置变更后刷新 provider 鉴权。
func (s *MetadataService) Reload() {
	s.bgm.RefreshTokenFromConfig()
}

// BgmSubjectId 解析订阅的 BGM 番剧 id（bgmUrl 优先，否则按标题搜索）。
func (s *MetadataService) BgmSubjectId(ctx context.Context, ani *domain.Ani) string {
	if ani == nil {
		return ""
	}
	if id := bgm.GetSubjectIdByURL(ani.BgmUrl); id != "" {
		return id
	}
	key := "bgm:subjectid:" + ani.Title + ":" + itoaSeason(ani.Season)
	if v, ok := s.cache.Get(key); ok {
		return v
	}
	id := s.searchSubjectID(ctx, ani.Title, ani.Season)
	if id != "" {
		s.cache.Put(key, id, 10*minute)
	}
	return id
}

func (s *MetadataService) searchSubjectID(ctx context.Context, title string, season int) string {
	list, err := s.bgm.Search(ctx, title)
	if err != nil {
		return ""
	}
	for _, info := range list {
		if season > 0 && info.Season > 0 && info.Season != season {
			continue
		}
		if info.Name == title || info.NameCn == title {
			return info.ID
		}
	}
	if len(list) > 0 {
		return list[0].ID
	}
	return ""
}

// RssToAni 从 RSS URL 构建订阅（填充 BGM 信息）。
func (s *MetadataService) RssToAni(ctx context.Context, dto *domain.RssToAniDTO) (*domain.Ani, error) {
	urlStr := strings.TrimSpace(dto.URL)
	if urlStr == "" {
		return nil, errors.New("RSS地址 不能为空")
	}
	typ := dto.Type
	if typ == "" {
		typ = "garden"
	}
	cfg := s.cfg.Get()

	ani := domain.DefaultAni()
	ani.URL = urlStr
	ani.Type = typ

	// 尝试从 URL 解析 bgm 链接
	bgmUrl := dto.BgmUrl
	switch typ {
	case "garden":
		if subj := garden.GetSubjectIdFromURL(urlStr); subj != "" {
			bgmUrl = "https://bgm.tv/subject/" + subj
		}
	case "ani-bt", "anime-garden":
		if subj := garden.GetSubjectIdFromURL(urlStr); subj != "" {
			bgmUrl = "https://bgm.tv/subject/" + subj
		}
	default:
		// mikan 等：尝试从 BGM 搜索
	}
	if strings.TrimSpace(bgmUrl) == "" && strings.TrimSpace(ani.Title) == "" {
		return nil, errors.New("无法确定 BGM 番剧")
	}
	ani.BgmUrl = bgmUrl

	// 用 BGM 填充订阅
	subjectID := bgm.GetSubjectIdByURL(bgmUrl)
	if subjectID == "" {
		return nil, errors.New("无法解析 BGM 番剧ID")
	}
	info, err := s.bgm.GetInfo(ctx, subjectID)
	if err != nil {
		return nil, errors.New("获取 BGM 信息失败: " + err.Error())
	}
	s.ToAni(ctx, info, ani)

	// 默认启用
	ani.Enable = true
	ani.DownloadNew = cfg.DownloadNew
	ani.GlobalExclude = cfg.EnabledExclude
	if cfg.ImportExclude {
		merged := append([]string{}, cfg.Exclude...)
		merged = append(merged, ani.Exclude...)
		ani.Exclude = dedupStrs(merged)
	}

	// 下载路径模板
	ani.CustomDownloadPathTemplate = GetDownloadPath(cfg, ani, nil)
	return ani, nil
}

// ToAni 用 BGM 信息填充订阅（镜像 BgmUtil.toAni）。
func (s *MetadataService) ToAni(ctx context.Context, info *domain.BgmInfo, ani *domain.Ani) *domain.Ani {
	if info == nil || ani == nil {
		return ani
	}
	cfg := s.cfg.Get()
	image := ""
	if cfg.BgmImage != "" {
		image = bgm.ImageField(&info.Images, cfg.BgmImage)
	} else {
		image = info.Images.Large
	}
	score := 0.0
	if info.Rating.Score != 0 {
		score = info.Rating.Score
	}
	platform := strings.ToUpper(info.Platform)
	ova := platform == "OVA" || platform == "剧场版"

	ani.BgmUrl = "https://bgm.tv/subject/" + info.ID
	ani.Title = bgm.GetFinalName(info, cfg.BgmJpName)
	ani.JpTitle = info.Name
	ani.Season = bgm.GetSeason(info)
	ani.TotalEpisodeNumber = s.bgm.GetEps(ctx, info)
	// BGM 已播出集数 = BGM 实际收录的剧集数（番剧更新到哪 BGM 收录到哪）
	if eps, err := s.bgm.GetEpisodes(ctx, info.ID); err == nil && len(eps) > 0 {
		ani.BgmAiredEps = len(eps)
	}
	ani.Ova = ova
	ani.Score = score
	if !info.Date.Time().IsZero() {
		ani.ReleaseDate = domain.Date(info.Date.Time())
	}
	ani.Image = image
	if image != "" {
		ani.Cover = s.SaveCover(image)
	}
	return ani
}

// SaveCover 下载封面到本地 files/ 并返回相对路径。
func (s *MetadataService) SaveCover(imageURL string) string {
	if imageURL == "" {
		return ""
	}
	b, err := s.TMDBFetcherGet(imageURL)
	if err != nil {
		return ""
	}
	rel := coverRelPath(imageURL)
	full := s.cfg.ConfigDirFile("files/" + rel)
	if err := writeFileAtomic(full, b); err != nil {
		return ""
	}
	return rel
}

func itoaSeason(s int) string {
	return strconvInt(s)
}

func dedupStrs(list []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range list {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}