package base

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/greenhats/anigo/internal/domain"
)

type fakeCfg struct{}

func (f *fakeCfg) Get() *domain.Config { return &domain.Config{} }

func TestFetcherGetParsesJSON(t *testing.T) {
	f := New(&fakeCfg{})
	// 注入假请求函数
	f.Req = func(ctx context.Context, method, rawURL string, body io.Reader, contentType string) ([]byte, int, error) {
		_ = method
		_ = rawURL
		_ = body
		return []byte(`{"id": 123, "name": "测试"}`), 200, nil
	}
	var v struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := f.Get(context.Background(), "http://x", &v); err != nil {
		t.Fatalf("Get err: %v", err)
	}
	if v.ID != 123 || v.Name != "测试" {
		t.Fatalf("解析错误: %+v", v)
	}
}

func TestFetcherGetHTTPError(t *testing.T) {
	f := New(&fakeCfg{})
	f.Req = func(ctx context.Context, method, rawURL string, body io.Reader, contentType string) ([]byte, int, error) {
		return []byte("bad"), 404, nil
	}
	var v map[string]interface{}
	err := f.Get(context.Background(), "http://x", &v)
	if err == nil {
		t.Fatal("404 应返回错误")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("错误应包含状态码: %v", err)
	}
}

func TestFetcherPostForm(t *testing.T) {
	f := New(&fakeCfg{})
	f.Req = func(ctx context.Context, method, rawURL string, body io.Reader, contentType string) ([]byte, int, error) {
		if contentType != "application/x-www-form-urlencoded" {
			t.Fatalf("contentType = %q", contentType)
		}
		return []byte(`{"ok":true}`), 200, nil
	}
	var v struct {
		OK bool `json:"ok"`
	}
	if err := f.PostForm(context.Background(), "http://x", nil, &v); err != nil {
		t.Fatalf("PostForm err: %v", err)
	}
	if !v.OK {
		t.Fatal("PostForm 解析错误")
	}
}