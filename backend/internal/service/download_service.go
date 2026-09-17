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

	gate      chan struct{}
	queue     *RefreshQueue
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
	regSeasonEp = regexp.MustCompile(`[Ss](\d+)[Ee](\d+(\.5)?)`)
)

// SeasonEpisodeRe 暴露 SxxExx 正则供其他包使用。
var SeasonEpisodeRe = regSeasonEp

// NewDownloadService 创建下载服务。
func NewDownloadService(cfg *ConfigService, rss *RssService, cloud CloudProvider, cache domain.Cache, meta *MetadataService, notify *NotifyService, logger *log.Logger) *DownloadService {
	s := &DownloadService{gate: make(chan struct{}, 1), cfg: cfg, rss: rss, cloud: cloud, cache: cache, meta: meta, notify: notify, logger: logger, playCache: map[string]*playCacheEntry{}}
	s.queue = NewRefreshQueue(128, func(ctx context.Context, id string) error {
		ani := s.findAni(id)
		if ani == nil || !ani.Enable {
			return fmt.Errorf("订阅已删除或停用")
		}
		return s.DownloadAni(ctx, ani)
	})
	return s
}

// pathResolve 返回下载路径的 bgmId/jpTitle 解析回调。
func (s *DownloadService) pathResolve(ctx context.Context) func(ani *domain.Ani) (string, string) {
	if s.meta == nil {
		return nil
	}
	return func(ani *domain.Ani) (string, string) {
		return s.meta.BgmSubjectId(ctx, ani), ani.JpTitle
	}
}

// Driver 返回当前网盘驱动。
func (s *DownloadService) Driver() domain.CloudDriver {
	return s.DriverForConfig(s.cfg.Get())
}

// DriverForConfig keeps driver selection and credentials on the same snapshot.
func (s *DownloadService) DriverForConfig(cfg *domain.Config) domain.CloudDriver {
	d := s.cloud.Get(cfg)
	if d == nil {
		return &NoopDriver{}
	}
	return d
}

// Login 测试网盘登录。
func (s *DownloadService) Login(ctx context.Context, test bool) bool {
	cfg := s.cfg.Get()
	ok, _ := s.DriverForConfig(cfg).Login(ctx, test, cfg)
	return ok
}

// DownloadLoginStatus 返回网盘登录状态。
func (s *DownloadService) DownloadLoginStatus() domain.LoginStatus {
	return s.Driver().GetLoginStatus()
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
