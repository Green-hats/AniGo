package service

import (
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

// Logger 返回底层 logger（供任务/下载/通知写入）。
func (s *LogService) Logger() *log.Logger { return s.logger }