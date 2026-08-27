package service

import (
	"path/filepath"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/log"
)

// LogService 封装日志读写，供 HTTP 层暴露 /api/logs。
type LogService struct {
	logger *log.Logger
}

// NewLogService 创建日志服务。
func NewLogService(logger *log.Logger) *LogService {
	return &LogService{logger: logger}
}

// List 返回当前日志。
func (s *LogService) List() []domain.Log { return s.logger.List() }

// Clear 清空日志。
func (s *LogService) Clear() { s.logger.Clear() }

// Reload 按最新配置调整日志级别与落盘。
// dir 为配置目录，logsFile 为相对路径（空表示不落盘）。
func (s *LogService) Reload(dir string, cfg *domain.Config) {
	s.logger.SetLevel(cfg.LogsLevel)
	path := ""
	if cfg.LogsFile != "" {
		path = filepath.Join(dir, cfg.LogsFile)
	}
	if err := s.logger.SetFile(path); err != nil {
		s.logger.Error("log", "日志落盘打开失败: "+err.Error())
	}
}