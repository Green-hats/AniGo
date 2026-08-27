// Package scoring 提供 RSS 条目的选版打分逻辑：
// 按集分组，从同集多版本中选出最优条目（分辨率 > 压制源 > 编码 > 色深 > 字幕嵌入/语言等）。
package scoring

import (
	"strings"

	"github.com/greenhats/anigo/internal/domain"
)

// resolutionScore 将分辨率转为优先级（数值越大越优先）。
func resolutionScore(res string) int {
	switch strings.ToLower(strings.TrimSpace(res)) {
	case "2160p", "4k":
		return 3
	case "1080p":
		return 2
	case "720p":
		return 1
	default:
		return 0
	}
}

// sourceScore 压制源质量分：BD > BDRip > WebRip > Web > 其他(0)。
func sourceScore(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "bd", "bdrip":
		return 4
	case "webrip":
		return 3
	case "web":
		return 2
	default:
		return 0
	}
}

// codecScore 视频编码分：HEVC/x265 > AVC/x264 > 其他(0)。
func codecScore(c string) int {
	switch strings.ToLower(strings.TrimSpace(c)) {
	case "hevc", "x265", "h265":
		return 2
	case "avc", "x264", "h264":
		return 1
	default:
		return 0
	}
}

// colorScore 色深分：10bit > 8bit > 其他(0)。
func colorScore(c string) int {
	switch strings.ToLower(strings.TrimSpace(c)) {
	case "10bit", "10bit+":
		return 2
	case "8bit":
		return 1
	default:
		return 0
	}
}

// embedScore 字幕嵌入方式分：内封 > 内嵌 > 外挂 > 其他(0)。
func embedScore(e string) int {
	switch strings.ToLower(strings.TrimSpace(e)) {
	case "内封", "内封字幕", "软字幕":
		return 3
	case "内嵌", "硬字幕":
		return 2
	case "外挂", "外挂字幕":
		return 1
	default:
		return 0
	}
}

// langScore 字幕语言丰富度分：按字幕语言标记长度/种类粗略计分。
// 简繁日 > 简日 > 简 > 繁 > 其他(0)。缺失视为 0。
func langScore(l string) int {
	s := strings.ToLower(strings.TrimSpace(l))
	if s == "" {
		return 0
	}
	if strings.Contains(s, "简") && strings.Contains(s, "繁") {
		return 4 // 简繁（双语最全）
	}
	if strings.Contains(s, "简") {
		return 3 // 含简中
	}
	if strings.Contains(s, "繁") {
		return 2 // 繁中
	}
	return 1 // 有语言标记但非简繁
}

// PickBestPerEpisode 按集分组，每集只保留一个最优条目。
// 比较链（从高到低）：分辨率 > 压制源 > 视频编码 > 色深 > 字幕嵌入方式
// > 字幕语言丰富度 > 是否匹配订阅字幕组 > 是否主源。
// 缺失的信号视为该档最低分（不影响后续比较）。
func PickBestPerEpisode(items []*domain.Item, subscribedSubgroup string) []*domain.Item {
	best := map[int]*domain.Item{}
	// 保持稳定：遇到更优的才替换
	better := func(cur, cand *domain.Item) bool {
		if cs, ds := resolutionScore(cur.Resolution), resolutionScore(cand.Resolution); ds > cs {
			return true
		} else if ds < cs {
			return false
		}
		if ds := sourceScore(cand.Source); ds > sourceScore(cur.Source) {
			return true
		} else if ds < sourceScore(cur.Source) {
			return false
		}
		if dc := codecScore(cand.VideoCodec); dc > codecScore(cur.VideoCodec) {
			return true
		} else if dc < codecScore(cur.VideoCodec) {
			return false
		}
		if db := colorScore(cand.ColorDepth); db > colorScore(cur.ColorDepth) {
			return true
		} else if db < colorScore(cur.ColorDepth) {
			return false
		}
		if de := embedScore(cand.SubtitleEmbed); de > embedScore(cur.SubtitleEmbed) {
			return true
		} else if de < embedScore(cur.SubtitleEmbed) {
			return false
		}
		if dl := langScore(cand.SubtitleLang); dl > langScore(cur.SubtitleLang) {
			return true
		} else if dl < langScore(cur.SubtitleLang) {
			return false
		}
		curSub := cur.Subgroup != "" && cur.Subgroup == subscribedSubgroup
		candSub := cand.Subgroup != "" && cand.Subgroup == subscribedSubgroup
		if candSub != curSub {
			return candSub
		}
		return cand.Master && !cur.Master
	}
	for _, it := range items {
		ep := int(it.Episode)
		cur, ok := best[ep]
		if !ok || better(cur, it) {
			best[ep] = it
		}
	}
	var out []*domain.Item
	for _, it := range best {
		out = append(out, it)
	}
	return out
}

// HardFilterTitle 用固定硬规则判断标题是否应在进 AI 前粗筛剔除。
// 只处理"明显不需要下载"的类型（多集合成/合集、明显 720p），
// 与订阅的 match/exclude 规则无关（那些已交给 AI 判断）。
func HardFilterTitle(title string) bool {
	t := strings.ToLower(strings.TrimSpace(title))
	if strings.Contains(t, "合集") || strings.Contains(t, "全集") || strings.Contains(t, "全卷") {
		return true
	}
	if strings.Contains(t, "720p") {
		return true
	}
	return false
}
