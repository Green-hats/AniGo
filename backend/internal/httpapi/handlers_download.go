package httpapi

import (
	"github.com/gin-gonic/gin"

	"github.com/greenhats/anigo/internal/domain"
)

// handleDownloadLoginTest 测试网盘登录。
func (s *Server) handleDownloadLoginTest(c *gin.Context) {
	var body domain.Config
	if err := c.ShouldBindJSON(&body); err == nil && body.Pan115Cookie != "" {
		// 允许用请求体里的 Cookie 临时测试
		cur := *s.cfg.Get()
		cur.Pan115Cookie = body.Pan115Cookie
		if ok, _ := s.download.Driver().Login(c.Request.Context(), true, &cur); ok {
			okMsg(c, "登录成功")
			return
		}
		fail(c, "登录失败")
		return
	}
	if !s.download.Login(true) {
		fail(c, "登录失败")
		return
	}
	okMsg(c, "登录成功")
}

// handleDownloadStatus 返回网盘登录状态。
func (s *Server) handleDownloadStatus(c *gin.Context) {
	ok(c, s.download.DownloadLoginStatus())
}

// handleRefreshAll 触发一轮全部订阅刷新。
func (s *Server) handleRefreshAll(c *gin.Context) {
	go s.download.SyncDownload(s.cfg.AniList())
	okMsg(c, "已开始刷新RSS")
}

// handleRefreshAni 触发单个订阅刷新。
func (s *Server) handleRefreshAni(c *gin.Context) {
	var body domain.IdDTO
	if !readJSONOrFail(c, &body) {
		return
	}
	ani := s.ani.FindAniByID(body.ID)
	if ani == nil {
		fail(c, "订阅不存在")
		return
	}
	go s.download.DownloadAni(ani)
	okMsg(c, "已开始刷新RSS")
}

// handleDeleteTorrent 删除云端任务/目录。
func (s *Server) handleDeleteTorrent(c *gin.Context) {
	var body struct {
		SavePath string `json:"savePath"`
		DeleteFiles bool `json:"deleteFiles"`
	}
	if !readJSONOrFail(c, &body) {
		return
	}
	if body.DeleteFiles && body.SavePath != "" {
		cfg := s.cfg.Get()
		if err := s.download.Driver().DeleteDir(c.Request.Context(), cfg, body.SavePath); err != nil {
			fail(c, err.Error())
			return
		}
	}
	okMsg(c, "删除成功")
}