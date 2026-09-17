// PikPak protocol adapted from github.com/52funny/pikpakcli (MIT).
// See LICENSE.pikpakcli for the upstream copyright and license.
package driverpikpak

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/util"
)

const (
	UserAgent    = "ANDROID-com.pikcloud.pikpak/1.21.0"
	clientID     = "YNxT9w7GMdWvEOKa"
	clientSecret = "dbw2OtmVEeuUvIptb1Coyg"
	filesPath    = "/drive/v1/files"
)

// PikPak serializes session changes and path creation with a cancellable gate.
// Tokens stay in memory and never appear in the application config or logs.
type PikPak struct {
	gate                                      chan struct{}
	stateMu                                   sync.RWMutex
	status                                    domain.LoginStatus
	client                                    *http.Client
	userBase, driveBase                       string
	fingerprint                               string
	access, refresh, captcha, subject, device string
	expiry                                    time.Time
}

func New() domain.CloudDriver { return newPikPak() }
func newPikPak() *PikPak {
	return &PikPak{gate: make(chan struct{}, 1), userBase: "https://user.mypikpak.com", driveBase: "https://api-drive.mypikpak.com"}
}
func (p *PikPak) Name() string { return "pikpak" }
func (p *PikPak) GetLoginStatus() domain.LoginStatus {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	return p.status
}
func (p *PikPak) setStatus(s domain.LoginStatus) { p.stateMu.Lock(); p.status = s; p.stateMu.Unlock() }
func (p *PikPak) lock(ctx context.Context, cfg *domain.Config) error {
	select {
	case p.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		p.unlock()
		return err
	}
	if cfg == nil {
		p.unlock()
		return errors.New("PikPak 配置为空")
	}
	data, _ := json.Marshal([]any{cfg.PikpakEmail, cfg.PikpakPassword, cfg.Proxy, cfg.ProxyHost, cfg.ProxyUsername, cfg.ProxyPassword})
	key := fmt.Sprintf("%x", sha256.Sum256(data))
	if p.fingerprint != key {
		if p.fingerprint != "" && p.client != nil {
			p.client.CloseIdleConnections()
			p.client = nil
		}
		p.fingerprint = key
		p.access, p.refresh, p.captcha, p.subject = "", "", "", ""
		p.expiry = time.Time{}
		p.device = fmt.Sprintf("%x", md5.Sum([]byte(strings.TrimSpace(cfg.PikpakEmail))))
		p.setStatus(domain.LoginStatus{Configured: cfg.PikpakEmail != "" && cfg.PikpakPassword != ""})
	}
	if p.client == nil {
		p.client = util.ClientFor(cfg, 30)
	}
	return nil
}
func (p *PikPak) unlock() { <-p.gate }

type apiError struct {
	code, status int
	message      string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("PikPak 请求失败 (HTTP %d, code %d): %s", e.status, e.code, e.message)
}

