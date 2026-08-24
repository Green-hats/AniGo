package service

import (
	"context"
	"fmt"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/provider/notifier"
)

// NotifyService 封装通知分发，供下载等服务调用。
type NotifyService struct {
	cfg      *ConfigService
	registry *notifier.Registry
}

// NewNotifyService 创建通知服务。
func NewNotifyService(cfg *ConfigService, logFn func(msg string)) *NotifyService {
	return &NotifyService{
		cfg:      cfg,
		registry: notifier.NewRegistry(cfg, logFn),
	}
}

// Send 发送一条通知（异步分发）。
func (s *NotifyService) Send(ctx context.Context, ani *domain.Ani, text string, status domain.NotificationStatusEnum) {
	s.registry.Dispatch(ctx, s.cfg.Get(), &domain.Notification{
		Text:   text,
		Status: status,
		Ani:    ani,
	})
}

// Test 测试一条通知渠道配置（同步，返回错误）。
func (s *NotifyService) Test(ctx context.Context, nc *domain.NotificationConfig, text string) error {
	n := s.registry.Get(nc.NotificationType)
	if n == nil {
		return fmt.Errorf("不支持的通道类型 %s", nc.NotificationType)
	}
	return n.Send(ctx, nc, &domain.Notification{
		Text:   text,
		Status: domain.NotifyError,
		Ani:    sampleAni(),
	})
}

// sampleAni 构造一个示例订阅，用于测试通知时预览占位符渲染效果。
func sampleAni() *domain.Ani {
	return &domain.Ani{
		Title:                "示例番剧",
		JpTitle:              "サンプル",
		Season:               1,
		Subgroup:             "ANi",
		Score:                8.5,
		BgmUrl:               "https://bgm.tv/subject/123",
		CurrentEpisodeNumber: 1,
		TotalEpisodeNumber:   12,
		ReleaseDate:          domain.Date(time.Now()),
	}
}