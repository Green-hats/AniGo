package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/util"
)

// AniService 负责订阅的增删改、列表分组与预览。
type AniService struct {
	cfg  *ConfigService
	rss  *RssService
	meta *MetadataService
	// onAdded 在订阅添加成功后触发（由 main 注入 DownloadService.DownloadAni）。
	onAdded func(ani *domain.Ani)
}

// NewAniService 创建订阅服务。
func NewAniService(cfg *ConfigService, rss *RssService, meta *MetadataService) *AniService {
	return &AniService{cfg: cfg, rss: rss, meta: meta}
}

// SetOnAdded 注册订阅添加后的回调（异步触发一轮下载）。
func (s *AniService) SetOnAdded(fn func(ani *domain.Ani)) { s.onAdded = fn }

// pathResolve 返回下载路径的 bgmId/jpTitle 解析回调。
func (s *AniService) pathResolve(ctx context.Context) func(ani *domain.Ani) (string, string) {
	if s.meta == nil {
		return nil
	}
	return func(ani *domain.Ani) (string, string) {
		return s.meta.BgmSubjectId(ctx, ani), ani.JpTitle
	}
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
		visibleTasks := []domain.DownloadTask{}
		for _, task := range a.DownloadTasks {
			if taskBelongs(task, cfg) {
				visibleTasks = append(visibleTasks, task)
			}
		}
		a.DownloadTasks = visibleTasks
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
	} else {
		ani = ani.Clone()
	}
	fillAniDefaultsFromAni(ani)
	replace := s.cfg.Get().Replace
	err := s.cfg.UpdateAniList(func(list *[]*domain.Ani) error {
		for _, a := range *list {
			if a == nil {
				continue
			}
			if a.ID == ani.ID {
				return fmt.Errorf("订阅已存在")
			}
			if a.Title == ani.Title && a.Season == ani.Season {
				if !replace {
					return fmt.Errorf("已存在同名订阅")
				}
				ani.ID = a.ID
				ani.Downloaded, ani.DownloadedHash, ani.DownloadTasks = a.Downloaded, a.DownloadedHash, a.DownloadTasks
				ani.DownloadedEps = a.DownloadedEps
				*a = *ani
				return nil
			}
		}
		*list = append(*list, ani)
		return nil
	})
	if err != nil {
		return err
	}
	if s.onAdded != nil && ani.Enable {
		s.onAdded(ani.Clone())
	}
	return nil
}

// SetAniRaw 更新订阅（部分合并，保留服务器管理字段）。
func (s *AniService) SetAniRaw(raw []byte) error {
	srcMap := map[string]interface{}{}
	if err := json.Unmarshal(raw, &srcMap); err != nil {
		return err
	}
	return s.cfg.UpdateAniList(func(listPtr *[]*domain.Ani) error {
		list := *listPtr
		id := strval(srcMap["id"])
		for _, a := range list {
			if a == nil || a.ID != id {
				continue
			}
			merged := a.Clone()
			if err := MergeAniMap(merged, srcMap); err != nil {
				return err
			}
			for _, other := range list {
				if other != nil && other.ID != id && other.Title == merged.Title && other.Season == merged.Season {
					return fmt.Errorf("订阅标题重复")
				}
			}
			*a = *merged
			return nil
		}
		return ErrAniNotFound
	})
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
	// 新订阅默认启用；Omit/Procrastinating/Message 默认开启
	// （与 DefaultAni 一致，避免前端漏传时订阅被禁用）
	a.Enable = true
	a.Omit = true
	a.Procrastinating = true
	a.Message = true
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
	merged.Downloaded = dst.Downloaded
	merged.DownloadedHash = dst.DownloadedHash
	merged.DownloadedEps = dst.DownloadedEps
	merged.DownloadTasks = dst.DownloadTasks
	*dst = *merged
	return nil
}

// DeleteAni 删除订阅。
func (s *AniService) DeleteAni(ids []string) error {
	return s.cfg.UpdateAniList(func(list *[]*domain.Ani) error {
		remaining := make([]*domain.Ani, 0, len(*list))
		for _, a := range *list {
			if a != nil && !containsStr(ids, a.ID) {
				remaining = append(remaining, a)
			}
		}
		*list = remaining
		return nil
	})
}

func (s *AniService) BatchEnable(ids []string, value bool) error {
	return s.cfg.UpdateAniList(func(list *[]*domain.Ani) error {
		for _, a := range *list {
			if a != nil && containsStr(ids, a.ID) {
				a.Enable = value
			}
		}
		return nil
	})
}

// PreviewAni 返回下载路径与条目（供 UI 预览）。
func (s *AniService) PreviewAni(ctx context.Context, ani *domain.Ani) (map[string]interface{}, error) {
	items, err := s.rss.GetItems(ctx, ani)
	if err != nil {
		return nil, err
	}
	savePath := GetDownloadPath(s.cfg.Get(), ani, s.pathResolve(ctx))
	omitItems := []int{}
	preview := []*domain.Item{}
	for _, it := range items {
		it.HasDownloaded = false
		preview = append(preview, it)
	}
	return map[string]interface{}{
		"downloadPath": savePath,
		"items":        preview,
		"omitList":     omitItems,
	}, nil
}

// DownloadPathPreview 返回订阅解析出的下载路径。
func (s *AniService) DownloadPathPreview(ctx context.Context, ani *domain.Ani) map[string]interface{} {
	return map[string]interface{}{
		"downloadPath": GetDownloadPath(s.cfg.Get(), ani, s.pathResolve(ctx)),
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
