package httpapi

import (
	"encoding/json"
	"sync"

	"github.com/gin-gonic/gin"

	driver115 "github.com/greenhats/anigo/internal/cloud/driver_115"
)

// qrSessionStore 保存进行中的扫码会话（单用户场景，一把锁足够）。
type qrSessionStore struct {
	mu      sync.Mutex
	session *driver115.QRSession
}

var qrStore = &qrSessionStore{}

// qrDriver 返回 115 驱动（仅当下载器配置为 115 时可用）。
func (s *Server) qrDriver() (*driver115.Pan115, bool) {
	d, ok := s.download.Driver().(*driver115.Pan115)
	return d, ok
}

// handleQRCreate 创建扫码会话，返回二维码内容。
func (s *Server) handleQRCreate(c *gin.Context) {
	d, is115 := s.qrDriver()
	if !is115 {
		fail(c, "当前下载器不是 115")
		return
	}
	sess, err := d.QRCreate(c.Request.Context(), s.cfg.Get())
	if err != nil {
		fail(c, err.Error())
		return
	}
	qrStore.mu.Lock()
	qrStore.session = sess
	qrStore.mu.Unlock()
	ok(c, gin.H{"qrText": sess.QRText})
}

// handleQRStatus 轮询扫码状态；confirmed 时自动换取 Cookie 并写入配置。
func (s *Server) handleQRStatus(c *gin.Context) {
	d, is115 := s.qrDriver()
	if !is115 {
		fail(c, "当前下载器不是 115")
		return
	}
	qrStore.mu.Lock()
	sess := qrStore.session
	qrStore.mu.Unlock()
	if sess == nil {
		fail(c, "没有进行中的扫码会话")
		return
	}
	status, err := d.QRStatusText(c.Request.Context(), s.cfg.Get(), sess)
	if err != nil {
		fail(c, err.Error())
		return
	}
	if status != "confirmed" {
		ok(c, gin.H{"status": status})
		return
	}
	cookie, err := d.QRFetchCookie(c.Request.Context(), s.cfg.Get(), sess)
	if err != nil {
		fail(c, err.Error())
		return
	}
	raw, err := json.Marshal(map[string]string{"pan115Cookie": cookie})
	if err != nil {
		fail(c, err.Error())
		return
	}
	if err := s.cfg.SetConfigRaw(raw); err != nil {
		fail(c, "Cookie 获取成功但保存配置失败: "+err.Error())
		return
	}
	qrStore.mu.Lock()
	qrStore.session = nil
	qrStore.mu.Unlock()
	ok(c, gin.H{"status": "confirmed"})
}

// handleQRCancel 作废当前会话。
func (s *Server) handleQRCancel(c *gin.Context) {
	qrStore.mu.Lock()
	qrStore.session = nil
	qrStore.mu.Unlock()
	okMsg(c, "已取消")
}
