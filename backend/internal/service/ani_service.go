package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/util"
)

// AniService 负责订阅的增删改、列表分组与预览。
type AniService struct {
	cfg *ConfigService
	rss *RssService
}

// NewAniService 创建订阅服务。
func NewAniService(cfg *ConfigService, rss *RssService) *AniService {
	return &AniService{cfg: cfg, rss: rss}
}

// ListAni 返回分组的订阅列表。
func (s *AniService) ListAni() *domain.ListAni {
	cfg := s.cfg.Get()
	list := s.cfg.AniList()
	sortBy := cfg.SortType

	sorted := append([]*domain.Ani(nil), list...)
	for _, a := range sorted {
		if a == nil {
			continue
		}
		a.Pinyin = util.GetPinyin(a.Title)
		a.PinyinInitials = util.GetPinyinInitials(a.Title)
	}
	switch sortBy {
	case "PINYIN":
		sort.SliceStable(sorted, func(i, j int) bool {
			if sorted[i] == nil || sorted[j] == nil {
				return false
			}
			return sorted[i].Pinyin < sorted[j].Pinyin
		})
	case "DOWNLOAD_TIME":
		sort.SliceStable(sorted, func(i, j int) bool {
			if sorted[i] == nil || sorted[j] == nil {
				return false
			}
			return sorted[i].LastDownloadTime > sorted[j].LastDownloadTime
		})
	default: // SCORE
		sort.SliceStable(sorted, func(i, j int) bool {
			if sorted[i] == nil || sorted[j] == nil {
				return false
			}
			return sorted[i].Score > sorted[j].Score
		})
	}

	weeks := []string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}
	weekItems := map[string][]*domain.Ani{}
	for _, w := range weeks {
		weekItems[w] = []*domain.Ani{}
	}

	releaseDateList := []string{}
	seenMonth := map[string]bool{}
	for i, ani := range sorted {
		if ani == nil {
			continue
		}
		ani.Sort = i
		if !ani.ReleaseDate.Time().IsZero() {
			month := ani.ReleaseDate.Time().Format("2006-01")
			if !seenMonth[month] {
				seenMonth[month] = true
				releaseDateList = append(releaseDateList, month)
			}
			wd := int(ani.ReleaseDate.Time().Weekday())
			weekItems[weeks[wd]] = append(weekItems[weeks[wd]], ani)
		}
	}
	sort.SliceStable(releaseDateList, func(i, j int) bool { return releaseDateList[i] > releaseDateList[j] })

	// 周顺序：当前星期优先，再循环
	today := int(domain.Now().Weekday())
	order := make([]string, 0, 7)
	for i := today; i >= 0; i-- {
		order = append(order, weeks[i])
	}
	for i := 6; i > 0; i-- {
		if !containsStr(order, weeks[i]) {
			order = append(order, weeks[i])
		}
	}
	var weekList []domain.WeekAni
	for _, w := range order {
		weekList = append(weekList, domain.WeekAni{WeekLabel: w, Items: weekItems[w]})
	}
	return &domain.ListAni{
		ReleaseDateList: releaseDateList,
		WeekList:        weekList,
		Total:           len(sorted),
	}
}

// AddAni 添加订阅。
func (s *AniService) AddAni(ani *domain.Ani) error {
	if ani == nil {
		ani = domain.DefaultAni()
	}
	// 保留用户提交的字段，仅补充缺失的默认值
	fillAniDefaultsFromAni(ani)
	list := s.cfg.AniList()
	for _, a := range list {
		if a != nil && a.ID == ani.ID {
			return fmt.Errorf("订阅已存在")
		}
	}
	// 重复标题+季
	for _, a := range list {
		if a != nil && a.Title == ani.Title && a.Season == ani.Season {
			if s.cfg.Get().Replace {
				origID := a.ID
				*a = *ani
				a.ID = origID
				return s.cfg.SaveAniList(list)
			}
			return fmt.Errorf("已存在同名订阅")
		}
	}
	list = append(list, ani)
	return s.cfg.SaveAniList(list)
}

// SetAniRaw 更新订阅（部分合并，保留服务器管理字段）。
func (s *AniService) SetAniRaw(raw []byte) error {
	srcMap := map[string]interface{}{}
	if err := json.Unmarshal(raw, &srcMap); err != nil {
		return err
	}
	list := s.cfg.AniList()
	for i, a := range list {
		if a == nil {
			continue
		}
		var id string
		if v, ok := srcMap["id"].(string); ok {
			id = v
		}
		if a.ID != id {
			continue
		}
		title := strval(srcMap["title"])
		season := intval(srcMap["season"])
		// 重复标题+季检查（排除自身）
		for j, other := range list {
			if other == nil || j == i {
				continue
			}
			if other.Title == title && other.Season == season {
				return fmt.Errorf("订阅标题重复")
			}
		}
		if err := MergeAniMap(a, srcMap); err != nil {
			return err
		}
		return s.cfg.SaveAniList(list)
	}
	return fmt.Errorf("订阅不存在")
}

