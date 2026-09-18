package httpapi

import (
	"github.com/gin-gonic/gin"
	"github.com/greenhats/anigo/internal/cloud"
	"strings"

	"github.com/greenhats/anigo/internal/domain"
)

// handleDownloadLoginTest 测试网盘登录。
func (s *Server) handleDownloadLoginTest(c *gin.Context) {
	var body struct {
		Tool     *string `json:"downloadToolType"`
		Cookie   *string `json:"pan115Cookie"`
		Email    *string `json:"pikpakEmail"`
		Password *string `json:"pikpakPassword"`
	}
	if !readJSONOrFail(c, &body) {
		return
	}
	cfg := s.cfg.Get()
	if body.Tool != nil {
		cfg.DownloadToolType = strings.ToLower(strings.TrimSpace(*body.Tool))
	}
	if body.Cookie != nil {
		cfg.Pan115Cookie = *body.Cookie
	}
	if body.Email != nil {
		cfg.PikpakEmail = *body.Email
	}
	if body.Password != nil {
		cfg.PikpakPassword = *body.Password
	}
	switch cfg.DownloadToolType {
	case "", "115", "pan115", "pikpak":
	default:
		fail(c, "不支持的网盘类型")
		return
	}
	// A separate driver tests unsaved credentials without replacing the live session.
	driver := cloud.NewRegistry().Get(cfg)
	success, err := driver.Login(c.Request.Context(), true, cfg)
	if err != nil {
		fail(c, err.Error())
		return
	}
	if !success {
		fail(c, driver.GetLoginStatus().Message)
		return
	}
	okMsg(c, "登录成功")
}

// handleDownloadStatus 返回网盘登录状态。
func (s *Server) handleDownloadStatus(c *gin.Context) {
	ok(c, s.download.DownloadLoginStatus())
}

// handlePlayList 返回订阅在所选网盘目录下的可播放文件列表（用于前端播放弹窗）。
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
	if err := s.download.EnqueueAll(); err != nil {
		fail(c, err.Error())
		return
	}
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
	if !ani.Enable {
		fail(c, "订阅已停用")
		return
	}
	if err := s.download.EnqueueRefresh(ani.ID); err != nil {
		fail(c, err.Error())
		return
	}
	okMsg(c, "已开始刷新RSS")
}

// handleDeleteTorrent 删除云端任务/目录。
func (s *Server) handleDeleteTorrent(c *gin.Context) {
	var body struct {
		SavePath    string `json:"savePath"`
		DeleteFiles bool   `json:"deleteFiles"`
	}
	if !readJSONOrFail(c, &body) {
		return
	}
	if body.DeleteFiles && body.SavePath != "" {
		cfg := s.cfg.Get()
		if err := s.download.DriverForConfig(cfg).DeleteDir(c.Request.Context(), cfg, body.SavePath); err != nil {
			fail(c, err.Error())
			return
		}
	}
	okMsg(c, "删除成功")
}

// Recovery changes local intent only; the regular queue owns network execution.
func (s *Server) handleRecoverTask(c *gin.Context) {
	var body struct {
		ID      string  `json:"id"`
		Hash    string  `json:"hash"`
		Episode float64 `json:"episode"`
		Action  string  `json:"action"`
	}
	if !readJSONOrFail(c, &body) {
		return
	}
	if err := s.download.RecoverTask(c.Request.Context(), body.ID, body.Hash, body.Episode, body.Action); err != nil {
		fail(c, err.Error())
		return
	}
	if err := s.download.EnqueueRefresh(body.ID); err != nil {
		okMsg(c, "恢复操作已保存，请稍后点击刷新执行")
		return
	}
	okMsg(c, "恢复操作已保存并加入刷新队列")
}