// raw performs exactly one request. Mutation retries are only allowed after an
// explicit authentication/captcha rejection, never after ambiguous I/O errors.
func (p *PikPak) raw(ctx context.Context, method, endpoint string, body any, out any) error {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("X-Device-Id", p.device)
	req.Header.Set("X-Peer-Id", p.device)
	req.Header.Set("X-Captcha-Token", p.captcha)
	req.Header.Set("X-Client-Version-Code", "10083")
	req.Header.Set("X-User-Region", "1")
	req.Header.Set("X-Alt-Capability", "3")
	req.Header.Set("Country", "CN")
	req.Header.Set("Product_flavor_name", "cha")
	if p.access != "" {
		req.Header.Set("Authorization", "Bearer "+p.access)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 8<<20 {
		return errors.New("PikPak 响应超过大小限制")
	}
	var envelope struct {
		Code        int    `json:"error_code"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if len(bytes.TrimSpace(data)) != 0 {
		if err := json.Unmarshal(data, &envelope); err != nil {
			return fmt.Errorf("PikPak 响应格式无效 (HTTP %d)", resp.StatusCode)
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || envelope.Code != 0 || envelope.Error != "" {
		msg := envelope.Error
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		// Only return the error identifier; remote descriptions can contain secrets.
		return &apiError{envelope.Code, resp.StatusCode, msg}
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("PikPak 响应格式无效: %w", err)
		}
	}
	return nil
}

type tokenResponse struct {
	Access  string `json:"access_token"`
	Refresh string `json:"refresh_token"`
	Subject string `json:"sub"`
	Expires int64  `json:"expires_in"`
}

func (p *PikPak) applyToken(t tokenResponse) error {
	if t.Access == "" || t.Expires <= 0 {
		return errors.New("PikPak 未返回有效登录凭据")
	}
	p.access = t.Access
	if t.Refresh != "" {
		p.refresh = t.Refresh
	}
	if t.Subject != "" {
		p.subject = t.Subject
	}
	p.expiry = time.Now().Add(time.Duration(t.Expires)*time.Second - 30*time.Second)
	return nil
}
func (p *PikPak) ensureLogin(ctx context.Context, cfg *domain.Config) error {
	if strings.TrimSpace(cfg.PikpakEmail) == "" || cfg.PikpakPassword == "" {
		return errors.New("请配置 PikPak 账号和密码")
	}
	if p.access != "" && time.Now().Before(p.expiry) {
		return nil
	}
	if p.refresh != "" {
		var t tokenResponse
		err := p.raw(ctx, "POST", p.userBase+"/v1/auth/token", map[string]string{"client_id": clientID, "client_secret": clientSecret, "grant_type": "refresh_token", "refresh_token": p.refresh}, &t)
		if err == nil {
			return p.applyToken(t)
		}
		var remote *apiError
		if !errors.As(err, &remote) {
			return err
		}
	}
	p.access, p.refresh, p.captcha = "", "", ""
	if err := p.initCaptcha(ctx, "POST:https://user.mypikpak.com/v1/auth/signin", map[string]string{"username": strings.TrimSpace(cfg.PikpakEmail)}); err != nil {
		return err
	}
	var t tokenResponse
	err := p.raw(ctx, "POST", p.userBase+"/v1/auth/signin", map[string]string{"client_id": clientID, "client_secret": clientSecret, "grant_type": "password", "username": strings.TrimSpace(cfg.PikpakEmail), "password": cfg.PikpakPassword, "captcha_token": p.captcha}, &t)
	if err != nil {
		return err
	}
	return p.applyToken(t)
}
func (p *PikPak) initCaptcha(ctx context.Context, action string, meta map[string]string) error {
	var result struct {
		Token string `json:"captcha_token"`
		URL   string `json:"url"`
	}
	err := p.raw(ctx, "POST", p.userBase+"/v1/shield/captcha/init", map[string]any{"client_id": clientID, "device_id": p.device, "action": action, "captcha_token": p.captcha, "meta": meta}, &result)
	if err != nil {
		return err
	}
	if result.Token == "" || result.URL != "" {
		return errors.New("PikPak 要求人工验证，请先在官方客户端完成验证后重试")
	}
	p.captcha = result.Token
	return nil
}

// Captcha signing constants are from pikpakcli/internal/api/captcha_token.go.
var captchaSalts = []string{"", "E32cSkYXC2bciKJGxRsE8ZgwmH/YwkvpD6/O9guSOa2irCwciH4xPHaH", "QtqgfMgHP2TFl", "zOKgHT56L7nIzFzDpUGhpWFrgP53m3G6ML", "S", "THxpsktzfFXizUv7DK1y/N7NZ1WhayViluBEvAJJ8bA1Wr6", "y9PXH3xGUhG/zQI8CaapRw2LhldCaFM9CRlKpZXJvj+pifu", "+RaaG7T8FRTI4cP019N5y9ofLyHE9ySFUr", "6Pf1l8UTeuzYldGtb/d"}

func (p *PikPak) authCaptcha(ctx context.Context, action string) error {
	ts := fmt.Sprint(time.Now().UnixMilli())
	sign := clientID + "1.21.0" + "com.pikcloud.pikpak" + p.device + ts
	for _, salt := range captchaSalts {
		sign = fmt.Sprintf("%x", md5.Sum([]byte(sign+salt)))
	}
	return p.initCaptcha(ctx, action, map[string]string{"captcha_sign": "1." + sign, "user_id": p.subject, "package_name": "com.pikcloud.pikpak", "client_version": "1.21.0", "timestamp": ts})
}
func (p *PikPak) request(ctx context.Context, cfg *domain.Config, method, path string, body, out any) error {
	if err := p.ensureLogin(ctx, cfg); err != nil {
		return err
	}
	authRetried, captchaRetried := false, false
	for {
		err := p.raw(ctx, method, p.driveBase+path, body, out)
		var remote *apiError
		if !errors.As(err, &remote) {
			return err
		}
		if (remote.status == 401 || remote.code == 16) && !authRetried {
			authRetried = true
			p.expiry = time.Time{}
			if err := p.ensureLogin(ctx, cfg); err != nil {
				return err
			}
			continue
		}
		if remote.code == 9 && !captchaRetried {
			captchaRetried = true
			actionPath, _, _ := strings.Cut(path, "?")
			if strings.HasPrefix(actionPath, filesPath+"/") {
				actionPath = filesPath
			}
			if err := p.authCaptcha(ctx, method+":"+actionPath); err != nil {
				return err
			}
			continue
		}
		return err
	}
}
func (p *PikPak) Login(ctx context.Context, test bool, cfg *domain.Config) (bool, error) {
	if err := p.lock(ctx, cfg); err != nil {
		return false, err
	}
	defer p.unlock()
	var result struct {
		Files []json.RawMessage `json:"files"`
	}
	err := p.request(ctx, cfg, "GET", filesPath+"?limit=1&parent_id=", nil, &result)
	if err == nil && result.Files == nil {
		err = errors.New("PikPak 文件列表响应无效")
	}
	p.setStatus(domain.LoginStatus{Configured: cfg.PikpakEmail != "" && cfg.PikpakPassword != "", OK: err == nil, Message: errorText(err)})
	return err == nil, err
}
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
