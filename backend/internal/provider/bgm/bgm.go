package bgm

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/provider/base"
)

// BGM 是 Bangumi 元数据客户端。
// 提供搜索、番剧详情、剧集、评分、用户信息与 OAuth。
type BGM struct {
	*base.Fetcher
	cache *base.Cacher
}

// New 创建 BGM 客户端。
func New(cfg base.ConfigProvider, cache base.Cache) *BGM {
	f := base.New(cfg)
	b := &BGM{Fetcher: f, cache: base.NewCacher(cache)}
	// 从配置读取 token，设置 Authorization 头
	b.RefreshTokenFromConfig()
	return b
}

// RefreshTokenFromConfig 根据配置重建鉴权头（配置变更后调用）。
func (b *BGM) RefreshTokenFromConfig() {
	if cfg := b.Cfg.Get(); cfg != nil && cfg.BgmToken != "" {
		b.Header["Authorization"] = "Bearer " + cfg.BgmToken
	} else {
		delete(b.Header, "Authorization")
	}
	b.Header["Accept"] = "application/json"
}

var subjectIdRe = regexp.MustCompile(`/subject/(\d+)`)

// GetSubjectIdByURL 从 BGM URL 提取番剧 id。
func GetSubjectIdByURL(bgmURL string) string {
	if m := subjectIdRe.FindStringSubmatch(bgmURL); len(m) > 1 {
		return m[1]
	}
	return ""
}

// api 返回配置的 BGM API 基础地址。
func (b *BGM) api() string {
	if cfg := b.Cfg.Get(); cfg != nil && cfg.BgmApi != "" {
		return strings.TrimSuffix(cfg.BgmApi, "/")
	}
	return "https://api.bgm.tv"
}

// Search 按名称搜索番剧。
func (b *BGM) Search(ctx context.Context, name string) ([]*domain.BgmInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("搜索关键词为空")
	}
	name = strings.ReplaceAll(name, "1/2", "½")
	u := b.api() + "/search/subject/" + url.PathEscape(name) + "?type=2&max_results=25&responseGroup=small"
	var body struct {
		List []map[string]interface{} `json:"list"`
	}
	if err := b.Get(ctx, u, &body); err != nil {
		return nil, err
	}
	var out []*domain.BgmInfo
	for _, m := range body.List {
		out = append(out, parseSubject(m))
	}
	return out, nil
}

// GetInfo 获取番剧详情（缓存 10 分钟）。
func (b *BGM) GetInfo(ctx context.Context, subjectID string) (*domain.BgmInfo, error) {
	if subjectID == "" {
		return nil, errors.New("subjectId 为空")
	}
	key := "bgm:info:" + subjectID
	var cached domain.BgmInfo
	if b.cache.Get(key, &cached) {
		return &cached, nil
	}
	var m map[string]interface{}
	if err := b.Get(ctx, b.api()+"/v0/subjects/"+subjectID, &m); err != nil {
		return nil, err
	}
	info := parseSubject(m)
	b.cache.Put(key, info, 10*time.Minute)
	return info, nil
}

// GetEpisodes 获取剧集列表（缓存 5 分钟）。
func (b *BGM) GetEpisodes(ctx context.Context, subjectID string) ([]*domain.BgmEpisode, error) {
	key := "bgm:eps:" + subjectID
	var cached []*domain.BgmEpisode
	if b.cache.Get(key, &cached) {
		return cached, nil
	}
	var eps []*domain.BgmEpisode
	for _, typ := range []int{0, 1} {
		var body struct {
			Data []domain.BgmEpisode `json:"data"`
		}
		u := fmt.Sprintf("%s/v0/episodes?subject_id=%s&type=%d&limit=100&offset=0", b.api(), subjectID, typ)
		if err := b.Get(ctx, u, &body); err != nil {
			continue
		}
		for i := range body.Data {
			e := body.Data[i]
			eps = append(eps, &e)
		}
		if len(eps) > 0 {
			break
		}
	}
	if len(eps) > 0 {
		b.cache.Put(key, eps, 5*time.Minute)
	}
	return eps, nil
}

// GetEpisodeTitleMap 返回集数 -> (中文名, 日文名) 映射。
func (b *BGM) GetEpisodeTitleMap(ctx context.Context, subjectID string) (map[int]string, map[int]string) {
	cn := map[int]string{}
	jp := map[int]string{}
	eps, err := b.GetEpisodes(ctx, subjectID)
	if err != nil {
		return cn, jp
	}
	for _, e := range eps {
		i := int(e.Sort)
		if e.Sort == 0 {
			i = int(e.Ep)
		}
		cn[i] = e.NameCn
		jp[i] = e.Name
	}
	return cn, jp
}

