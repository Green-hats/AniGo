package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"strconv"
	"strings"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/provider/base"
)

// Notifier 是通知渠道实现。使用 base.Fetcher 发 HTTP，支持可注入请求函数（测试友好）。
type Notifier struct {
	fetcher *base.Fetcher
}

// New 创建通知器（持有公共 HTTP fetcher）。
func New(cfg base.ConfigProvider) *Notifier {
	return &Notifier{fetcher: base.New(cfg)}
}

// Type 返回通知类型（由各实现决定；此处通过嵌入覆盖）。
func (n *Notifier) Type() domain.NotificationTypeEnum { return "" }

// ReplaceTemplate 填充通知模板占位符。
// 支持 ${text} ${title} ${season} ${episode} ${emoji} ${action} ${comment} 等。
func (n *Notifier) ReplaceTemplate(ani *domain.Ani, cfg *domain.NotificationConfig, text string, status domain.NotificationStatusEnum) string {
	tmpl := cfg.NotificationTemplate
	if tmpl == "" {
		tmpl = "${notification}"
	}
	tmpl = n.replaceBase(tmpl, ani, cfg, text, status)
	if strings.Contains(tmpl, "${notification}") {
		// ${notification} 展开为全局通知模板（或默认 ${text}）
		t := ""
		if g := n.fetcher.Cfg.Get(); g != nil {
			t = g.NotificationTemplate
		}
		if t == "" {
			t = "${text}"
		}
		t = n.replaceBase(t, ani, cfg, text, status)
		tmpl = strings.ReplaceAll(tmpl, "${notification}", t)
	}
	return strings.TrimSpace(tmpl)
}

func (n *Notifier) replaceBase(tmpl string, ani *domain.Ani, cfg *domain.NotificationConfig, text string, status domain.NotificationStatusEnum) string {
	tmpl = strings.ReplaceAll(tmpl, "${text}", text)
	tmpl = strings.ReplaceAll(tmpl, "${comment}", cfg.Comment)
	if ani != nil {
		tmpl = strings.ReplaceAll(tmpl, "${title}", ani.Title)
		tmpl = strings.ReplaceAll(tmpl, "${season}", strconv.Itoa(ani.Season))
		tmpl = strings.ReplaceAll(tmpl, "${seasonFormat}", fmt.Sprintf("%02d", ani.Season))
		tmpl = strings.ReplaceAll(tmpl, "${episode}", strconv.Itoa(ani.CurrentEpisodeNumber))
		tmpl = strings.ReplaceAll(tmpl, "${episodeFormat}", fmt.Sprintf("%02d", ani.CurrentEpisodeNumber))
		tmpl = strings.ReplaceAll(tmpl, "${score}", strconv.FormatFloat(ani.Score, 'f', -1, 64))
		tmpl = strings.ReplaceAll(tmpl, "${subgroup}", ani.Subgroup)
		tmpl = strings.ReplaceAll(tmpl, "${currentEpisodeNumber}", strconv.Itoa(ani.CurrentEpisodeNumber))
		tmpl = strings.ReplaceAll(tmpl, "${totalEpisodeNumber}", strconv.Itoa(ani.TotalEpisodeNumber))
		if d := ani.ReleaseDate.Time(); !d.IsZero() {
			tmpl = strings.ReplaceAll(tmpl, "${year}", strconv.Itoa(d.Year()))
			tmpl = strings.ReplaceAll(tmpl, "${month}", strconv.Itoa(int(d.Month())))
			tmpl = strings.ReplaceAll(tmpl, "${date}", strconv.Itoa(d.Day()))
		}
		tmpl = strings.ReplaceAll(tmpl, "${jpTitle}", ani.JpTitle)
		tmpl = strings.ReplaceAll(tmpl, "${bgmUrl}", ani.BgmUrl)
		tmpl = strings.ReplaceAll(tmpl, "${url}", ani.URL)
	}
	emoji, action := StatusMeta(status)
	tmpl = strings.ReplaceAll(tmpl, "${emoji}", emoji)
	tmpl = strings.ReplaceAll(tmpl, "${action}", action)
	return tmpl
}

