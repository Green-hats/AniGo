package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
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

// CloudProvider 是下载服务对网盘注册表的依赖接口，
// 由 cloud.Registry 实现（避免 service 依赖具体适配器）。
type CloudProvider interface {
	// Get 返回当前配置对应的网盘驱动。
	Get(cfg *domain.Config) domain.CloudDriver
}

var (
	regSeasonEp   = regexp.MustCompile(`[Ss](\d+)[Ee](\d+(\.5)?)`)
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

// SyncDownload 运行一轮下载（遍历所有启用的订阅）。
func (s *DownloadService) SyncDownload(ctx context.Context, list []*domain.Ani) {
	cfg := s.cfg.Get()
	if ok, _ := s.Driver().Login(ctx, true, cfg); !ok {
		st := s.Driver().GetLoginStatus()
		msg := st.Message
		if msg == "" {
			msg = "未知原因"
		}
		s.logf("WARN", "download", "下载客户端登录失败, 跳过本轮: %s", msg)
		return
	}
	for _, ani := range list {
		if ctx.Err() != nil {
			s.logf("INFO", "download", "下载同步被取消, 提前结束本轮")
			return
		}
		if ani == nil || !ani.Enable {
			continue
		}
		s.downloadAni(ctx, ani, false)
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// DownloadAni 是每个订阅的下载主流程（公共入口，带登录检查）。
func (s *DownloadService) DownloadAni(ctx context.Context, ani *domain.Ani) {
	s.downloadAni(ctx, ani, true)
}

// downloadAni 是 DownloadAni 的内部实现。
// checkLogin 为 true 时先验证网盘登录，Cookie 失效直接返回，避免盲目提交失败任务。
func (s *DownloadService) downloadAni(ctx context.Context, ani *domain.Ani, checkLogin bool) {
	downloadMutex <- struct{}{}
	defer func() { <-downloadMutex }()
	defer func() {
		if r := recover(); r != nil {
			s.logf("ERROR", "download", "DownloadAni panic: %v", r)
		}
	}()

	cfg := s.cfg.Get()
	if checkLogin {
		if ok, _ := s.Driver().Login(ctx, true, cfg); !ok {
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
		if ctx.Err() != nil {
			s.logf("INFO", "download", "%s 下载被取消, 提前终止", ani.Title)
			return
		}
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
		if err := driver.AddOfflineTask(ctx, cfg, item.Torrent, savePath+"/"+reName); err != nil {
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
