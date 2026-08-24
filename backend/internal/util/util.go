package util

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mozillazg/go-pinyin"

	"github.com/greenhats/anigo/internal/domain"
)

var videoExts = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".wmv": true, ".mov": true,
	".ts": true, ".flv": true, ".rmvb": true, ".rm": true, ".webm": true,
}

var subtitleExts = map[string]bool{
	".ass": true, ".ssa": true, ".sub": true, ".srt": true, ".lyc": true,
	".sup": true, ".pgs": true, ".mks": true,
}

var videoMimes = map[string]string{
	".mp4": "video/mp4", ".m4v": "video/x-m4v", ".mkv": "video/x-matroska",
	".avi": "video/x-msvideo", ".wmv": "video/x-ms-wmv", ".mov": "video/quicktime",
	".ts": "video/mp2t", ".flv": "video/x-flv", ".rmvb": "video/vnd.rn-realvideo",
	".rm": "video/vnd.rn-realvideo", ".webm": "video/webm",
}

// VideoMimeType 返回视频路径的媒体类型，未知时返回空串。
func VideoMimeType(name string) string {
	return videoMimes[strings.ToLower(filepath.Ext(name))]
}

// IsVideo 判断扩展名是否为视频文件。
func IsVideo(name string) bool { return videoExts[strings.ToLower(filepath.Ext(name))] }

// IsSubtitle 判断扩展名是否为字幕文件。
func IsSubtitle(name string) bool { return subtitleExts[strings.ToLower(filepath.Ext(name))] }

// FormatSize 将字节数转换为人类可读字符串，如 "1.23 MiB"
// （1024 进制，单位 B/KiB/MiB/GiB/TiB，镜像 FileUtils.formatSize）。
func FormatSize(size int64) string {
	if size <= 0 {
		return "0 B"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	i := 0
	f := float64(size)
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.2f %s", f, units[i])
}

var (
	pinyinArgs = pinyin.Args{
		Style:     pinyin.Normal,
		Heteronym: false,
		Separator: "",
		Fallback:  func(r rune, a pinyin.Args) []string { return []string{string(r)} },
	}
)

// GetPinyin 返回中文字符串的拼音。
func GetPinyin(s string) string {
	var sb strings.Builder
	for _, r := range s {
		res := pinyin.Pinyin(string(r), pinyinArgs)
		if len(res) > 0 && len(res[0]) > 0 {
			sb.WriteString(res[0][0])
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// GetPinyinInitials 返回拼音首字母（每个字拼音的首字母）。
func GetPinyinInitials(s string) string {
	var sb strings.Builder
	for _, r := range s {
		res := pinyin.Pinyin(string(r), pinyinArgs)
		if len(res) > 0 && len(res[0]) > 0 {
			sb.WriteString(res[0][0][:1])
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

var weekRegexes = []*regexp.Regexp{
	regexp.MustCompile(`(星期|周)日`),
	regexp.MustCompile(`(星期|周)一`),
	regexp.MustCompile(`(星期|周)二`),
	regexp.MustCompile(`(星期|周)三`),
	regexp.MustCompile(`(星期|周)四`),
	regexp.MustCompile(`(星期|周)五`),
	regexp.MustCompile(`(星期|周)六`),
}

// weekPatternIndex 将星期标签映射到模式索引（0=周日）。
func weekPatternIndex(label string) int {
	for i, re := range weekRegexes {
		if re.MatchString(label) {
			return i
		}
	}
	return -1
}

// WeekSortIndex 返回星期标签的排序位置，
// 从今天所在的星期开始排列并循环（镜像 WeekComparator）。
func WeekSortIndex(label string) int {
	idx := weekPatternIndex(label)
	if idx < 0 {
		return 1 << 30
	}
	today := int(domain.Now().Weekday()) // 0=周日
	var order []int
	for i := today; i >= 0; i-- {
		order = append(order, i)
	}
	for i := 6; i > 0; i-- {
		if !containsInt(order, i) {
			order = append(order, i)
		}
	}
	for i, v := range order {
		if v == idx {
			return i
		}
	}
	return 1 << 30
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}