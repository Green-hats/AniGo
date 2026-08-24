package tmdb

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/provider/base"
	"github.com/greenhats/anigo/internal/rename"
)

// TMDB 是 TMDB 元数据客户端。
// 提供搜索（剧集/电影）、剧集标题、图片与剧组信息。
type TMDB struct {
	*base.Fetcher
	cache *base.Cacher
}

// 兜底 api key（老项目同款，可被配置覆盖）。
const fallbackKey = "450e4f651e1c93e31383e20f8e731e5f"

// New 创建 TMDB 客户端。
func New(cfg base.ConfigProvider, cache base.Cache) *TMDB {
	return &TMDB{Fetcher: base.New(cfg), cache: base.NewCacher(cache)}
}

func (t *TMDB) api() string {
	if cfg := t.Cfg.Get(); cfg != nil && cfg.TmdbApi != "" {
		return strings.TrimSuffix(cfg.TmdbApi, "/")
	}
	return "https://api.themoviedb.org"
}

func (t *TMDB) imageBase() string {
	if cfg := t.Cfg.Get(); cfg != nil && cfg.TmdbImage != "" {
		return strings.TrimSuffix(cfg.TmdbImage, "/")
	}
	return "https://image.tmdb.org"
}

func (t *TMDB) apiKey() string {
	if cfg := t.Cfg.Get(); cfg != nil && cfg.TmdbApiKey != "" {
		return cfg.TmdbApiKey
	}
	return fallbackKey
}

func (t *TMDB) language() string {
	if cfg := t.Cfg.Get(); cfg != nil && cfg.TmdbLanguage != "" {
		return cfg.TmdbLanguage
	}
	return "zh-CN"
}

// get 带标准参数请求 TMDB。
func (t *TMDB) get(ctx context.Context, path string, params url.Values, v interface{}) error {
	if params == nil {
		params = url.Values{}
	}
	params.Set("api_key", t.apiKey())
	params.Set("language", t.language())
	u := t.api() + path + "?" + params.Encode()
	return t.Get(ctx, u, v)
}

// SearchTV 按名称搜索剧集。
func (t *TMDB) SearchTV(ctx context.Context, name string) ([]*domain.Tmdb, error) {
	var body struct {
		Results []struct {
			ID           int    `json:"id"`
			Name         string `json:"name"`
			OriginalName string `json:"original_name"`
			FirstAirDate string `json:"first_air_date"`
			PosterPath   string `json:"poster_path"`
			BackdropPath string `json:"backdrop_path"`
			Overview     string `json:"overview"`
			VoteAverage  float64 `json:"vote_average"`
			VoteCount    int    `json:"vote_count"`
		} `json:"results"`
	}
	if err := t.get(ctx, "/3/search/tv", url.Values{"query": {name}}, &body); err != nil {
		return nil, err
	}
	var out []*domain.Tmdb
	for _, r := range body.Results {
		it := &domain.Tmdb{
			ID: r.ID, Name: r.Name, OriginalName: r.OriginalName,
			PosterPath: r.PosterPath, BackdropPath: r.BackdropPath,
			Overview: r.Overview, VoteAverage: r.VoteAverage, VoteCount: r.VoteCount,
			TmdbType: "tv",
		}
		if d, err := time.Parse("2006-01-02", r.FirstAirDate); err == nil {
			it.Date = domain.Date(d)
		}
		out = append(out, it)
	}
	return out, nil
}

// SearchMovie 按名称搜索电影。
func (t *TMDB) SearchMovie(ctx context.Context, name string) ([]*domain.Tmdb, error) {
	var body struct {
		Results []struct {
			ID           int    `json:"id"`
			Title        string `json:"title"`
			OriginalName string `json:"original_title"`
			ReleaseDate  string `json:"release_date"`
			PosterPath   string `json:"poster_path"`
			BackdropPath string `json:"backdrop_path"`
			Overview     string `json:"overview"`
			VoteAverage  float64 `json:"vote_average"`
			VoteCount    int    `json:"vote_count"`
		} `json:"results"`
	}
	if err := t.get(ctx, "/3/search/movie", url.Values{"query": {name}}, &body); err != nil {
		return nil, err
	}
	var out []*domain.Tmdb
	for _, r := range body.Results {
		it := &domain.Tmdb{
			ID: r.ID, Name: r.Title, OriginalName: r.OriginalName,
			PosterPath: r.PosterPath, BackdropPath: r.BackdropPath,
			Overview: r.Overview, VoteAverage: r.VoteAverage, VoteCount: r.VoteCount,
			TmdbType: "movie",
		}
		if d, err := time.Parse("2006-01-02", r.ReleaseDate); err == nil {
			it.Date = domain.Date(d)
		}
		out = append(out, it)
	}
	return out, nil
}

