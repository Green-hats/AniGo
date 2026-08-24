package log

import (
	"sync"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

// 日志级别。
const (
	LevelDebug = "DEBUG"
	LevelInfo  = "INFO"
	LevelWarn  = "WARN"
	LevelError = "ERROR"
)

// Logger 是内存环形缓冲日志器。
// 供后台任务/下载/通知写入，前端通过 /api/logs 读取。
type Logger struct {
	mu    sync.RWMutex
	ring  []domain.Log
	max   int
}

// New 创建日志器，cap 为环形缓冲容量。
func New(cap int) *Logger {
	if cap <= 0 {
		cap = 256
	}
	return &Logger{ring: make([]domain.Log, 0, cap), max: cap}
}

// SetMax 调整环形缓冲容量。
func (l *Logger) SetMax(n int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n <= 0 {
		n = 256
	}
	l.max = n
	if len(l.ring) > l.max {
		l.ring = l.ring[len(l.ring)-l.max:]
	}
}

// push 追加一条日志。
func (l *Logger) push(level, logger, msg string) {
	entry := domain.Log{
		Message:    msg,
		Level:      level,
		LoggerName: logger,
		ThreadName: time.Now().Format("15:04:05"),
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ring = append(l.ring, entry)
	if len(l.ring) > l.max {
		l.ring = l.ring[len(l.ring)-l.max:]
	}
}

// Log 记录一条带级别的日志。
func (l *Logger) Log(level, logger, msg string) { l.push(level, logger, msg) }

// Debug 记录 DEBUG 级日志。
func (l *Logger) Debug(logger, msg string) { l.push(LevelDebug, logger, msg) }

// Info 记录 INFO 级日志。
func (l *Logger) Info(logger, msg string) { l.push(LevelInfo, logger, msg) }

// Warn 记录 WARN 级日志。
func (l *Logger) Warn(logger, msg string) { l.push(LevelWarn, logger, msg) }

// Error 记录 ERROR 级日志。
func (l *Logger) Error(logger, msg string) { l.push(LevelError, logger, msg) }

// List 返回当前所有日志（最新的在末尾）。
func (l *Logger) List() []domain.Log {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]domain.Log, len(l.ring))
	copy(out, l.ring)
	return out
}

// Clear 清空日志。
func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ring = l.ring[:0]
}