// StatusMeta 返回状态对应的 emoji 与动作名。
func StatusMeta(status domain.NotificationStatusEnum) (string, string) {
	switch status {
	case domain.NotifyDownloadStart:
		return "⬇️", "开始下载"
	case domain.NotifyDownloadEnd:
		return "✅", "下载完成"
	case domain.NotifyOmit:
		return "⚠️", "缺少集数"
	case domain.NotifyError:
		return "❌", "错误"
	case domain.NotifyCompleted:
		return "🏁", "订阅完成"
	case domain.NotifyProcrastinating:
		return "🐟", "摸鱼"
	}
	return "", ""
}

// sendHTTP 发送一次 HTTP 请求（表单或 JSON）。
func (n *Notifier) sendHTTP(ctx context.Context, method, rawURL string, headers map[string]string, form url.Values, jsonBody interface{}) error {
	var rdr io.Reader
	contentType := ""
	if form != nil {
		rdr = strings.NewReader(form.Encode())
		contentType = "application/x-www-form-urlencoded"
	} else if jsonBody != nil {
		b, err := json.Marshal(jsonBody)
		if err != nil {
			return err
		}
		rdr = strings.NewReader(string(b))
		contentType = "application/json"
	}
	body, code, err := n.fetcher.Req(ctx, method, rawURL, rdr, contentType)
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("http %d: %s", code, string(body))
	}
	return nil
}

// 单个通知器实现

// Telegram 通过 Bot API 发送。
type Telegram struct {
	*Notifier
}

// Type 返回 Telegram。
func (t *Telegram) Type() domain.NotificationTypeEnum { return domain.NotifyTelegram }

// Send 发送 sendMessage 或 sendPhoto。
func (t *Telegram) Send(ctx context.Context, cfg *domain.NotificationConfig, n *domain.Notification) error {
	if cfg.TelegramBotToken == "" || cfg.TelegramChatId == "" {
		return fmt.Errorf("telegram 未配置完成")
	}
	tmpl := t.ReplaceTemplate(n.Ani, cfg, n.Text, n.Status)
	host := cfg.TelegramApiHost
	if host == "" {
		host = "https://api.telegram.org"
	}
	host = strings.TrimSuffix(host, "/")
	form := url.Values{}
	form.Set("chat_id", cfg.TelegramChatId)
	if cfg.TelegramTopicId > 0 {
		form.Set("message_thread_id", strconv.Itoa(cfg.TelegramTopicId))
	}
	if cfg.TelegramFormat == "markdown" {
		form.Set("parse_mode", "MarkdownV2")
	} else if cfg.TelegramFormat == "html" {
		form.Set("parse_mode", "HTML")
	}
	if cfg.TelegramImage && n.Ani != nil && n.Ani.Image != "" {
		u := fmt.Sprintf("%s/bot%s/sendPhoto", host, cfg.TelegramBotToken)
		form.Set("caption", tmpl)
		form.Set("photo", n.Ani.Image)
		return t.sendHTTP(ctx, "POST", u, nil, form, nil)
	}
	u := fmt.Sprintf("%s/bot%s/sendMessage", host, cfg.TelegramBotToken)
	form.Set("text", tmpl)
	return t.sendHTTP(ctx, "POST", u, nil, form, nil)
}

// Bark 发送 iOS 推送。
type Bark struct {
	*Notifier
}

// Type 返回 Bark。
func (b *Bark) Type() domain.NotificationTypeEnum { return domain.NotifyBark }

// Send 发送 Bark 推送。
func (b *Bark) Send(ctx context.Context, cfg *domain.NotificationConfig, n *domain.Notification) error {
	server := cfg.BarkServerUrl
	if server == "" {
		server = "https://api.day.app"
	}
	title := ""
	if n.Ani != nil {
		title = n.Ani.Title
	}
	if title == "" {
		_, title = StatusMeta(n.Status)
	}
	for _, key := range splitKeys(cfg.BarkDeviceKeys) {
		u := fmt.Sprintf("%s/%s", strings.TrimSuffix(server, "/"), key)
		form := url.Values{}
		form.Set("title", title)
		form.Set("body", b.ReplaceTemplate(n.Ani, cfg, n.Text, n.Status))
		if cfg.BarkGroup != "" {
			form.Set("group", cfg.BarkGroup)
		}
		if cfg.BarkLevel != "" {
			form.Set("level", cfg.BarkLevel)
		}
		if cfg.BarkVolume > 0 {
			form.Set("volume", strconv.Itoa(cfg.BarkVolume))
		}
		if err := b.sendHTTP(ctx, "POST", u, nil, form, nil); err != nil {
			return err
		}
	}
	return nil
}

