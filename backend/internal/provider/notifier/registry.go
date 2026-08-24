package notifier

import (
	"context"
	"sort"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/provider/base"
)

// Registry 是按类型构建通知器的注册表。
type Registry struct {
	base   *Notifier
	logFn  func(msg string)
	byType map[domain.NotificationTypeEnum]domain.Notifier
}

// NewRegistry 创建通知器注册表并构建所有内置通知器。
func NewRegistry(cfg base.ConfigProvider, logFn func(msg string)) *Registry {
	r := &Registry{
		base:   New(cfg),
		logFn:  logFn,
		byType: map[domain.NotificationTypeEnum]domain.Notifier{},
	}
	// 注册内置通知器
	r.register(&Telegram{Notifier: r.base})
	r.register(&Bark{Notifier: r.base})
	r.register(&ServerChan{Notifier: r.base})
	r.register(&WebHook{Notifier: r.base})
	r.register(&Shell{Notifier: r.base})
	r.register(&System{Notifier: r.base, LogFn: logFn})
	return r
}

func (r *Registry) register(n domain.Notifier) {
	if n != nil && n.Type() != "" {
		r.byType[n.Type()] = n
	}
}

// Get 返回指定类型的通知器，未注册返回 nil。
func (r *Registry) Get(t domain.NotificationTypeEnum) domain.Notifier {
	return r.byType[t]
}

// Dispatch 将一条通知分发到所有启用的、匹配状态的渠道。
// 按 Sort 排序，异步重试，不阻塞调用方。
func (r *Registry) Dispatch(ctx context.Context, cfg *domain.Config, n *domain.Notification) {
	if n.Ani != nil && !n.Ani.Message {
		return
	}
	list := append([]domain.NotificationConfig(nil), cfg.NotificationConfigList...)
	sort.SliceStable(list, func(i, j int) bool { return list[i].Sort < list[j].Sort })
	for _, nc := range list {
		if !nc.Enable {
			continue
		}
		matched := false
		for _, s := range nc.StatusList {
			if s == n.Status {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		notifier := r.Get(nc.NotificationType)
		if notifier == nil {
			continue
		}
		ncfg := nc
		go func() {
			retry := ncfg.Retry
			if retry <= 0 {
				retry = 1
			}
			for i := 0; i < retry; i++ {
				if i > 0 {
					// 短暂等待后重试
					_ = i
				}
				if err := notifier.Send(ctx, &ncfg, n); err == nil {
					return
				}
			}
		}()
	}
}