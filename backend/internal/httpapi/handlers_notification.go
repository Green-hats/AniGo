package httpapi

import (
	"github.com/gin-gonic/gin"

	"github.com/greenhats/anigo/internal/domain"
)

// handleTestNotification 测试一条通知配置（同步发送）。
func (s *Server) handleTestNotification(c *gin.Context) {
	var body domain.NotificationConfig
	if !readJSONOrFail(c, &body) {
		return
	}
	if err := s.notify.Test(c.Request.Context(), &body, "测试通知"); err != nil {
		fail(c, "发送失败: "+err.Error())
		return
	}
	okMsg(c, "发送成功")
}