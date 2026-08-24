package util

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

// Version 是应用版本（构建时覆盖）。
var Version = "0.1.0"

// DefaultClient 返回遵循配置代理设置的 HTTP 客户端。
func DefaultClient(cfg *domain.Config) *http.Client {
	return newClient(cfg, 20)
}

// ClientFor 返回指定超时（秒）的 HTTP 客户端，遵循代理设置。
func ClientFor(cfg *domain.Config, timeoutSec int) *http.Client {
	return newClient(cfg, timeoutSec)
}

func newClient(cfg *domain.Config, timeoutSec int) *http.Client {
	if cfg == nil {
		cfg = &domain.Config{}
	}
	if timeoutSec <= 0 {
		timeoutSec = 20
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if cfg.Proxy {
		if p := proxyURL(cfg); p != nil {
			transport.Proxy = http.ProxyURL(p)
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   time.Duration(timeoutSec) * time.Second,
	}
}

func proxyURL(cfg *domain.Config) *url.URL {
	if cfg.ProxyHost == "" {
		return nil
	}
	scheme := "http"
	if strings.HasPrefix(cfg.ProxyHost, "http://") || strings.HasPrefix(cfg.ProxyHost, "https://") || strings.HasPrefix(cfg.ProxyHost, "socks5://") {
		scheme = ""
	}
	raw := cfg.ProxyHost
	if scheme != "" {
		raw = scheme + "://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	if cfg.ProxyUsername != "" {
		u.User = url.UserPassword(cfg.ProxyUsername, cfg.ProxyPassword)
	}
	return u
}

// UserAgent 返回标准 UA 字符串。
func UserAgent() string {
	return "ani-rss-go/" + Version
}

// GetBytes 获取 URL 并返回响应体字节。
func GetBytes(cfg *domain.Config, rawURL string) ([]byte, error) {
	resp, err := Get(cfg, rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("http status " + resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// Get 以配置 UA 获取 URL，返回响应。
func Get(cfg *domain.Config, rawURL string) (*http.Response, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent())
	return DefaultClient(cfg).Do(req)
}

// NewRequest 创建带标准 UA 的请求。
func NewRequest(cfg *domain.Config, method, rawURL string) (*http.Request, error) {
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent())
	return req, nil
}

// MD5Hex 计算字符串的 md5 摘要十六进制。
func MD5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}