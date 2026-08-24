package rename

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/greenhats/anigo/internal/domain"
)

// RegStr 是剧集提取正则（RenameUtil.REG_STR）。
var RegStr = `(.*|\[.*])(( - |Vol |[Ee][Pp]?)\d+(\.5)?( ?\(\d+\))?|【\d+(\.5)?】|\[\d+(\.5)?( ?\(\d+\))?( ?[vV]\d)?( ?END)?( ?完)?( ?FIN)?]|第\d+(\.5)?[话話集]( - END)?|^\[TOC].* \d+|^六四位元字幕组.*★\d+(\.5)?★)`

var (
	regEp       = regexp.MustCompile(RegStr)
	regNumber   = regexp.MustCompile(`\d+(\.5)?`)
	regYear     = regexp.MustCompile(` ?\(((19|20)\d{2})\)`)
	regTmdbId   = regexp.MustCompile(` ?(\[tmdbid=(\d+)]|\{tmdb-(\d+)})`)
	regRes      = regexp.MustCompile(`(720|1080|2160)[Pp]`)
	regHashTail = regexp.MustCompile(`\[([A-Z]|\d){8}]$`)
)

// Rename 提取剧集号并构造条目的 reName。
// 无法解析出剧集号时返回 false（条目应被丢弃）。
func Rename(ani *domain.Ani, item *domain.Item, cfg *domain.Config) bool {
	offset := ani.Offset
	season := ani.Season
	title := ani.Title

	if ani.Ova {
		item.ReName = RenameDel(title, cfg)
		return true
	}

	itemTitle := item.Title
	itemTitle = strings.ReplaceAll(itemTitle, "+NCOPED", "")
	itemTitle = strings.TrimSpace(itemTitle)
	itemTitle = strings.ReplaceAll(itemTitle, "\n", " ")
	itemTitle = strings.ReplaceAll(itemTitle, "\t", " ")
	itemTitle = regHashTail.ReplaceAllString(itemTitle, "")
	itemTitle = strings.TrimSpace(itemTitle)

	var e string
	if ani.CustomEpisode {
		re, err := regexp.Compile(ani.CustomEpisodeStr)
		if err == nil {
			groups := re.FindStringSubmatch(itemTitle)
			idx := ani.CustomEpisodeGroupIndex
			if idx < len(groups) {
				e = groups[idx]
			}
		}
	} else {
		groups := regEp.FindStringSubmatch(itemTitle)
		if len(groups) > 2 {
			e = groups[2]
		}
	}

	if strings.TrimSpace(e) == "" {
		return false
	}

	episodeStr := regNumber.FindString(e)
	if episodeStr == "" {
		return false
	}

	episodeF, err := strconv.ParseFloat(episodeStr, 64)
	if err != nil {
		return false
	}
	episode := episodeF + float64(offset)
	item.Episode = episode

	seasonFormat := fmt.Sprintf("%02d", season)
	episodeFormat := fmt.Sprintf("%02d", int(episode))
	episodeStrInt := strconv.Itoa(int(episode))

	is5 := isHalf(episode)

	if cfg.Skip5 && is5 {
		return false
	}

	if is5 {
		episodeFormat = episodeFormat + ".5"
		episodeStrInt = episodeStrInt + ".5"
	}

	itemTitle = GetName(itemTitle)
	resolution := GetResolution(itemTitle)
	tmdbId := ""
	if ani.Tmdb != nil && ani.Tmdb.ID != 0 {
		tmdbId = strconv.Itoa(ani.Tmdb.ID)
	}

	subgroup := item.Subgroup
	if strings.TrimSpace(subgroup) == "" {
		subgroup = "未知字幕组"
	}

	tmpl := GetRenameTemplate(ani, cfg)
	tmpl = strings.ReplaceAll(tmpl, "${seasonFormat}", seasonFormat)
	tmpl = strings.ReplaceAll(tmpl, "${episodeFormat}", episodeFormat)
	tmpl = strings.ReplaceAll(tmpl, "${season}", strconv.Itoa(season))
	tmpl = strings.ReplaceAll(tmpl, "${episode}", episodeStrInt)
	tmpl = strings.ReplaceAll(tmpl, "${subgroup}", subgroup)
	tmpl = strings.ReplaceAll(tmpl, "${itemTitle}", itemTitle)
	tmpl = strings.ReplaceAll(tmpl, "${resolution}", resolution)
	tmpl = strings.ReplaceAll(tmpl, "${tmdbid}", tmdbId)
	tmpl = strings.ReplaceAll(tmpl, "${title}", title)
	tmpl = strings.ReplaceAll(tmpl, "${themoviedbName}", ani.ThemoviedbName)
	tmpl = RenameDel(tmpl, cfg)

	reName := GetName(tmpl)
	if cfg.MaxFileNameLength > 0 && len([]rune(reName)) > cfg.MaxFileNameLength {
		reName = string([]rune(reName)[:cfg.MaxFileNameLength])
	}
	item.ReName = reName
	return true
}

// GetRenameTemplate 解析订阅生效的重命名模板。
func GetRenameTemplate(ani *domain.Ani, cfg *domain.Config) string {
	tmpl := cfg.RenameTemplate
	if ani.CustomRenameTemplateEnable {
		tmpl = ani.CustomRenameTemplate
	}
	if strings.TrimSpace(tmpl) == "" {
		tmpl = "${title} S${seasonFormat}E${episodeFormat}"
	}
	return tmpl
}

func isHalf(ep float64) bool {
	return int(ep) != int(ep+0.499999) && ep != float64(int(ep))
}

// Is5 判断剧集号是否为 x.5 特别篇。
func Is5(ep float64) bool {
	return ep != float64(int(ep))
}

// GetResolution 从标题提取视频分辨率。
func GetResolution(itemTitle string) string {
	repl := map[string]string{
		"1920x1080": "1080p",
		"3840x2160": "2160p",
		"1280x720":  "720p",
	}
	for k, v := range repl {
		itemTitle = strings.ReplaceAll(itemTitle, k, v)
	}
	if regRes.MatchString(itemTitle) {
		return strings.ToLower(regRes.FindString(itemTitle))
	}
	return "none"
}

// GetName 净化文件名（getName）。
func GetName(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "1/2", "½")
	repl := map[string]string{
		"/": " ",
		"\\": " ",
		":": "：",
		"?": "？",
		"|": "｜",
		"*": " ",
		"<": " ",
		">": " ",
		"\"": " ",
	}
	for k, v := range repl {
		s = strings.ReplaceAll(s, k, v)
	}
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// RenameDel 根据配置去除 tmdb id 与年份标记。
func RenameDel(title string, cfg *domain.Config) string {
	if strings.TrimSpace(title) == "" {
		return ""
	}
	if cfg.RenameDelTmdbId {
		title = regTmdbId.ReplaceAllString(title, "")
	}
	if cfg.RenameDelYear {
		title = regYear.ReplaceAllString(title, "")
	}
	return strings.TrimSpace(title)
}

// GetSubgroup 从首条括号条目提取字幕组。
func GetSubgroup(items []*domain.Item) string {
	reg := regexp.MustCompile(`^\[(.+?)]`)
	for _, item := range items {
		title := item.Title
		if m := reg.FindStringSubmatch(title); len(m) > 1 {
			return m[1]
		}
	}
	return "未知字幕组"
}