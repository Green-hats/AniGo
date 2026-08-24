package httpapi

import (
	"github.com/gin-gonic/gin"
)

// handleLogs 返回当前日志（按时间倒序，最新的在前）。
func (s *Server) handleLogs(c *gin.Context) {
	list := s.logs.List()
	// 倒序：前端展示最新的在最上面
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
	ok(c, list)
}

// handleClearLogs 清空日志。
func (s *Server) handleClearLogs(c *gin.Context) {
	s.logs.Clear()
	okMsg(c, "已清空日志")
}