// ServerChan 发送 ServerChan 通知。
type ServerChan struct {
	*Notifier
}

// Type 返回 ServerChan。
func (s *ServerChan) Type() domain.NotificationTypeEnum { return domain.NotifyServerChan }

// Send 发送 ServerChan。
func (s *ServerChan) Send(ctx context.Context, cfg *domain.NotificationConfig, n *domain.Notification) error {
	if cfg.ServerChanSendKey == "" {
		return fmt.Errorf("serverChan 未配置完成")
	}
	title := ""
	if n.Ani != nil {
		title = n.Ani.Title
	}
	if title == "" {
		_, title = StatusMeta(n.Status)
	}
	u := ""
	if cfg.ServerChanType == domain.ServerChanType3 {
		u = cfg.ServerChan3ApiUrl
		if u == "" {
			u = "https://sctapi.ftqq.com"
		}
		u = strings.TrimSuffix(u, "/") + "/" + cfg.ServerChanSendKey + ".send"
	} else {
		u = "https://sctapi.ftqq.com/" + cfg.ServerChanSendKey + ".send"
	}
	form := url.Values{}
	form.Set("title", title)
	form.Set("desp", s.ReplaceTemplate(n.Ani, cfg, n.Text, n.Status))
	return s.sendHTTP(ctx, "POST", u, nil, form, nil)
}

// WebHook 发送通用 WebHook。
type WebHook struct {
	*Notifier
}

// Type 返回 WebHook。
func (w *WebHook) Type() domain.NotificationTypeEnum { return domain.NotifyWebHook }

// Send 发送 WebHook。
func (w *WebHook) Send(ctx context.Context, cfg *domain.NotificationConfig, n *domain.Notification) error {
	if cfg.WebHookUrl == "" {
		return fmt.Errorf("webhook 未配置完成")
	}
	method := strings.ToUpper(cfg.WebHookMethod)
	if method == "" {
		method = "POST"
	}
	body := w.ReplaceTemplate(n.Ani, cfg, n.Text, n.Status)
	var headers map[string]string
	if cfg.WebHookHeader != "" {
		headers = parseHeaders(cfg.WebHookHeader)
	}
	var payload interface{} = body
	if strings.HasPrefix(strings.TrimSpace(body), "{") {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(body), &m); err == nil {
			payload = m
		}
	}
	return w.sendHTTP(ctx, method, cfg.WebHookUrl, headers, nil, payload)
}

// Shell 执行 shell 命令。
type Shell struct {
	*Notifier
}

// Type 返回 Shell。
func (s *Shell) Type() domain.NotificationTypeEnum { return domain.NotifyShell }

// Send 执行 shell 命令。
func (s *Shell) Send(ctx context.Context, cfg *domain.NotificationConfig, n *domain.Notification) error {
	if cfg.Shell == "" {
		return fmt.Errorf("shell 未配置完成")
	}
	cmd := exec.CommandContext(ctx, "bash", "-c", cfg.Shell)
	cmd.Env = append(cmd.Environ(),
		"ANI_RSS_TEXT="+n.Text,
		"ANI_RSS_TITLE="+aniTitle(n.Ani),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("shell: %v %s", err, string(out))
	}
	return nil
}

// System 记录到应用日志。
type System struct {
	*Notifier
	// LogFn 输出日志（由 service 注入）。
	LogFn func(msg string)
}

// Type 返回 System。
func (s *System) Type() domain.NotificationTypeEnum { return domain.NotifySystem }

// Send 记录日志。
func (s *System) Send(ctx context.Context, cfg *domain.NotificationConfig, n *domain.Notification) error {
	msg := s.ReplaceTemplate(n.Ani, cfg, n.Text, n.Status)
	if s.LogFn != nil {
		s.LogFn(msg)
	}
	return nil
}

// 辅助函数

func splitKeys(s string) []string {
	var out []string
	for _, k := range strings.Split(s, ",") {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out
}

func parseHeaders(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			out[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return out
}

func aniTitle(ani *domain.Ani) string {
	if ani == nil {
		return ""
	}
	return ani.Title
}