// Me 返回当前登录用户。
func (b *BGM) Me(ctx context.Context) (*domain.BgmMe, error) {
	var m map[string]interface{}
	if err := b.Get(ctx, b.api()+"/v0/me", &m); err != nil {
		return nil, err
	}
	me := &domain.BgmMe{
		ID:          intVal(m["id"]),
		Sign:        strVal(m["sign"]),
		URL:         strVal(m["url"]),
		Username:    strVal(m["username"]),
		Nickname:    strVal(m["nickname"]),
		UserGroup:   intVal(m["user_group"]),
		Email:       strVal(m["email"]),
		TimeOffset:  intVal(m["time_offset"]),
		ExpiresDays: intVal(m["expires_days"]),
	}
	if avatar, ok := m["avatar"].(map[string]interface{}); ok {
		me.Avatar = domain.BgmAvatar{
			Large:  strVal(avatar["large"]),
			Medium: strVal(avatar["medium"]),
			Small:  strVal(avatar["small"]),
		}
	}
	return me, nil
}

// GetRate 获取当前用户对某番剧的评分。
func (b *BGM) GetRate(ctx context.Context, subjectID string) (int, error) {
	me, err := b.Me(ctx)
	if err != nil {
		return 0, err
	}
	var m map[string]interface{}
	u := fmt.Sprintf("%s/v0/users/%s/collections/%s", b.api(), url.PathEscape(me.Username), subjectID)
	if err := b.Get(ctx, u, &m); err != nil {
		return 0, err
	}
	return int(floatVal(m["rating"])), nil
}

// SetRate 设置评分。
func (b *BGM) SetRate(ctx context.Context, subjectID string, score float64) error {
	payload := map[string]interface{}{"score": score}
	return b.PostJSON(ctx, b.api()+"/v0/users/-/collections/"+subjectID, payload, nil)
}

// ExchangeCode 用 OAuth 授权码换取 token 并写回配置。
func (b *BGM) ExchangeCode(ctx context.Context, code string) (access, refresh string, err error) {
	cfg := b.Cfg.Get()
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", cfg.BgmAppID)
	form.Set("client_secret", cfg.BgmAppSecret)
	form.Set("code", code)
	form.Set("redirect_uri", cfg.BgmRedirectUri)
	var m map[string]interface{}
	if err := b.PostForm(ctx, "https://bgm.tv/oauth/access_token", form, &m); err != nil {
		return "", "", err
	}
	access = strVal(m["access_token"])
	refresh = strVal(m["refresh_token"])
	if access == "" {
		return "", "", errors.New("oauth 换取 token 失败")
	}
	return access, refresh, nil
}

