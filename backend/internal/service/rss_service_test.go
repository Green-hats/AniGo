package service

import (
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

func TestCurrentEpisodeNumberDedup(t *testing.T) {
	s := &RssService{cfg: &ConfigService{cfg: &domain.Config{}}}
	// 模拟 12 集番剧，每集 2 个版本 → 24 个条目
	var items []*domain.Item
	for ep := 1; ep <= 12; ep++ {
		items = append(items,
			&domain.Item{Episode: float64(ep), Master: true},
			&domain.Item{Episode: float64(ep), Master: true},
		)
	}
	ani := &domain.Ani{DownloadNew: false}
	if got := s.CurrentEpisodeNumber(ani, items); got != 12 {
		t.Errorf("按集去重应返回 12, got %d", got)
	}
}

func TestCurrentEpisodeNumberDownloadNew(t *testing.T) {
	s := &RssService{cfg: &ConfigService{cfg: &domain.Config{}}}
	var items []*domain.Item
	for ep := 1; ep <= 3; ep++ {
		items = append(items, &domain.Item{Episode: float64(ep), Master: true})
	}
	ani := &domain.Ani{DownloadNew: true}
	if got := s.CurrentEpisodeNumber(ani, items); got != 3 {
		t.Errorf("DownloadNew 应返回最大集 3, got %d", got)
	}
}

func TestAIBackoff(t *testing.T) {
	s := &RssService{failCount: map[string]int{}, failTime: map[string]time.Time{}}
	key := "ani1|url"
	// 未达阈值不应退避
	if s.aiInBackoff(key) {
		t.Error("初始不应退避")
	}
	// 连续失败达阈值
	for i := 0; i < aiFailBackoffThreshold; i++ {
		s.aiRecordFail(key)
	}
	if !s.aiInBackoff(key) {
		t.Error("达阈值后应立即退避")
	}
	// 成功重置后不再退避
	s.aiResetFail(key)
	if s.aiInBackoff(key) {
		t.Error("重置后不应退避")
	}
}