package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/log"
	"github.com/greenhats/anigo/internal/rename"
)

// DownloadService 是下载主循环：登录 → 遍历订阅 → 解析 RSS → 查重 →
// 调网盘离线下载 → 缺集/摸鱼/完结检测。
type DownloadService struct {
	cfg    *ConfigService
	rss    *RssService
	cloud  CloudProvider
	cache  domain.Cache
	meta   *MetadataService
	notify *NotifyService
	logger *log.Logger

	playMu    sync.Mutex
	playCache map[string]*playCacheEntry // 播放列表短缓存（key=订阅id）
}

// playCacheEntry 是播放列表的短缓存条目。
type playCacheEntry struct {
	items   []domain.PlayItem
	expire  time.Time
}

// CloudProvider 是下载服务对网盘注册表的依赖接口，
// 由 cloud.Registry 实现（避免 service 依赖具体适配器）。
type CloudProvider interface {
	// Get 返回当前配置对应的网盘驱动。
	Get(cfg *domain.Config) domain.CloudDriver
}

var (
	regSeasonEp = regexp.MustCompile(`[Ss](\d+)[Ee](\d+(\.5)?)`)
	downloadMutex = make(chan struct{}, 1)
)

// SeasonEpisodeRe 暴露 SxxExx 正则供其他包使用。
var SeasonEpisodeRe = regSeasonEp

// NewDownloadService 创建下载服务。
func NewDownloadService(cfg *ConfigService, rss *RssService, cloud CloudProvider, cache domain.Cache, meta *MetadataService, notify *NotifyService, logger *log.Logger) *DownloadService {
	return &DownloadService{cfg: cfg, rss: rss, cloud: cloud, cache: cache, meta: meta, notify: notify, logger: logger, playCache: map[string]*playCacheEntry{}}
}

// pathResolve 返回下载路径的 bgmId/jpTitle 解析回调。
func (s *DownloadService) pathResolve() func(ani *domain.Ani) (string, string) {
	if s.meta == nil {
		return nil
	}
	return func(ani *domain.Ani) (string, string) {
		return s.meta.BgmSubjectId(context.Background(), ani), ani.JpTitle
	}
}

// Driver 返回当前网盘驱动。
func (s *DownloadService) Driver() domain.CloudDriver {
	cfg := s.cfg.Get()
	d := s.cloud.Get(cfg)
	if d == nil {
		return &NoopDriver{}
	}
	return d
}

// Login 测试网盘登录。
func (s *DownloadService) Login(test bool) bool {
	cfg := s.cfg.Get()
	ok, _ := s.Driver().Login(context.Background(), test, cfg)
	return ok
}