// GetByName 搜索 TMDB（OVA 优先电影，否则优先剧集；失败时互换再试）。
func (t *TMDB) GetByName(ctx context.Context, name string, ova bool) (*domain.Tmdb, error) {
	var results []*domain.Tmdb
	var err error
	if ova {
		results, err = t.SearchMovie(ctx, name)
	} else {
		results, err = t.SearchTV(ctx, name)
	}
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		if ova {
			results, err = t.SearchTV(ctx, name)
		} else {
			results, err = t.SearchMovie(ctx, name)
		}
		if err != nil || len(results) == 0 {
			return nil, errors.New("TMDB 无搜索结果")
		}
	}
	return results[0], nil
}

// GetFinalName 构建带年份/ID 后缀的显示名。
func (t *TMDB) GetFinalName(tm *domain.Tmdb) string {
	cfg := t.Cfg.Get()
	title := tm.Name
	if cfg.TmdbOriginalName && tm.OriginalName != "" {
		title = tm.OriginalName
	}
	if cfg.TitleYear {
		title = rename.RenameDelConfig(title, false)
		year := ""
		if !tm.Date.Time().IsZero() {
			year = strconv.Itoa(tm.Date.Time().Year())
		}
		title = fmt.Sprintf("%s (%s)", title, year)
	}
	if cfg.TmdbId {
		if cfg.TmdbIdPlexMode {
			title = fmt.Sprintf("%s {tmdb-%d}", title, tm.ID)
		} else {
			title = fmt.Sprintf("%s [tmdbid=%d]", title, tm.ID)
		}
	}
	return rename.GetName(title)
}

// ImageURL 构建完整图片 URL。
func (t *TMDB) ImageURL(path string) string {
	if path == "" {
		return ""
	}
	return t.imageBase() + "/t/p/original" + path
}

// GetEpisodeTitleMap 返回集数 -> 标题映射（缓存 5 分钟）。
func (t *TMDB) GetEpisodeTitleMap(ctx context.Context, ani *domain.Ani) map[int]string {
	out := map[int]string{}
	if ani == nil || ani.Ova || ani.Tmdb == nil || ani.Tmdb.ID == 0 {
		return out
	}
	season := ani.Season
	key := fmt.Sprintf("tmdb:eps:%d:%s:%d", ani.Tmdb.ID, ani.Tmdb.TmdbGroupId, season)
	var cached map[int]string
	if t.cache.Get(key, &cached) {
		return cached
	}
	if season <= 0 {
		season = 1
	}
	s, err := t.getSeason(ctx, ani.Tmdb, season)
	if err != nil {
		return out
	}
	for _, e := range s.Episodes {
		out[e.EpisodeNumber] = rename.GetName(e.Name)
	}
	if len(out) == 0 {
		t.cache.Put(key, out, 10*time.Second)
	} else {
		t.cache.Put(key, out, 5*time.Minute)
	}
	return out
}

