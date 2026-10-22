package garden

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/provider/base"
	"github.com/greenhats/anigo/internal/util"
)

// Garden 是 animes.garden 番剧源客户端。
// 提供番剧周列表、字幕组资源与 RSS 订阅链接。
type Garden struct {
	*base.Fetcher
}

const apiHost = "https://api.animes.garden"
const calendarHost = "https://bgm.animes.garden"

// New 创建 animes.garden 客户端。
func New(cfg base.ConfigProvider) *Garden {
	return &Garden{Fetcher: base.New(cfg)}
}

// ListSubjects 获取当前季度日历，与 animes.garden/calendar/YYYY-MM 一致。
// /subjects 是跨季度的资源索引，不能作为当季新番名单。
func (g *Garden) ListSubjects(ctx context.Context) ([]*domain.AnimeGarden, error) {
	return g.listSubjects(ctx, domain.Now())
}

type calendarSubject struct {
	ID        domain.StrID    `json:"id"`
	Title     string          `json:"title"`
	Poster    string          `json:"poster"`
	OnairDate domain.DateTime `json:"onair_date"`
	Alias     struct {
		Zh []string `json:"zh"`
	} `json:"alias"`
	Search struct {
		Include []string `json:"include"`
	} `json:"search"`
	Bangumi struct {
		Date     domain.DateTime `json:"date"`
		Tags     []string        `json:"tags"`
		MetaTags []string        `json:"meta_tags"`
		Images   struct {
			Large string `json:"large"`
		} `json:"images"`
	} `json:"bangumi"`
}

func (g *Garden) listSubjects(ctx context.Context, now time.Time) ([]*domain.AnimeGarden, error) {
	now = now.In(domain.Loc())
	month := (int(now.Month())-1)/3*3 + 1
	season := fmt.Sprintf("%04d-%02d", now.Year(), month)
	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			Seasons  []string            `json:"seasons"`
			Calendar [][]calendarSubject `json:"calendar"`
			Web      []calendarSubject   `json:"web"`
		} `json:"data"`
	}
	if err := g.Get(ctx, calendarHost+"/calendar?season="+season, &body); err != nil {
		return nil, err
	}
	if !body.OK || len(body.Data.Seasons) != 1 || body.Data.Seasons[0] != season || len(body.Data.Calendar) != 7 {
		return nil, fmt.Errorf("获取 %s 季度日历失败：源站返回了无效的日历数据", season)
	}
	// 源站按星期一到星期日分组；WEB 保留独立分组，不按首播日期重新推算星期。
	weeks := []string{"星期一", "星期二", "星期三", "星期四", "星期五", "星期六", "星期日", "WEB"}
	buckets := append(body.Data.Calendar, body.Data.Web)
	out := make([]*domain.AnimeGarden, 0, len(weeks))
	for i, bucket := range buckets {
		subs := make([]domain.AnimeGardenSubject, 0, len(bucket))
		for _, s := range bucket {
			// 与源站日历使用相同的可见性规则：有封面，且不属于国创标签。
			if s.Poster == "" && s.Bangumi.Images.Large == "" {
				continue
			}
			china := false
			for _, tags := range [][]string{s.Bangumi.MetaTags, s.Bangumi.Tags} {
				for _, tag := range tags {
					switch tag {
					case "国创", "国产", "国产动画", "国漫", "中国":
						china = true
					}
				}
			}
			if china {
				continue
			}
			name := s.Title
			if len(s.Alias.Zh) > 0 && s.Alias.Zh[0] != "" {
				name = s.Alias.Zh[0]
			}
			date := s.OnairDate
			if date.Time().IsZero() {
				date = s.Bangumi.Date
			}
			subs = append(subs, domain.AnimeGardenSubject{
				ID: s.ID, Name: name, Keywords: s.Search.Include,
				ActivedAt: date, WeekLabel: weeks[i],
				Cover: fmt.Sprintf("%s/bangumi/subject/%s/poster.jpeg?quality=large", calendarHost, s.ID),
			})
		}
		if len(subs) == 0 {
			continue
		}
		sort.SliceStable(subs, func(i, j int) bool {
			return subs[i].ActivedAt.Time().After(subs[j].ActivedAt.Time())
		})
		out = append(out, &domain.AnimeGarden{WeekLabel: weeks[i], Season: season, Subjects: subs})
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

	// API 只返回字节数 size，这里补上人类可读的 formatSize
	for i := range body.Resources {
		if body.Resources[i].FormatSize == "" && body.Resources[i].Size > 0 {
			body.Resources[i].FormatSize = util.FormatSize(body.Resources[i].Size)
		}
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
