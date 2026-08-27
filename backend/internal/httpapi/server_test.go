package httpapi

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/auth"
	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/log"
	"github.com/greenhats/anigo/internal/service"
	"github.com/greenhats/anigo/internal/store"
)

// noopCloud 提供永不登录的网盘驱动，避免测试依赖真实 115。
type noopCloud struct{}

func (noopCloud) Get(cfg *domain.Config) domain.CloudDriver { return &service.NoopDriver{} }

// newTestServer 组装一个完整但隔离外部依赖的 Server。
func newTestServer(t *testing.T) *Server {
	t.Helper()
	st := store.NewJSONStore(t.TempDir())
	cfg, err := service.NewConfigService(st, store.NewTTLCache())
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}
	// 关闭 AI：默认配置带开发 Key，避免测试触发真实网络请求
	cfg.Get().AiApiKey = ""
	// 关闭鉴权：现有测试不带凭证，密码置空即放行（发布版首次配置同款语义）
	cfg.Get().Login.Password = ""
	cache := store.NewTTLCache()
	logger := log.New(64)
	rss := service.NewRssService(cfg, logger)
	meta := service.NewMetadataService(cfg, cache)
	notify := service.NewNotifyService(cfg, func(string) {})
	ani := service.NewAniService(cfg, rss, meta)
	download := service.NewDownloadService(cfg, rss, noopCloud{}, cache, meta, notify, logger)
	status := service.NewStatusService(cfg, rss, download, cache)
	logs := service.NewLogService(logger)
	return NewServer(cfg, ani, rss, download, meta, notify, logs, status)
}

// doReq 向 Server 发起一次请求并返回响应。
func doReq(t *testing.T, s *Server, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

// decodeResult 解析统一 Result 包装。
func decodeResult(t *testing.T, w *httptest.ResponseRecorder) *domain.Result {
	t.Helper()
	var res domain.Result
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("解析响应失败: %v, body=%s", err, w.Body.String())
	}
	return &res
}

func TestPing(t *testing.T) {
	s := newTestServer(t)
	w := doReq(t, s, http.MethodPost, "/api/ping", nil)
	if res := decodeResult(t, w); res.Code != 200 {
		t.Errorf("ping code = %d, want 200", res.Code)
	}
}

func TestGetConfigBlanksPassword(t *testing.T) {
	s := newTestServer(t)
	w := doReq(t, s, http.MethodPost, "/api/config", nil)
	res := decodeResult(t, w)
	if res.Code != 200 {
		t.Fatalf("code = %d, want 200", res.Code)
	}
	cfg := res.Data.(map[string]interface{})
	login := cfg["login"].(map[string]interface{})
	if login["password"] != "" {
		t.Errorf("响应中的密码应被置空, got %q", login["password"])
	}
}

func TestCustomCssJs(t *testing.T) {
	s := newTestServer(t)
	w := doReq(t, s, http.MethodGet, "/api/custom.css", nil)
	if ct := w.Header().Get("Content-Type"); ct != "text/css; charset=utf-8" {
		t.Errorf("css content-type = %q", ct)
	}
	if body := w.Body.String(); body != "/* empty css */" {
		t.Errorf("css body = %q", body)
	}
	w2 := doReq(t, s, http.MethodGet, "/api/custom.js", nil)
	if body := w2.Body.String(); body != "// empty js" {
		t.Errorf("js body = %q", body)
	}
}

func TestSetConfigAndReadBack(t *testing.T) {
	s := newTestServer(t)
	w := doReq(t, s, http.MethodPost, "/api/setConfig", map[string]interface{}{"rssSleepMinutes": 30})
	if res := decodeResult(t, w); res.Code != 200 {
		t.Fatalf("setConfig code = %d, want 200, msg=%s", res.Code, res.Message)
	}
	w2 := doReq(t, s, http.MethodPost, "/api/config", nil)
	res := decodeResult(t, w2)
	cfg := res.Data.(map[string]interface{})
	if v, _ := cfg["rssSleepMinutes"].(float64); int(v) != 30 {
		t.Errorf("rssSleepMinutes = %v, want 30", cfg["rssSleepMinutes"])
	}
}