// getSeason 获取季信息（支持 episode group）。
func (t *TMDB) getSeason(ctx context.Context, tm *domain.Tmdb, season int) (*seasonInfo, error) {
	if tm == nil || tm.ID == 0 {
		return nil, errors.New("no tmdb")
	}
	if tm.TmdbGroupId != "" {
		return t.getSeasonFromGroup(ctx, tm.TmdbGroupId, season)
	}
	var s seasonInfo
	if err := t.get(ctx, fmt.Sprintf("/3/tv/%d/season/%d", tm.ID, season), nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

type seasonInfo struct {
	SeasonNumber int             `json:"season_number"`
	Name         string          `json:"name"`
	Overview     string          `json:"overview"`
	PosterPath   string          `json:"poster_path"`
	VoteAverage  float64         `json:"vote_average"`
	AirDate      string          `json:"air_date"`
	Episodes     []tmdbEpisode   `json:"episodes"`
}

type tmdbEpisode struct {
	EpisodeNumber int     `json:"episode_number"`
	SeasonNumber  int     `json:"season_number"`
	Name          string  `json:"name"`
	Overview      string  `json:"overview"`
	VoteAverage   float64 `json:"vote_average"`
	AirDate       string  `json:"air_date"`
	Runtime       int     `json:"runtime"`
	StillPath     string  `json:"still_path"`
}

func (t *TMDB) getSeasonFromGroup(ctx context.Context, groupID string, season int) (*seasonInfo, error) {
	var body struct {
		Groups []struct {
			Order    int           `json:"order"`
			Episodes []tmdbEpisode `json:"episodes"`
		} `json:"groups"`
	}
	if err := t.get(ctx, "/3/tv/episode_group/"+groupID, nil, &body); err != nil {
		return nil, err
	}
	for _, g := range body.Groups {
		if g.Order == season {
			s := &seasonInfo{SeasonNumber: season}
			for i, e := range g.Episodes {
				e.EpisodeNumber = i + 1
				s.Episodes = append(s.Episodes, e)
			}
			if len(s.Episodes) > 0 {
				s.AirDate = s.Episodes[0].AirDate
			}
			return s, nil
		}
	}
	return nil, errors.New("season not found in group")
}

// GetTmdbGroup 列出某剧的剧组（episode group）列表。
func (t *TMDB) GetTmdbGroup(ctx context.Context, tm *domain.Tmdb) ([]*domain.Tmdb, error) {
	if tm == nil || tm.ID == 0 {
		return nil, errors.New("no tmdb")
	}
	var body struct {
		Results []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := t.get(ctx, fmt.Sprintf("/3/tv/%d/episode_groups", tm.ID), nil, &body); err != nil {
		return nil, err
	}
	var out []*domain.Tmdb
	for _, s := range body.Results {
		out = append(out, &domain.Tmdb{ID: 0, Name: s.Name, TmdbGroupId: s.ID})
	}
	return out, nil
}

// 图片排序：优先匹配配置语言。
func (t *TMDB) imageScore(im *tmdbImage) float64 {
	lang := t.language()
	if im.Iso6391 == "" {
		return 50 + im.VoteAverage
	}
	if im.Iso6391+"-"+im.Iso3166_1 == lang {
		return im.VoteAverage
	}
	if lang == "zh-CN" {
		return 10 + im.VoteAverage
	}
	if strings.HasPrefix(lang, "zh-") {
		return 20 + im.VoteAverage
	}
	if strings.HasPrefix(lang, "ja-") {
		return 30 + im.VoteAverage
	}
	return 40 + im.VoteAverage
}

type tmdbImage struct {
	FilePath    string  `json:"file_path"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	VoteAverage float64 `json:"vote_average"`
	Iso6391     string  `json:"iso_639_1"`
	Iso3166_1   string  `json:"iso_3166_1"`
}

// GetTmdbImages 获取并排序图片。
func (t *TMDB) GetTmdbImages(ctx context.Context, tm *domain.Tmdb) ([]string, error) {
	if tm == nil || tm.ID == 0 {
		return nil, errors.New("no tmdb")
	}
	var body struct {
		Backdrops []tmdbImage `json:"backdrops"`
		Posters   []tmdbImage `json:"posters"`
	}
	if err := t.get(ctx, fmt.Sprintf("/3/%s/%d/images", tm.TmdbType, tm.ID), nil, &body); err != nil {
		return nil, err
	}
	all := append(body.Backdrops, body.Posters...)
	sort.SliceStable(all, func(i, j int) bool { return t.imageScore(&all[i]) < t.imageScore(&all[j]) })
	var out []string
	for i, im := range all {
		if i >= 20 {
			break
		}
		if im.FilePath != "" {
			out = append(out, t.ImageURL(im.FilePath))
		}
	}
	return out, nil
}