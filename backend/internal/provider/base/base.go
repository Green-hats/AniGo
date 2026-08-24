package base

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/util"
)

// ConfigProvider 是 provider 读取配置的最小接口，
// 由 service.ConfigService 实现（避免 provider 依赖具体 service）。
type ConfigProvider interface {
	Get() *domain.Config
}

// Cache 是 provider 使用的缓存端口（domain.Cache）。
type Cache interface {
	Get(key string) (string, bool)
	Put(key, val string, ttl time.Duration)
	Contains(key string) bool
	Clear()
}

// Fetcher 是外部 API 的公共 HTTP 客户端封装。
// 内嵌到各 provider：提供带鉴权头、可注入请求函数、明确错误处理的能力。
type Fetcher struct {
	Cfg ConfigProvider
	// 每个 provider 自定义的请求头（如 BGM 的 Authorization）
	Header map[string]string
	// Req 是可替换的底层请求函数（默认指向 request，测试可注入）。
	Req func(ctx context.Context, method, rawURL string, body io.Reader, contentType string) ([]byte, int, error)

	mu        sync.Mutex
	httpClient *http.Client
}

// New 创建 Fetcher。
func New(cfg ConfigProvider) *Fetcher {
	f := &Fetcher{Cfg: cfg, Header: map[string]string{}}
	f.Req = f.request
	return f
}

// client 返回遵循代理设置的 HTTP 客户端（懒初始化）。
func (f *Fetcher) client() *http.Client {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.httpClient == nil {
		f.httpClient = util.ClientFor(f.Cfg.Get(), 20)
	}
	return f.httpClient
}

// request 执行一次 HTTP 请求，返回响应体字节与状态码。
func (f *Fetcher) request(ctx context.Context, method, rawURL string, body io.Reader, contentType string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", util.UserAgent())
	for k, v := range f.Header {
		req.Header.Set(k, v)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := f.client().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return b, resp.StatusCode, nil
}

// Get 执行 GET 并解析 JSON 到 v。非 2xx 返回错误。
func (f *Fetcher) Get(ctx context.Context, rawURL string, v interface{}) error {
	b, code, err := f.Req(ctx, "GET", rawURL, nil, "")
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return &HTTPError{Status: code, Body: string(b)}
	}
	if v == nil {
		return nil
	}
	return json.Unmarshal(b, v)
}

// GetRaw 执行 GET，返回响应体字节（不解析）。
func (f *Fetcher) GetRaw(ctx context.Context, rawURL string) ([]byte, error) {
	b, code, err := f.Req(ctx, "GET", rawURL, nil, "")
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, &HTTPError{Status: code, Body: string(b)}
	}
	return b, nil
}

// HTTPError 是 HTTP 非 2xx 响应错误。
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("http %d: %s", e.Status, e.Body)
}