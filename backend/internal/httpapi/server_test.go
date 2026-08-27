package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

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