// fillAniDefaultsFromAni 为订阅补充缺失的默认字段，保留用户已提交的值。
func fillAniDefaultsFromAni(a *domain.Ani) {
	def := domain.DefaultAni()
	if strings.TrimSpace(a.ID) == "" {
		a.ID = def.ID
	}
	if a.ReleaseDate.Time().IsZero() {
		a.ReleaseDate = def.ReleaseDate
	}
	if len(a.StandbyRssList) == 0 {
		a.StandbyRssList = []domain.StandbyRss{}
	}
	if len(a.Match) == 0 {
		a.Match = []string{}
	}
	if len(a.Exclude) == 0 {
		a.Exclude = append([]string(nil), def.Exclude...)
	}
	if len(a.NotDownload) == 0 {
		a.NotDownload = []float64{}
	}
	if len(a.CustomPriorityKeywords) == 0 {
		a.CustomPriorityKeywords = []string{}
	}
	if len(a.CustomTags) == 0 {
		a.CustomTags = []string{}
	}
	if a.CustomEpisodeGroupIndex == 0 {
		a.CustomEpisodeGroupIndex = 2
	}
	if a.CustomRenameTemplate == "" {
		a.CustomRenameTemplate = "[${subgroup}] ${title} S${seasonFormat}E${episodeFormat}"
	}
	if a.CustomEpisodeStr == "" {
		a.CustomEpisodeStr = domain.RENAME_REG_STR()
	}
}

// MergeAniMap 将 srcMap 中 JSON 存在的字段复制到 dst，保留当前集数与下载时间。
func MergeAniMap(dst *domain.Ani, srcMap map[string]interface{}) error {
	dstBytes, _ := json.Marshal(dst)
	dstMap := map[string]interface{}{}
	_ = json.Unmarshal(dstBytes, &dstMap)
	for k, v := range srcMap {
		if k == "currentEpisodeNumber" || k == "lastDownloadTime" {
			continue
		}
		dstMap[k] = v
	}
	mergedBytes, err := json.Marshal(dstMap)
	if err != nil {
		return err
	}
	merged := &domain.Ani{}
	if err := json.Unmarshal(mergedBytes, merged); err != nil {
		return err
	}
	merged.CurrentEpisodeNumber = dst.CurrentEpisodeNumber
	merged.LastDownloadTime = dst.LastDownloadTime
	*dst = *merged
	return nil
}

// DeleteAni 删除订阅。
func (s *AniService) DeleteAni(ids []string) {
	list := s.cfg.AniList()
	var remaining []*domain.Ani
	for _, a := range list {
		if a == nil {
			continue
		}
		if containsStr(ids, a.ID) {
			continue
		}
		remaining = append(remaining, a)
	}
	_ = s.cfg.SaveAniList(remaining)
}

// BatchEnable 批量启用/停用订阅。
func (s *AniService) BatchEnable(ids []string, value bool) {
	list := s.cfg.AniList()
	for _, a := range list {
		if a == nil || !containsStr(ids, a.ID) {
			continue
		}
		a.Enable = value
	}
	_ = s.cfg.SaveAniList(list)
}

// PreviewAni 返回下载路径与条目（供 UI 预览）。
func (s *AniService) PreviewAni(ani *domain.Ani) map[string]interface{} {
	items := s.rss.GetItems(ani)
	savePath := GetDownloadPath(s.cfg.Get(), ani)
	preview := []*domain.Item{}
	for _, it := range items {
		preview = append(preview, it)
	}
	return map[string]interface{}{
		"downloadPath": savePath,
		"items":        preview,
		"omitList":     []int{},
	}
}

// DownloadPathPreview 返回订阅解析出的下载路径。
func (s *AniService) DownloadPathPreview(ani *domain.Ani) map[string]interface{} {
	return map[string]interface{}{
		"downloadPath": GetDownloadPath(s.cfg.Get(), ani),
	}
}

// FindAniByID 按 ID 查找订阅。
func (s *AniService) FindAniByID(id string) *domain.Ani {
	for _, a := range s.cfg.AniList() {
		if a != nil && a.ID == id {
			return a
		}
	}
	return nil
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func strval(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intval(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	}
	return 0
}