package util

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

func TestMD5Hex(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "d41d8cd98f00b204e9800998ecf8427e"},
		{"abc", "900150983cd24fb0d6963f7d28e17f72"},
		{"admin", "21232f297a57a5a743894a0e4a801fc3"},
	}
	for _, c := range cases {
		if got := MD5Hex(c.in); got != c.want {
			t.Errorf("MD5Hex(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestUserAgent(t *testing.T) {
	got := UserAgent()
	want := "ani-rss-go/" + Version
	if got != want {
		t.Errorf("UserAgent = %q, want %q", got, want)
	}
}

func TestProxyURL(t *testing.T) {
	cases := []struct {
		name string
		cfg  domain.Config
		want string
	}{
		{"空 host 返回 nil", domain.Config{Proxy: true}, ""},
		{"无 scheme 自动补 http", domain.Config{Proxy: true, ProxyHost: "127.0.0.1:8080"}, "http://127.0.0.1:8080"},
		{"带 http scheme 保留", domain.Config{Proxy: true, ProxyHost: "http://p.example:3128"}, "http://p.example:3128"},
		{"带 socks5 scheme 保留", domain.Config{Proxy: true, ProxyHost: "socks5://s.example:1080"}, "socks5://s.example:1080"},
		{"带用户名密码", domain.Config{Proxy: true, ProxyHost: "p.example:3128", ProxyUsername: "u", ProxyPassword: "pwd"}, "http://u:pwd@p.example:3128"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u := proxyURL(&c.cfg)
			if c.want == "" {
				if u != nil {
					t.Errorf("proxyURL = %v, want nil", u)
				}
				return
			}
			if u == nil {
				t.Fatal("proxyURL = nil, want URL")
			}
			if u.String() != c.want {
				t.Errorf("proxyURL = %q, want %q", u.String(), c.want)
			}
		})
	}
}

func TestProxyURLInvalid(t *testing.T) {
	cfg := &domain.Config{Proxy: true, ProxyHost: "http://\x7f"}
	if u := proxyURL(cfg); u != nil {
		t.Errorf("非法 URL 应返回 nil, got %v", u)
	}
}

func TestClientForTimeout(t *testing.T) {
	c := ClientFor(nil, 5)
	if c.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", c.Timeout)
	}
	// 非正超时回退到 20s
	c2 := ClientFor(nil, 0)
	if c2.Timeout != 20*time.Second {
		t.Errorf("Timeout = %v, want 20s", c2.Timeout)
	}
	// nil config 不 panic
	c3 := DefaultClient(nil)
	if c3 == nil || c3.Timeout <= 0 {
		t.Error("DefaultClient(nil) 应返回有效客户端")
	}
}

func TestDefaultClientProxySetting(t *testing.T) {
	cfg := &domain.Config{Proxy: true, ProxyHost: "127.0.0.1:12345"}
	c := DefaultClient(cfg)
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport 类型 = %T", c.Transport)
	}
	if tr.Proxy == nil {
		t.Error("启用代理时 Proxy 不应为 nil")
	}
}

func TestGetBytesNon200(t *testing.T) {
	// 用 httptest 验证非 2xx 返回错误，2xx 返回 body
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	mux.HandleFunc("/err", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	if _, err := GetBytes(nil, ts.URL+"/ok"); err != nil {
		t.Errorf("GET /ok 不应出错: %v", err)
	}
	if _, err := GetBytes(nil, ts.URL+"/err"); err == nil {
		t.Error("GET /err 应返回错误")
	}
}