// RefreshToken 用 refresh token 换取新的 access token。
func (b *BGM) RefreshToken(ctx context.Context) (access, refresh string, err error) {
	cfg := b.Cfg.Get()
	if cfg.BgmTokenType == "INPUT" {
		return cfg.BgmToken, cfg.BgmRefreshToken, nil
	}
	if cfg.BgmRefreshToken == "" {
		return "", "", errors.New("未配置 bgmRefreshToken")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", cfg.BgmAppID)
	form.Set("client_secret", cfg.BgmAppSecret)
	form.Set("refresh_token", cfg.BgmRefreshToken)
	form.Set("redirect_uri", cfg.BgmRedirectUri)
	var m map[string]interface{}
	if err := b.PostForm(ctx, "https://bgm.tv/oauth/access_token", form, &m); err != nil {
		return "", "", err
	}
	access = strVal(m["access_token"])
	refresh = strVal(m["refresh_token"])
	if access == "" {
		return "", "", errors.New("oauth 刷新 token 失败")
	}
	return access, refresh, nil
}

// parseSubject 解析 BGM subject JSON 为领域模型。
func parseSubject(m map[string]interface{}) *domain.BgmInfo {
	info := &domain.BgmInfo{
		ID:       strVal(m["id"]),
		URL:      "https://bgm.tv/subject/" + strVal(m["id"]),
		Name:     strVal(m["name"]),
		NameCn:   strVal(m["name_cn"]),
		Eps:      intVal(m["eps"]),
		Platform: strVal(m["platform"]),
	}
	if p := strVal(m["type"]); p != "" && info.Platform == "" {
		info.Platform = p
	}
	if dateStr, ok := m["date"].(string); ok && dateStr != "" {
		if t, err := time.ParseInLocation("2006-01-02", dateStr, domain.Loc()); err == nil {
			info.Date = domain.DateTime(t)
		}
	} else if dateStr, ok := m["air_date"].(string); ok && dateStr != "" {
		if t, err := time.ParseInLocation("2006-01-02", dateStr, domain.Loc()); err == nil {
			info.Date = domain.DateTime(t)
		}
	}
	if images, ok := m["images"].(map[string]interface{}); ok {
		info.Images = domain.BgmImages{
			Small:  strVal(images["small"]),
			Grid:   strVal(images["grid"]),
			Large:  strVal(images["large"]),
			Medium: strVal(images["medium"]),
			Common: strVal(images["common"]),
		}
	}
	if rating, ok := m["rating"].(map[string]interface{}); ok {
		info.Rating.Rank = intVal(rating["rank"])
		info.Rating.Score = floatVal(rating["score"])
		info.Rating.Total = intVal(rating["total"])
	}
	if tags, ok := m["tags"].([]interface{}); ok {
		for _, t := range tags {
			tm, ok := t.(map[string]interface{})
			if !ok {
				continue
			}
			info.Tags = append(info.Tags, domain.BgmTag{
				Name:      strVal(tm["name"]),
				Count:     intVal(tm["count"]),
				TotalCont: intVal(tm["totalCont"]),
			})
		}
	}
	return info
}

// GetSeasonByName 从标题推断季数（支持中文数字）。
func GetSeasonByName(name string) int {
	if name == "" {
		return 1
	}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`第 ?([一二三四五六七八九十百千]+) ?[季期]`),
		regexp.MustCompile(`[Ss]eason ?(\d+)`),
		regexp.MustCompile(`(\d+)(st|nd|rd|th) ?[Ss]eason`),
		regexp.MustCompile(`[Ss](\d+)$`),
	} {
		m := re.FindStringSubmatch(name)
		if len(m) < 2 {
			continue
		}
		s := m[1]
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
		if n := chineseNum(s); n > 0 {
			return n
		}
	}
	return 1
}

// GetSeason 从标签/标题推断季数。
func GetSeason(info *domain.BgmInfo) int {
	if info == nil {
		return 1
	}
	for _, tag := range info.Tags {
		if n := GetSeasonByName(tag.Name); n > 1 {
			return n
		}
	}
	if info.NameCn != "" {
		if n := GetSeasonByName(info.NameCn); n > 1 {
			return n
		}
	}
	if info.Name != "" {
		if n := GetSeasonByName(info.Name); n > 1 {
			return n
		}
	}
	return 1
}

// GetEps 获取总集数（优先剧集列表长度，回退 info.Eps）。
func (b *BGM) GetEps(ctx context.Context, info *domain.BgmInfo) int {
	if info == nil || info.Eps < 1 {
		return 0
	}
	if eps, err := b.GetEpisodes(ctx, info.ID); err == nil && len(eps) > 0 {
		return len(eps)
	}
	return info.Eps
}

// GetFinalName 返回显示标题（优先中文名，BgmJpName 时用日文名）。
func GetFinalName(info *domain.BgmInfo, jpName bool) string {
	if info == nil {
		return "无标题"
	}
	title := info.NameCn
	if title == "" || jpName {
		title = info.Name
	}
	if strings.TrimSpace(title) == "" {
		title = "无标题"
	}
	return strings.TrimSpace(title)
}

// ImageField 按 key 选择图片字段。
func ImageField(images *domain.BgmImages, key string) string {
	switch key {
	case "small":
		return images.Small
	case "medium":
		return images.Medium
	case "grid":
		return images.Grid
	case "common":
		return images.Common
	default:
		return images.Large
	}
}

// chineseNum 解析中文数字（支持 十/十五/二十三 等）。
func chineseNum(s string) int {
	if s == "十" {
		return 10
	}
	if strings.Contains(s, "十") {
		parts := strings.Split(s, "十")
		left := 1 // "十五" → 十前面无数字时默认 1
		if parts[0] != "" {
			left = chineseDigits(parts[0])
		}
		right := 0
		if len(parts) > 1 && parts[1] != "" {
			right = chineseDigits(parts[1])
		}
		if left > 0 {
			return left*10 + right
		}
		return right
	}
	return chineseDigits(s)
}

func chineseDigits(s string) int {
	digits := map[rune]int{'一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	n := 0
	for _, r := range s {
		if v, ok := digits[r]; ok {
			n = n*10 + v
		}
	}
	return n
}

func strVal(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	}
	return fmt.Sprintf("%v", v)
}

func intVal(v interface{}) int {
	return int(floatVal(v))
}

func floatVal(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	return 0
}