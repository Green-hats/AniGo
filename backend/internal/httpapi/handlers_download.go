package httpapi

import (
	"context"

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

// handlePlayList 返回订阅在 115 云端目录下的可播放文件列表（用于前端播放弹窗）。
func (s *Server) handlePlayList(c *gin.Context) {
	var body domain.IdDTO
	if !readJSONOrFail(c, &body) {
		return
	}
	ani := s.ani.FindAniByID(body.ID)
	if ani == nil {
		fail(c, "订阅不存在")
		return
	}
	items, err := s.download.PlayList(c.Request.Context(), ani)
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, items)
}

// handleRefreshAll 触发一轮全部订阅刷新。
// 手动刷新是"发起即返回"的用户操作，须用与请求无关的 ctx，
// 否则 handler 返回后请求 ctx 被取消，后台刷新会立即中止。
func (s *Server) handleRefreshAll(c *gin.Context) {
	go s.download.SyncDownload(context.Background(), s.cfg.AniList())
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
	go s.download.DownloadAni(context.Background(), ani)
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