package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/util"
)

// GetDownloadPath 解析订阅的下载路径模板。
func GetDownloadPath(cfg *domain.Config, ani *domain.Ani) string {
	tmpl := cfg.DownloadPathTemplate
	if ani.Ova && cfg.OvaDownloadPathTemplate != "" {
		tmpl = cfg.OvaDownloadPathTemplate
	}
	if ani.CustomDownloadPath && strings.TrimSpace(ani.CustomDownloadPathTemplate) != "" {
		for _, line := range strings.Split(ani.CustomDownloadPathTemplate, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				tmpl = line
				break
			}
		}
	}

	title := strings.TrimSpace(ani.Title)
	letter := util.GetPinyinInitials(title)
	tmpl = strings.ReplaceAll(tmpl, "${letter}", letter)

	releaseDate := ani.ReleaseDate.Time()
	if releaseDate.IsZero() {
		releaseDate = time.Now()
	}
	tmdbDate := releaseDate
	if ani.Tmdb != nil && !ani.Tmdb.Date.Time().IsZero() {
		tmdbDate = ani.Tmdb.Date.Time()
	}
	tmdbYear := tmdbDate.Year()
	year := releaseDate.Year()
	month := int(releaseDate.Month())
	monthFormat := fmt.Sprintf("%02d", month)

	if strings.Contains(tmpl, "${quarter}") || strings.Contains(tmpl, "${quarterFormat}") || strings.Contains(tmpl, "${quarterName}") {
		var quarter int
		var quarterName string
		switch month {
		case 12, 1, 2:
			if month == 12 {
				year++
			}
			quarter = 1
			quarterName = "冬"
		case 3, 4, 5:
			quarter = 4
			quarterName = "春"
		case 6, 7, 8:
			quarter = 7
			quarterName = "夏"
		default:
			quarter = 10
			quarterName = "秋"
		}
		tmpl = strings.ReplaceAll(tmpl, "${quarter}", strconv.Itoa(quarter))
		tmpl = strings.ReplaceAll(tmpl, "${quarterFormat}", fmt.Sprintf("%02d", quarter))
		tmpl = strings.ReplaceAll(tmpl, "${quarterName}", quarterName)
	}

	tmpl = strings.ReplaceAll(tmpl, "${tmdbYear}", strconv.Itoa(tmdbYear))
	tmpl = strings.ReplaceAll(tmpl, "${year}", strconv.Itoa(year))
	tmpl = strings.ReplaceAll(tmpl, "${month}", strconv.Itoa(month))
	tmpl = strings.ReplaceAll(tmpl, "${monthFormat}", monthFormat)

	season := ani.Season
	tmpl = strings.ReplaceAll(tmpl, "${season}", strconv.Itoa(season))
	tmpl = strings.ReplaceAll(tmpl, "${seasonFormat}", fmt.Sprintf("%02d", season))

	tmpl = strings.ReplaceAll(tmpl, "${title}", ani.Title)
	tmpl = strings.ReplaceAll(tmpl, "${themoviedbName}", ani.ThemoviedbName)
	tmpl = strings.ReplaceAll(tmpl, "${subgroup}", ani.Subgroup)

	tmdbId := ""
	if ani.Tmdb != nil && ani.Tmdb.ID != 0 {
		tmdbId = strconv.Itoa(ani.Tmdb.ID)
	}
	tmpl = strings.ReplaceAll(tmpl, "${tmdbid}", tmdbId)
	tmpl = strings.ReplaceAll(tmpl, "${jpTitle}", ani.JpTitle)

	return cleanPath(tmpl)
}

// cleanPath 规范化斜杠处理。
func cleanPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimSuffix(p, "/")
	return p
}