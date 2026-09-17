package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const playTicketTTL = 3 * time.Hour

type playTicket struct {
	PickCode string `json:"p"`
	Expires  int64  `json:"e"`
}

func (s *Server) ticketSignature(payload string) []byte {
	cfg := s.cfg.Get()
	mac := hmac.New(sha256.New, s.playSecret)
	binding, _ := json.Marshal([]string{payload, cfg.Login.Password, cfg.DownloadToolType, cfg.Pan115Cookie, cfg.PikpakEmail, cfg.PikpakPassword})
	mac.Write(binding)
	return mac.Sum(nil)
}

func (s *Server) signPlayTicket(pickcode string, expiry time.Time) string {
	body, _ := json.Marshal(playTicket{pickcode, expiry.Unix()})
	payload := base64.RawURLEncoding.EncodeToString(body)
	return payload + "." + base64.RawURLEncoding.EncodeToString(s.ticketSignature(payload))
}

func (s *Server) validPlayTicket(token, pickcode string) bool {
	if len(token) > 2048 || pickcode == "" {
		return false
	}
	payload, signature, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil || !hmac.Equal(sig, s.ticketSignature(payload)) {
		return false
	}
	body, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return false
	}
	var ticket playTicket
	return json.Unmarshal(body, &ticket) == nil && ticket.PickCode == pickcode && ticket.Expires > time.Now().Unix()
}

// 只有已登录且能读取该订阅播放列表的用户可以申请单文件凭证。
func (s *Server) handlePlayTicket(c *gin.Context) {
	var body struct {
		ID       string `json:"id"`
		PickCode string `json:"pickCode"`
	}
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
	for _, item := range items {
		if body.PickCode == "" || item.PickCode != body.PickCode {
			continue
		}
		expiry := time.Now().Add(playTicketTTL)
		query := url.Values{"pickcode": {item.PickCode}, "ticket": {s.signPlayTicket(item.PickCode, expiry)}}
		c.Header("Cache-Control", "no-store")
		ok(c, gin.H{"url": "/api/file?" + query.Encode(), "expiresAt": expiry.UnixMilli()})
		return
	}
	fail(c, "文件不属于该订阅")
}