func TestClearCache(t *testing.T) {
	s := newTestServer(t)
	w := doReq(t, s, http.MethodPost, "/api/clearCache", nil)
	if res := decodeResult(t, w); res.Code != 200 {
		t.Errorf("clearCache code = %d, want 200", res.Code)
	}
}

func TestLogsAndClearLogs(t *testing.T) {
	s := newTestServer(t)
	w := doReq(t, s, http.MethodPost, "/api/logs", nil)
	res := decodeResult(t, w)
	if res.Code != 200 {
		t.Fatalf("logs code = %d, want 200", res.Code)
	}
	if _, ok := res.Data.([]interface{}); !ok {
		t.Errorf("logs data 应为数组, got %T", res.Data)
	}
	w2 := doReq(t, s, http.MethodPost, "/api/clearLogs", nil)
	if res := decodeResult(t, w2); res.Code != 200 {
		t.Errorf("clearLogs code = %d, want 200", res.Code)
	}
}

func TestStatus(t *testing.T) {
	s := newTestServer(t)
	w := doReq(t, s, http.MethodPost, "/api/status", nil)
	res := decodeResult(t, w)
	if res.Code != 200 {
		t.Fatalf("status code = %d, want 200", res.Code)
	}
	data := res.Data.(map[string]interface{})
	if _, ok := data["cloud"]; !ok {
		t.Errorf("status 缺少 cloud 字段")
	}
	// AI 未配置时不应发起网络探测
	if ai, ok := data["ai"].(map[string]interface{}); ok {
		if ai["configured"] == true {
			t.Errorf("AI 未配置却返回 configured=true")
		}
	}
}

func TestListAniEmpty(t *testing.T) {
	s := newTestServer(t)
	w := doReq(t, s, http.MethodPost, "/api/listAni", nil)
	res := decodeResult(t, w)
	data := res.Data.(map[string]interface{})
	if total, _ := data["total"].(float64); int(total) != 0 {
		t.Errorf("空列表 total = %v, want 0", data["total"])
	}
}

func TestAniAddDeleteLifecycle(t *testing.T) {
	s := newTestServer(t)
	// 添加
	w := doReq(t, s, http.MethodPost, "/api/addAni", map[string]interface{}{
		"id": "ani-1", "title": "测试番剧", "season": 1, "url": "http://example.invalid/rss.xml",
	})
	if res := decodeResult(t, w); res.Code != 200 {
		t.Fatalf("addAni code = %d, msg=%s", res.Code, res.Message)
	}
	// 列表可见
	w2 := doReq(t, s, http.MethodPost, "/api/listAni", nil)
	data := decodeResult(t, w2).Data.(map[string]interface{})
	if total, _ := data["total"].(float64); int(total) != 1 {
		t.Errorf("addAni 后 total = %v, want 1", data["total"])
	}
	// 删除
	w3 := doReq(t, s, http.MethodPost, "/api/deleteAni", []string{"ani-1"})
	if res := decodeResult(t, w3); res.Code != 200 {
		t.Fatalf("deleteAni code = %d, msg=%s", res.Code, res.Message)
	}
	w4 := doReq(t, s, http.MethodPost, "/api/listAni", nil)
	data4 := decodeResult(t, w4).Data.(map[string]interface{})
	if total, _ := data4["total"].(float64); int(total) != 0 {
		t.Errorf("deleteAni 后 total = %v, want 0", data4["total"])
	}
}

func TestRefreshAniNotFound(t *testing.T) {
	s := newTestServer(t)
	w := doReq(t, s, http.MethodPost, "/api/refreshAni", map[string]interface{}{"id": "不存在"})
	res := decodeResult(t, w)
	if res.Code != 500 {
		t.Errorf("未知订阅应返回 500, got %d", res.Code)
	}
}

