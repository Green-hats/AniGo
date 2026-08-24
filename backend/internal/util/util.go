package util

import (
	"fmt"
	"strings"

	"github.com/mozillazg/go-pinyin"
)

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