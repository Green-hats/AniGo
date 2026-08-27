package service

import (
	"context"
	"fmt"

	"github.com/greenhats/anigo/internal/domain"
)

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
