package notifier

import (
	"testing"

	"github.com/greenhats/anigo/internal/domain"
)

type fakeCfg struct{}

func (f *fakeCfg) Get() *domain.Config { return &domain.Config{} }

func TestReplaceTemplate(t *testing.T) {
	n := New(&fakeCfg{})
	ani := &domain.Ani{Title: "间谍过家家", Season: 1, CurrentEpisodeNumber: 3, BgmUrl: "https://bgm.tv/subject/1"}
	cfg := &domain.NotificationConfig{
		NotificationTemplate: "${emoji} ${title} S${seasonFormat}E${episodeFormat} ${text}",
		Comment:              "备注",
	}
	text := "有新集"
	out := n.ReplaceTemplate(ani, cfg, text, domain.NotifyDownloadStart)
	want := "⬇️ 间谍过家家 S01E03 有新集"
	if out != want {
		t.Errorf("模板渲染 = %q, want %q", out, want)
	}
}

func TestReplaceTemplateNotificationNested(t *testing.T) {
	n := New(&fakeCfg{})
	ani := &domain.Ani{Title: "测试番", CurrentEpisodeNumber: 2}
	cfg := &domain.NotificationConfig{
		NotificationTemplate: "${notification} [${comment}]",
		Comment:              "c",
	}
	out := n.ReplaceTemplate(ani, cfg, "消息", domain.NotifyOmit)
	// ${notification} 展开为全局模板（fakeCfg 未配置 → 默认 ${text}）
	want := "消息 [c]"
	if out != want {
		t.Errorf("嵌套模板 = %q, want %q", out, want)
	}
}

func TestStatusMeta(t *testing.T) {
	cases := []struct {
		status   domain.NotificationStatusEnum
		emoji    string
		action   string
	}{
		{domain.NotifyDownloadStart, "⬇️", "开始下载"},
		{domain.NotifyDownloadEnd, "✅", "下载完成"},
		{domain.NotifyOmit, "⚠️", "缺少集数"},
		{domain.NotifyError, "❌", "错误"},
		{domain.NotifyCompleted, "🏁", "订阅完成"},
		{domain.NotifyProcrastinating, "🐟", "摸鱼"},
	}
	for _, c := range cases {
		emoji, action := StatusMeta(c.status)
		if emoji != c.emoji || action != c.action {
			t.Errorf("StatusMeta(%s) = (%q,%q), want (%q,%q)", c.status, emoji, action, c.emoji, c.action)
		}
	}
}

func TestParseHeaders(t *testing.T) {
	got := parseHeaders("Authorization: Bearer abc\nX-Custom: 1")
	if got["Authorization"] != "Bearer abc" || got["X-Custom"] != "1" {
		t.Errorf("parseHeaders = %v", got)
	}
}

func TestSplitKeys(t *testing.T) {
	got := splitKeys("a, b,, c")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("splitKeys = %v", got)
	}
}