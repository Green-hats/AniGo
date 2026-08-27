package log

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

// 日志级别（按数值升序，数值越大级别越高）。
const (
	LevelDebug = "DEBUG"
	LevelInfo  = "INFO"
	LevelWarn  = "WARN"
	LevelError = "ERROR"
)

// levelRank 返回级别的数值（未知级别视为 INFO）。
func levelRank(level string) int {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case LevelDebug:
		return 0
	case LevelInfo:
		return 1
	case LevelWarn:
		return 2
	case LevelError:
		return 3
	default:
		return 1
	}
}

// Logger 是内存环形缓冲日志器，可选追加到磁盘文件。
// 供后台任务/下载/通知写入，前端通过 /api/logs 读取。
type Logger struct {
	mu       sync.RWMutex
	ring     []domain.Log
	max      int
	minLevel int // 低于该级别的日志不记录（内存与文件均过滤）
	file     *os.File
	filePath string
}

// New 创建日志器，cap 为环形缓冲容量。
func New(cap int) *Logger {
	if cap <= 0 {
		cap = 256
	}
	return &Logger{ring: make([]domain.Log, 0, cap), max: cap, minLevel: levelRank(LevelInfo)}
}

// SetLevel 设置最小记录级别，低于该级别的日志被丢弃（不进入内存也不落盘）。
func (l *Logger) SetLevel(level string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minLevel = levelRank(level)
}

// SetFile 启用/停用磁盘落盘。path 为空时关闭文件；否则以追加模式打开。
// 调用方应保证父目录存在；文件不存在时自动创建。
func (l *Logger) SetFile(path string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if path == l.filePath && path != "" {
		return nil
	}
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	l.filePath = path
	if path == "" {
		return nil
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	l.file = f
	return nil
}

// Close 关闭落盘文件。
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	l.filePath = ""
}

// push 追加一条日志。低于最小级别时直接丢弃；启用了落盘时同步写入文件。
func (l *Logger) push(level, logger, msg string) {
	entry := domain.Log{
		Message:    msg,
		Level:      level,
		LoggerName: logger,
		ThreadName: time.Now().Format("15:04:05"),
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if levelRank(entry.Level) < l.minLevel {
		return
	}
	l.ring = append(l.ring, entry)
	if len(l.ring) > l.max {
		l.ring = l.ring[len(l.ring)-l.max:]
	}
	if l.file != nil {
		ts := time.Now().Format("2006-01-02 15:04:05")
		_, _ = fmt.Fprintf(l.file, "[%s] [%s] [%s] %s\n", ts, entry.Level, entry.LoggerName, entry.Message)
	}
}

// Log 记录一条带级别的日志。
func (l *Logger) Log(level, logger, msg string) { l.push(level, logger, msg) }

// Info 记录 INFO 级日志。
func (l *Logger) Info(logger, msg string) { l.push(LevelInfo, logger, msg) }

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