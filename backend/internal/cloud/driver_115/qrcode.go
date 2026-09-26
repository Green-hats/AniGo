package driver115

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

// 115 扫码登录端点（与 OpenList 官方脚本一致的流程）。
const (
	apiQRToken  = "https://qrcodeapi.115.com/api/1.0/web/1.0/token/"
	apiQRStatus = "https://qrcodeapi.115.com/get/status/"
	apiQRLogin  = "https://passportapi.115.com/app/1.0/web/1.0/login/qrcode/"
)

// flexBool 兼容 115 返回的 state 字段（可能是 1/0 或 true/false）。
type flexBool bool

func (f *flexBool) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	*f = s == "1" || s == "true"
	return nil
}

// QRTokenResp 是 token 接口响应。
type QRTokenResp struct {
	State flexBool `json:"state"`
	Data  struct {
		UID   string `json:"uid"`
		Time  int64  `json:"time"`
		Sign  string `json:"sign"`
		// QRCode 是要编码进二维码的 URL（https://yun.115.com/scan/dg-...）。
		QRCode string `json:"qrcode"`
	} `json:"data"`
}

// QRStatusResp 是轮询接口响应。
// status: 0=等待扫码 1=已扫码待确认 2=已确认 -1=过期 -2=取消
type QRStatusResp struct {
	State flexBool `json:"state"`
	Data  struct {
		Status int `json:"status"`
		Msg    string `json:"msg"`
	} `json:"data"`
}

// QRLoginResp 是登录接口响应，data.cookie 即凭据。
// 115 的认证 Cookie 已不止 UID/CID/SEID（2025 年新增必需的 KID），
// 因此用 map 收全部字段，按固定顺序拼接。
type QRLoginResp struct {
	State flexBool          `json:"state"`
	Error string            `json:"error"`
	Data  struct {
		UserID int64             `json:"user_id"`
		Cookie map[string]string `json:"cookie"`
	} `json:"data"`
}

// QRSession 描述一次扫码会话。
type QRSession struct {
	UID string `json:"uid"`
	// Time/Sign 来自 token 接口，轮询时原样透传。
	Time int64  `json:"time"`
	Sign string `json:"sign"`
	// QRText 是要渲染成二维码的 URL。
	QRText string `json:"qrText"`
}

// qrClient 是扫码流程专用 HTTP 客户端：不走配置代理。
// 经代理访问 get/status 会被服务器 hold ~30s（代理出口 IP 特征触发），
// 而 115 国内直连稳定，故强制直连。
var qrClient = &http.Client{
	// get/status 是长轮询：未扫码时服务器 hold ~30s 才返回，超时须大于它
	Timeout: 35 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
	},
}

// rawGet 无 Cookie 的 GET（扫码流程本身不需要登录态）。
func (p *Pan115) rawGet(ctx context.Context, cfg *domain.Config, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua115)
	resp, err := qrClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// rawPostForm 无 Cookie 的表单 POST。
func (p *Pan115) rawPostForm(ctx context.Context, cfg *domain.Config, rawURL string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua115)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := qrClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// QRCreate 发起一次扫码登录会话。
func (p *Pan115) QRCreate(ctx context.Context, cfg *domain.Config) (*QRSession, error) {
	b, err := p.rawGet(ctx, cfg, apiQRToken)
	if err != nil {
		return nil, fmt.Errorf("请求二维码 token 失败: %w", err)
	}
	var tr QRTokenResp
	if err := json.Unmarshal(b, &tr); err != nil {
		return nil, fmt.Errorf("获取二维码 token 失败: %v (内容: %s)", err, preview(b))
	}
	if !tr.State || tr.Data.QRCode == "" {
		return nil, fmt.Errorf("获取二维码 token 失败 (内容: %s)", preview(b))
	}
	return &QRSession{
		UID:    tr.Data.UID,
		Time:   tr.Data.Time,
		Sign:   tr.Data.Sign,
		QRText: tr.Data.QRCode,
	}, nil
}

// QRStatusText 轮询扫码状态，返回 waiting/scanned/confirmed/expired/canceled。
func (p *Pan115) QRStatusText(ctx context.Context, cfg *domain.Config, s *QRSession) (string, error) {
	q := url.Values{}
	q.Set("uid", s.UID)
	q.Set("time", fmt.Sprintf("%d", s.Time))
	q.Set("sign", s.Sign)
	b, err := p.rawGet(ctx, cfg, apiQRStatus+"?"+q.Encode())
	if err != nil {
		return "error", err
	}
	var sr QRStatusResp
	if err := json.Unmarshal(b, &sr); err != nil {
		return "error", fmt.Errorf("查询扫码状态失败: %v (内容: %s)", err, preview(b))
	}
	// 长轮询结束（未扫码）时返回 state=1 且 data 为空对象，status 字段缺失
	if sr.Data.Status == 0 && (strings.Contains(preview(b), `"data":{}`) || strings.Contains(preview(b), `"data": {}`)) {
		return "waiting", nil
	}
	switch sr.Data.Status {
	case 0:
		return "waiting", nil
	case 1:
		return "scanned", nil
	case 2:
		return "confirmed", nil
	case -1:
		return "expired", nil
	case -2:
		return "canceled", nil
	default:
		return "error", fmt.Errorf("扫码状态异常: %d (%s)", sr.Data.Status, sr.Data.Msg)
	}
}

// QRFetchCookie 在状态 confirmed 后换取登录 Cookie（UID=..;CID=..;SEID=..）。
func (p *Pan115) QRFetchCookie(ctx context.Context, cfg *domain.Config, s *QRSession) (string, error) {
	form := url.Values{}
	form.Set("app", "web")
	form.Set("account", s.UID)
	b, err := p.rawPostForm(ctx, cfg, apiQRLogin, form)
	if err != nil {
		return "", fmt.Errorf("请求扫码登录失败: %w", err)
	}
	var lr QRLoginResp
	if err := json.Unmarshal(b, &lr); err != nil {
		return "", fmt.Errorf("扫码登录换取 Cookie 失败: %v (内容: %s)", err, preview(b))
	}
	if !lr.State || len(lr.Data.Cookie) == 0 {
		return "", fmt.Errorf("扫码登录换取 Cookie 失败: %s (内容: %s)", lr.Error, preview(b))
	}
	// 按稳定顺序拼接全部字段；SEID 缺失视为异常
	if lr.Data.Cookie["SEID"] == "" {
		return "", fmt.Errorf("扫码登录响应缺少 SEID (内容: %s)", preview(b))
	}
	order := []string{"UID", "CID", "SEID", "KID"}
	seen := map[string]bool{}
	parts := make([]string, 0, len(lr.Data.Cookie))
	for _, k := range order {
		if v := lr.Data.Cookie[k]; v != "" {
			parts = append(parts, k+"="+v)
			seen[k] = true
		}
	}
	// 兜底：把未知的其他字段也带上（保序遍历 map）
	var extra []string
	for k, v := range lr.Data.Cookie {
		if !seen[k] && v != "" {
			extra = append(extra, k+"="+v)
		}
	}
	sort.Strings(extra)
	parts = append(parts, extra...)
	return strings.Join(parts, ";"), nil
}

// preview 截取响应前 150 字节用于错误诊断。
func preview(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 150 {
		s = s[:150]
	}
	return s
}
