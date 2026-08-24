package rename

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/greenhats/anigo/internal/domain"
)

var (
	regYear = regexp.MustCompile(` ?\(((19|20)\d{2})\)`)
	regRes  = regexp.MustCompile(`(720|1080|2160)[Pp]`)
)

// RenameWithEpisode 用给定的集号渲染重命名模板（跳过正则提取）。
// episode 由外部（如 AI 解析）提供。无法解析时返回 false。
func RenameWithEpisode(ani *domain.Ani, item *domain.Item, cfg *domain.Config, episode float64) bool {
	if ani.Ova {
		item.ReName = RenameDel(titleOf(ani), cfg)
		return true
	}
	if episode <= 0 {
		return false
	}
	season := ani.Season
	title := ani.Title
	offset := ani.Offset
	episode = episode + float64(offset)
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

	itemTitle := GetName(item.Title)
	resolution := GetResolution(itemTitle)
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
	tmpl = strings.ReplaceAll(tmpl, "${title}", title)
	tmpl = RenameDel(tmpl, cfg)

	reName := GetName(tmpl)
	if cfg.MaxFileNameLength > 0 && len([]rune(reName)) > cfg.MaxFileNameLength {
		reName = string([]rune(reName)[:cfg.MaxFileNameLength])
	}
	item.ReName = reName
	return true
}

func titleOf(ani *domain.Ani) string {
	if ani == nil {
		return ""
	}
	return ani.Title
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
	// 判断是否为 x.5 特别篇，容忍解析时的浮点误差（如 3.5000001）。
	frac := ep - math.Floor(ep)
	return math.Abs(frac-0.5) < 1e-6
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

// RenameDel 根据配置去除年份标记。
func RenameDel(title string, cfg *domain.Config) string {
	if strings.TrimSpace(title) == "" {
		return ""
	}
	if cfg.RenameDelYear {
		title = regYear.ReplaceAllString(title, "")
	}
	return strings.TrimSpace(title)
}