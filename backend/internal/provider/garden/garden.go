package garden

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/provider/base"
)

// Garden 是 animes.garden 番剧源客户端。
// 提供番剧周列表、字幕组资源与 RSS 订阅链接。
type Garden struct {
	*base.Fetcher
}

const apiHost = "https://api.animes.garden"

// New 创建 animes.garden 客户端。
func New(cfg base.ConfigProvider) *Garden {
	return &Garden{Fetcher: base.New(cfg)}
}

// ListSubjects 获取番剧周列表。
// bgmUrl 非空时返回单个"搜索"分组（用于指定番剧直达）。
func (g *Garden) ListSubjects(ctx context.Context) ([]*domain.AnimeGarden, error) {
	var body struct {
		Subjects []struct {
			ID         domain.StrID `json:"id"`
			Name       string       `json:"name"`
			Keywords   []string     `json:"keywords"`
			ActivedAt  domain.DateTime `json:"activedAt"`
			IsArchived bool         `json:"isArchived"`
		} `json:"subjects"`
	}
	if err := g.Get(ctx, apiHost+"/subjects", &body); err != nil {
		return nil, err
	}
	weeks := []string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}
	byWeek := map[string][]domain.AnimeGardenSubject{}
	for _, s := range body.Subjects {
		week := weeks[int(s.ActivedAt.Time().Weekday())]
		byWeek[week] = append(byWeek[week], domain.AnimeGardenSubject{
			ID: s.ID, Name: s.Name, Keywords: s.Keywords,
			ActivedAt: s.ActivedAt, IsArchived: s.IsArchived, WeekLabel: week,
		})
	}
	var out []*domain.AnimeGarden
	for _, w := range weeks {
		subs, ok := byWeek[w]
		if !ok {
			continue
		}
		out = append(out, &domain.AnimeGarden{WeekLabel: w, Subjects: subs})
	}
	return out, nil
}

// Group 获取某番剧（bgmId）的字幕组资源分组。
func (g *Garden) Group(ctx context.Context, bgmID string) ([]*domain.AnimeGardenGroup, error) {
	params := url.Values{}
	params.Set("subject", bgmID)
	params.Set("pageSize", "200")
	params.Set("duplicate", "false")
	var body struct {
		Resources []domain.AnimeGardenItem `json:"resources"`
	}
	if err := g.Get(ctx, apiHost+"/resources?"+params.Encode(), &body); err != nil {
		return nil, err
	}

	// 按字幕组分桶
	groupByFansub := map[string][]domain.AnimeGardenItem{}
	var order []string
	for _, it := range body.Resources {
		if it.Fansub == nil {
			continue
		}
		id := string(it.Fansub.ID)
		if _, ok := groupByFansub[id]; !ok {
			order = append(order, id)
		}
		groupByFansub[id] = append(groupByFansub[id], it)
	}

	var groups []*domain.AnimeGardenGroup
	for _, id := range order {
		items := groupByFansub[id]
		if len(items) == 0 {
			continue
		}
		fansub := items[0].Fansub
		name := url.QueryEscape(fansub.Name)
		g := &domain.AnimeGardenGroup{
			ID:    id,
			Name:  fansub.Name,
			Rss:   fmt.Sprintf("%s/feed.xml?subject=%s&fansub=%s", apiHost, bgmID, name),
			BgmId: bgmID,
		}
		if len(items) > 0 {
			g.LastUpdatedAt = items[0].CreatedAt
		}
		g.Items = items
		groups = append(groups, g)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		return groups[i].LastUpdatedAt.Time().After(groups[j].LastUpdatedAt.Time())
	})
	return groups, nil
}

// GetSubjectIdFromURL 从 animes.garden 链接提取番剧 id。
func GetSubjectIdFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if v := u.Query().Get("subject"); v != "" {
		return v
	}
	// /subject/{id}
	path := strings.Trim(u.Path, "/")
	if strings.HasPrefix(path, "subject/") {
		return strings.TrimPrefix(path, "subject/")
	}
	return ""
}