package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
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

// PlayList 返回订阅在所选网盘目录下所有可播放文件（含提取的集号与 pickcode）。
// 递归遍历子目录：115 离线会把每个文件包成一个同名文件夹（fc=0 视为目录），
// 真正可播放的文件在文件夹内部（fc=1）。
// 不在此处取 CDN URL：URL 会过期且绑定 UA，改为播放时经本地代理转发获取。
// 结果做 30 秒短缓存，避免短时间内重复打开播放弹窗时反复遍历云端目录。
func (s *DownloadService) PlayList(ctx context.Context, ani *domain.Ani) ([]domain.PlayItem, error) {
	cfg := s.cfg.Get()
	fingerprint, _ := json.Marshal([]any{ani.ID, ani.Title, ani.Season, ani.BgmUrl, ani.Subgroup, ani.CustomDownloadPath, ani.CustomDownloadPathTemplate, cfg.DownloadPathTemplate, cfg.OvaDownloadPathTemplate, cfg.DownloadToolType, cfg.Pan115Cookie, cfg.PikpakEmail, cfg.PikpakPassword})
	key := fmt.Sprintf("%x", sha256.Sum256(fingerprint))
	s.playMu.Lock()
	if e, ok := s.playCache[key]; ok && time.Now().Before(e.expire) {
		items := slices.Clone(e.items)
		s.playMu.Unlock()
		return items, nil
	}
	s.playMu.Unlock()

	driver := s.cloud.Get(cfg)
	if driver == nil {
		return nil, fmt.Errorf("未配置网盘驱动")
	}
	if ok, err := driver.Login(ctx, true, cfg); err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("下载客户端登录失败: %s", driver.GetLoginStatus().Message)
	}
	savePath := GetDownloadPath(cfg, ani, s.pathResolve(ctx))
	items := []domain.PlayItem{}
	var walk func(path string, depth int) error
	walk = func(path string, depth int) error {
		if depth > 6 {
			return nil
		}
		files, err := driver.ListDir(ctx, cfg, path)
		if err != nil {
			return err
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
	for k, e := range s.playCache {
		if time.Now().After(e.expire) {
			delete(s.playCache, k)
		}
	}
	if len(s.playCache) >= 128 {
		for k := range s.playCache {
			delete(s.playCache, k)
			break
		}
	}
	s.playCache[key] = &playCacheEntry{items: slices.Clone(items), expire: time.Now().Add(30 * time.Second)}
	s.playMu.Unlock()
	return items, nil
}

// PlayStreamURL 通过 pickcode / 文件 ID 取云端直链（播放代理转发用）。
func (s *DownloadService) PlayStreamURL(ctx context.Context, pickcode string) (string, error) {
	cfg := s.cfg.Get()
	driver := s.cloud.Get(cfg)
	if driver == nil {
		return "", fmt.Errorf("未配置网盘驱动")
	}
	return driver.FileURLByPickCode(ctx, cfg, pickcode)
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
	regPlayCn   = regexp.MustCompile(`第\s*(\d+)`)                             // 第01话
	regPlayDash = regexp.MustCompile(`[\s._\-[(](\d{2,3})[\s.\])]`)           // - 04 / [04] / _01
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
