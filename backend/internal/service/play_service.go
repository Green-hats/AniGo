package service

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

// playCacheEntry 是播放列表的短缓存条目。
type playCacheEntry struct {
	items  []domain.PlayItem
	expire time.Time
}

// PlayList 返回订阅在 115 云端目录下所有可播放文件（含提取的集号与 pickcode）。
// 递归遍历子目录：115 离线会把每个文件包成一个同名文件夹（fc=0 视为目录），
// 真正可播放的文件在文件夹内部（fc=1）。
// 不在此处取 CDN URL：URL 会过期且绑定 UA，改为播放时经本地代理转发获取。
// 结果做 30 秒短缓存，避免短时间内重复打开播放弹窗时反复遍历 115。
func (s *DownloadService) PlayList(ctx context.Context, ani *domain.Ani) ([]domain.PlayItem, error) {
	s.playMu.Lock()
	if e, ok := s.playCache[ani.ID]; ok && time.Now().Before(e.expire) {
		items := e.items
		s.playMu.Unlock()
		return items, nil
	}
	s.playMu.Unlock()

	cfg := s.cfg.Get()
	if !s.Login(true) {
		return nil, fmt.Errorf("下载客户端登录失败: %s", s.Driver().GetLoginStatus().Message)
	}
	savePath := GetDownloadPath(cfg, ani, s.pathResolve())
	var items []domain.PlayItem
	var walk func(path string, depth int) error
	walk = func(path string, depth int) error {
		if depth > 6 {
			return nil
		}
		files, err := s.Driver().ListDir(ctx, cfg, path)
		if err != nil {
			return nil
		}
		for _, f := range files {
			// 115 的 /files 接口：fc=0 → 目录（含同名文件包文件夹），fc=1 → 文件。
			// 目录一律继续下钻；文件仅视频扩展名作为播放项。
			if f.IsDir {
				if err := walk(path+"/"+f.Name, depth+1); err != nil {
					return err
				}
				continue
			}
			if !isVideoFile(f.Name) {
				continue
			}
			items = append(items, domain.PlayItem{
				Episode:  extractEpisode(f.Name),
				Filename: f.Name,
				PickCode: f.PickCode,
			})
		}
		return nil
	}
	if err := walk(savePath, 0); err != nil {
		return nil, err
	}

	s.playMu.Lock()
	s.playCache[ani.ID] = &playCacheEntry{items: items, expire: time.Now().Add(30 * time.Second)}
	s.playMu.Unlock()
	return items, nil
}

// PlayStreamURL 通过 pickcode 取 115 CDN 直链（播放代理转发用）。
func (s *DownloadService) PlayStreamURL(ctx context.Context, pickcode string) (string, error) {
	cfg := s.cfg.Get()
	return s.Driver().FileURLByPickCode(ctx, cfg, pickcode)
}

// 常见视频文件扩展名（115 转存后的种子原始文件名通常是这些）。
var videoExts = map[string]bool{
	"mkv": true, "mp4": true, "avi": true, "ts": true, "flv": true,
	"webm": true, "wmv": true, "mov": true, "m4v": true, "m2ts": true, "rmvb": true,
}

// isVideoFile 判断文件名是否为可播放的视频文件（按扩展名）。
func isVideoFile(name string) bool {
	idx := strings.LastIndex(name, ".")
	if idx < 0 {
		return false
	}
	return videoExts[strings.ToLower(name[idx+1:])]
}

// 播放列表的集号提取正则（针对 115 云端种子原始文件名）。
var (
	regPlayEp   = regexp.MustCompile(`(?i)(?:^|[^0-9])(?:e|ep)[\s._-]*(\d+)`) // S01E01 / EP03 / E01
	regPlayCn   = regexp.MustCompile(`第\s*(\d+)`)                            // 第01话
	regPlayDash = regexp.MustCompile(`[\s._\-[(](\d{2,3})[\s.\])]`)          // - 04 / [04] / _01
)

// extractEpisode 从 115 云端文件名（种子原始名）中尽力提取集号。
// 无法识别返回 0。
func extractEpisode(filename string) int {
	for _, re := range []*regexp.Regexp{regPlayEp, regPlayCn, regPlayDash} {
		if m := re.FindStringSubmatch(filename); len(m) > 1 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				return n
			}
		}
	}
	return 0
}
