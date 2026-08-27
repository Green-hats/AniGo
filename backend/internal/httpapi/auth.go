package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/greenhats/anigo/internal/auth"
	"github.com/greenhats/anigo/internal/domain"
)

// session 是一次登录会话。
type session struct {
	token string
	exp   time.Time
}

// sessionStore 管理内存中的登录会话（单实例单进程）。
type sessionStore struct {
	mu       sync.RWMutex
	sessions map[string]session
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[string]session)}
}

// newToken 生成 32 字节随机 token。
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// create 创建会话并返回 token。顺带清理已过期会话，防止长期运行内存累积。
func (s *sessionStore) create(ttl time.Duration) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	now := time.Now()
	s.mu.Lock()
	s.purgeExpiredLocked(now)
	s.sessions[token] = session{token: token, exp: now.Add(ttl)}
	s.mu.Unlock()
	return token, nil
}

// purgeExpiredLocked 删除所有已过期会话（调用方需持写锁）。
func (s *sessionStore) purgeExpiredLocked(now time.Time) {
	for k, ss := range s.sessions {
		if now.After(ss.exp) {
			delete(s.sessions, k)
		}
	}
}

// valid 校验 token 是否有效，过期会话自动清理。
func (s *sessionStore) valid(token string) bool {
	s.mu.RLock()
	ss, ok := s.sessions[token]
	s.mu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(ss.exp) {
		s.delete(token)
		return false
	}
	return true
}

// delete 删除会话（登出）。
func (s *sessionStore) delete(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// publicPaths 免鉴权端点。
var publicPaths = map[string]bool{
	"/api/ping":       true,
	"/api/login":      true,
	"/api/custom.js":  true,
	"/api/custom.css": true,
}

// authMiddleware 校验所有 /api 请求的登录态。
// 白名单或密码为空（发布版首次配置）时放行。
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, "/api/") {
			c.Next()
			return
		}
		if publicPaths[path] {
			c.Next()
			return
		}
		if s.cfg.Get().Login.Password == "" {
			c.Next()
			return
		}
		if token := bearerToken(c); token != "" && s.sessions.valid(token) {
			c.Next()
			return
		}
		failCode(c, http.StatusUnauthorized, "未登录或登录已过期")
		c.Abort()
	}
}

// bearerToken 从 Authorization: Bearer 头取 token。
func bearerToken(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

// handleLogin 账号密码登录，成功返回新 token。
func (s *Server) handleLogin(c *gin.Context) {
	cfg := s.cfg.Get()
	// 密码为空视为未启用鉴权，直接放行
	if cfg.Login.Password == "" {
		ok(c, map[string]interface{}{"login": true, "token": ""})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err.Error())
		return
	}
	match, upgrade := auth.CheckPassword(cfg.Login.Password, req.Password)
	if !match || req.Username != cfg.Login.Username {
		failCode(c, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if upgrade {
		// 遗留 MD5 密码验证通过，自动升级为 bcrypt
		if hash, err := auth.HashPassword(req.Password); err == nil {
			_ = s.cfg.SetConfigRaw([]byte(`{"login":{"password":"` + hash + `"}}`))
		}
	}
	hours := cfg.LoginEffectiveHours
	if hours <= 0 {
		hours = 3
	}
	token, err := s.sessions.create(time.Duration(hours) * time.Hour)
	if err != nil {
		fail(c, err.Error())
		return
	}
	ok(c, map[string]interface{}{"login": true, "token": token})
}

// handleLogout 登出，销毁当前 token。
func (s *Server) handleLogout(c *gin.Context) {
	if token := bearerToken(c); token != "" {
		s.sessions.delete(token)
	}
	ok(c, nil)
}

// handleCheckLogin 探测当前登录态。
func (s *Server) handleCheckLogin(c *gin.Context) {
	ok(c, map[string]interface{}{"login": true})
}

// failCode 以指定 code 写错误结果。
func failCode(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, domain.NewCodeError(code, msg))
}