// DownloadLoginStatus 返回网盘登录状态。
func (s *DownloadService) DownloadLoginStatus() domain.LoginStatus {
	return s.Driver().GetLoginStatus()
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

// SyncDownload 运行一轮下载（遍历所有启用的订阅）。
func (s *DownloadService) SyncDownload(list []*domain.Ani) {
	cfg := s.cfg.Get()
	if ok, _ := s.Driver().Login(context.Background(), true, cfg); !ok {
		st := s.Driver().GetLoginStatus()
		msg := st.Message
		if msg == "" {
			msg = "未知原因"
		}
		s.logf("WARN", "download", "下载客户端登录失败, 跳过本轮: %s", msg)
		return
	}
	for _, ani := range list {
		if ani == nil || !ani.Enable {
			continue
		}
		s.downloadAni(ani, false)
		time.Sleep(500 * time.Millisecond)
	}
}

// DownloadAni 是每个订阅的下载主流程（公共入口，带登录检查）。
func (s *DownloadService) DownloadAni(ani *domain.Ani) {
	s.downloadAni(ani, true)
}

// downloadAni 是 DownloadAni 的内部实现。
// checkLogin 为 true 时先验证网盘登录，Cookie 失效直接返回，避免盲目提交失败任务。
func (s *DownloadService) downloadAni(ani *domain.Ani, checkLogin bool) {
	downloadMutex <- struct{}{}
	defer func() { <-downloadMutex }()
	defer func() {
		if r := recover(); r != nil {
			s.logf("ERROR", "download", "DownloadAni panic: %v", r)
		}
	}()

	cfg := s.cfg.Get()
	if checkLogin {
		if ok, _ := s.Driver().Login(context.Background(), true, cfg); !ok {
			st := s.Driver().GetLoginStatus()
			msg := st.Message
			if msg == "" {
				msg = "未知原因"
			}
			s.logf("WARN", "download", "%s 下载客户端登录失败, 跳过: %s", ani.Title, msg)
			return
		}
	}

	items := s.rss.GetItems(ani)
	s.logf("INFO", "download", "%s 刷新完成, 共 %d 个条目", ani.Title, len(items))
	s.RssOmit(ani, items)
	s.RssProcrastinating(ani, items)

	savePath := GetDownloadPath(cfg, ani, s.pathResolve())
	driver := s.Driver()
	sync := false
	currentDownloadCount := 0

	for _, item := range items {
		reName := item.ReName
		hash := strings.ToLower(item.InfoHash)
		episode := item.Episode
		is5 := rename.Is5(episode)

		if s.cache.Contains("hash:" + hash) {
			if item.Master && !is5 {
				currentDownloadCount++
			}
			continue
		}

		// 已下载过的资源（infoHash 持久化，重启后不重复提交）
		if containsStr(ani.DownloadedHash, hash) {
			if item.Master && !is5 {
				currentDownloadCount++
			}
			continue
		}

		// 已下载过的集（持久化，重启后不重复下载）
		if containsFloat(ani.Downloaded, episode) {
			if item.Master && !is5 {
				currentDownloadCount++
			}
			continue
		}

		// 用户显式标记不下载的集数
		if containsFloat(ani.NotDownload, episode) {
			if item.Master && !is5 {
				currentDownloadCount++
			}
			continue
		}

		// 只下载最新发布的集（DownloadNew）
		if ani.DownloadNew {
			newItem := items[len(items)-1]
			if !item.PubDate.Time().IsZero() && !newItem.PubDate.Time().IsZero() {
				if item.PubDate.Time().Format("2006-01-02") != newItem.PubDate.Time().Format("2006-01-02") {
					if item.Master && !is5 {
						currentDownloadCount++
					}
					continue
				}
			} else if item != newItem {
				if item.Master && !is5 {
					currentDownloadCount++
				}
				continue
			}
		}

		// 延迟下载（发布时间距今不足 DelayedDownload 分钟的暂不下）
		if !item.PubDate.Time().IsZero() && cfg.DelayedDownload > 0 {
			if time.Now().Add(-time.Duration(cfg.DelayedDownload) * time.Minute).Before(item.PubDate.Time()) {
				continue
			}
		}

		if item.Master && !is5 {
			currentDownloadCount++
		}

		// 提交离线下载（异步，115 自行转存）
		if err := driver.AddOfflineTask(context.Background(), cfg, item.Torrent, savePath+"/"+reName); err != nil {
			s.logf("ERROR", "download", "%s 添加下载失败: %v", reName, err)
			continue
		}
		s.logf("INFO", "download", "添加下载 %s → %s", reName, savePath)
		s.cache.Put("hash:"+hash, reName, 24*time.Hour)
		ani.Downloaded = append(ani.Downloaded, episode)
		ani.DownloadedHash = append(ani.DownloadedHash, hash)
		sync = true
		s.notifySend(ani, reName, item.Master, domain.NotifyDownloadStart)
	}

	if sync {
		ani.DownloadedEps = currentDownloadCount
		ani.CurrentEpisodeNumber = s.rss.CurrentEpisodeNumber(ani, items)
		ani.LastDownloadTime = domain.NowMillis()
		s.logf("INFO", "download", "%s 已下载 %d 集, RSS 更新到 %d 集", ani.Title, currentDownloadCount, ani.CurrentEpisodeNumber)
		if err := s.cfg.SaveAniList(s.cfg.AniList()); err != nil {
			s.logf("ERROR", "download", "保存订阅失败: %v", err)
		}
	}

	if !cfg.AutoDisabled {
		return
	}
	if ani.TotalEpisodeNumber < 1 {
		return
	}
	if currentDownloadCount >= ani.TotalEpisodeNumber {
		ani.Enable = false
		_ = s.cfg.SaveAniList(s.cfg.AniList())
		s.notifySend(ani, fmt.Sprintf("%s 订阅已完结", ani.Title), true, domain.NotifyCompleted)
	}
}

// logf 写入下载日志（logger 未注入时静默跳过）。
func (s *DownloadService) logf(level, logger, format string, args ...interface{}) {
	if s.logger == nil {
		return
	}
	s.logger.Log(level, logger, fmt.Sprintf(format, args...))
}

// notifySend 发送下载相关通知（notify 未注入时静默跳过）。
func (s *DownloadService) notifySend(ani *domain.Ani, text string, master bool, status domain.NotificationStatusEnum) {
	if s.notify == nil {
		return
	}
	if !master {
		text = "(备用RSS) " + text
	}
	s.notify.Send(context.Background(), ani, text, status)
}

// RssOmit 通知缺失集数。
func (s *DownloadService) RssOmit(ani *domain.Ani, items []*domain.Item) {
	cfg := s.cfg.Get()
	if !cfg.Omit || !ani.Omit || ani.Ova || len(items) == 0 {
		return
	}
	distinct := map[int]bool{}
	for _, it := range items {
		distinct[int(it.Episode)] = true
	}
	eps := make([]int, 0, len(distinct))
	for e := range distinct {
		eps = append(eps, e)
	}
	sort.Ints(eps)
	if len(eps) == 0 {
		return
	}
	min, max := eps[0], eps[len(eps)-1]
	if min == max {
		return
	}
	var missing []int
	for ep := min; ep <= max; ep++ {
		if distinct[ep] {
			continue
		}
		if len(missing) > 50 {
			return
		}
		missing = append(missing, ep)
	}
	if len(missing) == 0 || len(missing) > 10 {
		return
	}
	var sList []string
	for _, ep := range missing {
		msg := fmt.Sprintf("缺少集数 %s S%02dE%02d", ani.Title, ani.Season, ep)
		key := fmt.Sprintf("omit:%s:ep-%d", ani.ID, ep)
		if s.cache.Contains(key) {
			continue
		}
		s.cache.Put(key, msg, 24*time.Hour)
		sList = append(sList, msg)
	}
	if len(sList) > 0 {
		s.notifySend(ani, strings.Join(sList, "\n"), true, domain.NotifyOmit)
	}
}

// RssProcrastinating 检测摸鱼（最新发布很久没更新）。
func (s *DownloadService) RssProcrastinating(ani *domain.Ani, items []*domain.Item) {
	cfg := s.cfg.Get()
	if !cfg.Procrastinating || !ani.Procrastinating {
		return
	}
	if cfg.ProcrastinatingMasterOnly {
		var master []*domain.Item
		for _, it := range items {
			if it.Master {
				master = append(master, it)
			}
		}
		items = master
	}
	var latest time.Time
	for _, it := range items {
		if !it.PubDate.Time().IsZero() && it.PubDate.Time().After(latest) {
			latest = it.PubDate.Time()
		}
	}
	if latest.IsZero() || latest.After(time.Now()) {
		return
	}
	day := int(time.Since(latest).Hours() / 24)
	if day < cfg.ProcrastinatingDay {
		return
	}
	text := fmt.Sprintf("检测到%s, 已摸鱼%d天", ani.Title, day)
	key := "procrastinating:" + ani.ID
	if s.cache.Contains(key) {
		return
	}
	s.cache.Put(key, text, 24*time.Hour)
	s.notifySend(ani, text, true, domain.NotifyProcrastinating)
}

// containsFloat 判断浮点列表是否包含某值。
func containsFloat(list []float64, v float64) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
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


// NoopDriver 是空驱动占位。
type NoopDriver struct{}

// Name 返回空驱动名。
func (n *NoopDriver) Name() string { return "none" }

// Login 返回未配置。
func (n *NoopDriver) Login(ctx context.Context, test bool, cfg *domain.Config) (bool, error) {
	return false, nil
}

// AddOfflineTask 返回未配置错误。
func (n *NoopDriver) AddOfflineTask(ctx context.Context, cfg *domain.Config, magnet, destPath string) error {
	return fmt.Errorf("未配置网盘驱动")
}

// FileExists 返回 false。
func (n *NoopDriver) FileExists(ctx context.Context, cfg *domain.Config, path string) (bool, error) {
	return false, nil
}

// FileURL 返回空串。
func (n *NoopDriver) FileURL(ctx context.Context, cfg *domain.Config, path string) (string, error) {
	return "", nil
}

// FileURLByPickCode 返回空串。
func (n *NoopDriver) FileURLByPickCode(ctx context.Context, cfg *domain.Config, pickcode string) (string, error) {
	return "", nil
}

// ListDir 返回空。
func (n *NoopDriver) ListDir(ctx context.Context, cfg *domain.Config, path string) ([]domain.CloudFile, error) {
	return nil, nil
}

// DeleteDir 返回 nil。
func (n *NoopDriver) DeleteDir(ctx context.Context, cfg *domain.Config, path string) error { return nil }

// GetLoginStatus 返回未配置状态。
func (n *NoopDriver) GetLoginStatus() domain.LoginStatus {
	return domain.LoginStatus{Configured: false, Message: "未配置网盘"}
}