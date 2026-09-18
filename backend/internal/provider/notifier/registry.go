package notifier

import (
	"context"
	"sync"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/provider/base"
)

// Registry 是按类型构建通知器的注册表。
type Registry struct {
	base      *Notifier
	logFn     func(msg string)
	byType    map[domain.NotificationTypeEnum]domain.Notifier
	mu        sync.Mutex
	lifecycle sync.Mutex
	records   map[string]*delivery
	queue     chan string
	cancel    context.CancelFunc
	ctx       context.Context
	wg        sync.WaitGroup
	backoff   time.Duration
}

// NewRegistry 创建通知器注册表并构建所有内置通知器。
func NewRegistry(cfg base.ConfigProvider, logFn func(msg string)) *Registry {
	r := &Registry{
		base:    New(cfg),
		records: map[string]*delivery{},
		backoff: time.Second,
		logFn:   logFn,
		byType:  map[domain.NotificationTypeEnum]domain.Notifier{},
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