func TestAIPingNotConfigured(t *testing.T) {
	s := newTestServer(t)
	w := doReq(t, s, http.MethodPost, "/api/aiPing", nil)
	res := decodeResult(t, w)
	if res.Code != 500 {
		t.Errorf("未配置 AI 应返回 500, got %d", res.Code)
	}
}

// doReqAuth 与 doReq 相同，但附带 Bearer token。
func doReqAuth(t *testing.T, s *Server, method, path string, body interface{}, token string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

// loginAndGetToken 以指定密码登录并返回 token。
func loginAndGetToken(t *testing.T, s *Server, password string) string {
	t.Helper()
	w := doReq(t, s, http.MethodPost, "/api/login", map[string]interface{}{
		"username": "admin",
		"password": password,
	})
	res := decodeResult(t, w)
	if res.Code != 200 {
		t.Fatalf("login code = %d, msg=%s", res.Code, res.Message)
	}
	token := res.Data.(map[string]interface{})["token"].(string)
	if token == "" {
		t.Fatal("login 未返回 token")
	}
	return token
}

// bcryptHash 生成真实 bcrypt 哈希（测试用）。
func bcryptHash(t *testing.T, plain string) string {
	t.Helper()
	h, err := auth.HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	return h
}

func TestLoginWrongPassword(t *testing.T) {
	s := newTestServer(t)
	s.cfg.Get().Login.Password = bcryptHash(t, "secret")
	w := doReq(t, s, http.MethodPost, "/api/login", map[string]interface{}{
		"username": "admin",
		"password": "wrong",
	})
	res := decodeResult(t, w)
	if res.Code != 401 {
		t.Errorf("错误密码 login code = %d, want 401", res.Code)
	}
}

func TestLoginWrongUsername(t *testing.T) {
	s := newTestServer(t)
	s.cfg.Get().Login.Password = bcryptHash(t, "secret")
	w := doReq(t, s, http.MethodPost, "/api/login", map[string]interface{}{
		"username": "hacker",
		"password": "secret",
	})
	res := decodeResult(t, w)
	if res.Code != 401 {
		t.Errorf("错误用户名 login code = %d, want 401", res.Code)
	}
}

func TestAuthRequired(t *testing.T) {
	s := newTestServer(t)
	s.cfg.Get().Login.Password = bcryptHash(t, "secret")
	// 无 token 访问受保护端点应 401
	w := doReq(t, s, http.MethodPost, "/api/config", nil)
	if res := decodeResult(t, w); res.Code != 401 {
		t.Errorf("无 token 请求 code = %d, want 401", res.Code)
	}
	// 白名单端点仍放行
	w2 := doReq(t, s, http.MethodGet, "/api/ping", nil)
	if res := decodeResult(t, w2); res.Code != 200 {
		t.Errorf("ping code = %d, want 200", res.Code)
	}
	// login 本身放行
	w3 := doReq(t, s, http.MethodPost, "/api/login", map[string]interface{}{"username": "admin", "password": "secret"})
	if res := decodeResult(t, w3); res.Code != 200 {
		t.Errorf("login code = %d, want 200", res.Code)
	}
}

func TestLoginAndAccess(t *testing.T) {
	s := newTestServer(t)
	s.cfg.Get().Login.Password = bcryptHash(t, "secret")
	token := loginAndGetToken(t, s, "secret")
	// 带 token 访问受保护端点成功
	w := doReqAuth(t, s, http.MethodPost, "/api/config", nil, token)
	if res := decodeResult(t, w); res.Code != 200 {
		t.Errorf("带 token 请求 code = %d, want 200", res.Code)
	}
	// checkLogin 应返回已登录
	w2 := doReqAuth(t, s, http.MethodPost, "/api/checkLogin", nil, token)
	res := decodeResult(t, w2)
	if res.Code != 200 {
		t.Errorf("checkLogin code = %d, want 200", res.Code)
	}
	if login := res.Data.(map[string]interface{})["login"]; login != true {
		t.Errorf("checkLogin login = %v, want true", login)
	}
}

func TestLogout(t *testing.T) {
	s := newTestServer(t)
	s.cfg.Get().Login.Password = bcryptHash(t, "secret")
	token := loginAndGetToken(t, s, "secret")
	w := doReqAuth(t, s, http.MethodPost, "/api/logout", nil, token)
	if res := decodeResult(t, w); res.Code != 200 {
		t.Fatalf("logout code = %d", res.Code)
	}
	// 登出后原 token 失效
	w2 := doReqAuth(t, s, http.MethodPost, "/api/config", nil, token)
	if res := decodeResult(t, w2); res.Code != 401 {
		t.Errorf("登出后请求 code = %d, want 401", res.Code)
	}
}

func TestLegacyMD5PasswordUpgrade(t *testing.T) {
	s := newTestServer(t)
	// 模拟老配置：MD5 密码
	legacy := fmt.Sprintf("%x", md5.Sum([]byte("admin")))
	s.cfg.Get().Login.Password = legacy
	token := loginAndGetToken(t, s, "admin")
	if token == "" {
		t.Fatal("MD5 密码登录应成功")
	}
	// 登录成功后密码应升级为 bcrypt
	if s.cfg.Get().Login.Password == legacy {
		t.Error("MD5 密码应自动升级为 bcrypt")
	}
}

func TestSetConfigHashesPassword(t *testing.T) {
	s := newTestServer(t)
	// 设置明文密码
	w := doReq(t, s, http.MethodPost, "/api/setConfig", map[string]interface{}{
		"login": map[string]interface{}{"username": "admin", "password": "mysecret"},
	})
	if res := decodeResult(t, w); res.Code != 200 {
		t.Fatalf("setConfig code = %d, msg=%s", res.Code, res.Message)
	}
	// 存储值应为 bcrypt 而非明文
	stored := s.cfg.Get().Login.Password
	if stored == "mysecret" {
		t.Error("密码不应以明文存储")
	}
	// 用明文能登录（登录后配置已启用鉴权，需带 token）
	token := loginAndGetToken(t, s, "mysecret")
	// 读配置不泄露密码
	w3 := doReqAuth(t, s, http.MethodPost, "/api/config", nil, token)
	res3 := decodeResult(t, w3)
	login := res3.Data.(map[string]interface{})["login"].(map[string]interface{})
	if login["password"] != "" {
		t.Errorf("读取配置密码应为空, got %q", login["password"])
	}
}

func TestEmptyPasswordBypass(t *testing.T) {
	s := newTestServer(t)
	// newTestServer 已把密码置空：无凭证也应放行（首次配置语义）
	w := doReq(t, s, http.MethodPost, "/api/config", nil)
	if res := decodeResult(t, w); res.Code != 200 {
		t.Errorf("空密码时请求 code = %d, want 200", res.Code)
	}
}

func TestSessionPurgeExpiredOnCreate(t *testing.T) {
	ss := newSessionStore()
	// 两个已过期会话
	if _, err := ss.create(-2 * time.Hour); err != nil {
		t.Fatalf("create expired: %v", err)
	}
	if _, err := ss.create(-1 * time.Hour); err != nil {
		t.Fatalf("create expired: %v", err)
	}
	// 新会话触发惰性清理
	tok, err := ss.create(time.Hour)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := len(ss.sessions); got != 1 {
		t.Errorf("过期会话应在下次 create 时被清理, got %d sessions, want 1", got)
	}
	if !ss.valid(tok) {
		t.Error("未过期会话应保持有效")
	}
}

func TestExportConfig(t *testing.T) {
	s := newTestServer(t)
	w := doReq(t, s, http.MethodGet, "/api/exportConfig", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("exportConfig status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("content-type = %q, want application/zip", ct)
	}
}