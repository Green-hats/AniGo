package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/greenhats/anigo/internal/domain"
)

// 测试 Parse 的 JSON 解析逻辑（用 completeFn 注入固定回复）。
func TestParseJSON(t *testing.T) {
	d := NewDeepSeek(&domain.Config{AiApiKey: "test", AiModel: "deepseek-chat"})
	ani := &domain.Ani{Title: "间谍过家家"}

	// 1. 纯 JSON 数组
	d.completeFn = func(ctx context.Context, system, user string) (string, error) {
		return `[{"episode":3,"resolution":"1080P","subgroup":"ANi","title":"间谍过家家","isSpecial":false}]`, nil
	}
	out, err := d.Parse(context.Background(), ani, []string{"[ANi] 间谍过家家 03 [1080P][Baha][WEB-DL]"})
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 result, got %d", len(out))
	}
	if out[0].Episode != 3 || out[0].Subgroup != "ANi" || out[0].Title != "间谍过家家" {
		t.Fatalf("bad parse: %+v", out[0])
	}
	if out[0].RawTitle != "[ANi] 间谍过家家 03 [1080P][Baha][WEB-DL]" {
		t.Fatalf("rawTitle 未回填: %+v", out[0])
	}

	// 2. 带 ```json 围栏
	d.completeFn = func(ctx context.Context, system, user string) (string, error) {
		return "```json\n[{\"episode\":6,\"resolution\":\"720P\",\"subgroup\":\"喵萌奶茶屋\",\"title\":\"间谍过家家\",\"isSpecial\":false}]\n```", nil
	}
	out2, err := d.Parse(context.Background(), ani, []string{"[喵萌奶茶屋] 间谍过家家 06 [720P]"})
	if err != nil {
		t.Fatalf("Parse fence err: %v", err)
	}
	if out2[0].Episode != 6 {
		t.Fatalf("bad fence parse: %+v", out2[0])
	}

	// 3. AI 返回长度不足 → 补全 rawTitle
	d.completeFn = func(ctx context.Context, system, user string) (string, error) {
		return `[]`, nil
	}
	out3, _ := d.Parse(context.Background(), ani, []string{"t1", "t2"})
	if len(out3) != 2 || out3[0].RawTitle != "t1" || out3[1].RawTitle != "t2" {
		t.Fatalf("padding fail: %+v", out3)
	}

	// 4. 解析选版信号字段（内封/内嵌、编码、压制源、色深、字幕语言）
	d.completeFn = func(ctx context.Context, system, user string) (string, error) {
		return `[{"rawTitle":"x","episode":4,"resolution":"1080P","subgroup":"北宇治字幕组","title":"与你相恋到生命尽头","isSpecial":false,"subtitleEmbed":"内封","videoCodec":"HEVC","source":"WebRip","colorDepth":"10bit","subtitleLang":"简繁日"}]`, nil
	}
	out4, err := d.Parse(context.Background(), ani, []string{"[北宇治字幕组] 与你相恋到生命尽头 [04][WebRip][HEVC_AAC][简繁日内封]"})
	if err != nil {
		t.Fatalf("Parse signals err: %v", err)
	}
	p := out4[0]
	if p.SubtitleEmbed != "内封" || p.VideoCodec != "HEVC" || p.Source != "WebRip" || p.ColorDepth != "10bit" || p.SubtitleLang != "简繁日" {
		t.Fatalf("bad signals: %+v", p)
	}
}

func TestTrimJSONFence(t *testing.T) {
	cases := map[string]string{
		"```json\n[1]\n```":       "[1]",
		"```\n[1]\n```":           "[1]",
		"[1]":                     "[1]",
		"  [1]  ":                 "[1]",
		"```json\n{\"a\":1}\n```": `{"a":1}`,
	}
	for in, want := range cases {
		if got := trimJSONFence(in); got != want {
			t.Errorf("trimJSONFence(%q) = %q, want %q", in, got, want)
		}
	}
}

// 简中开关开启时，Parse 的 prompt 应包含简体中文字幕与匹配/排除规则；关闭时不含简中要求。
func TestParsePromptIncludesRulesAndSubtitle(t *testing.T) {
	var captured string

	// 开启简中 + 有匹配/排除规则
	d := NewDeepSeek(&domain.Config{AiApiKey: "test", AiModel: "deepseek-chat", AiSubtitleSC: true})
	d.completeFn = func(ctx context.Context, system, user string) (string, error) {
		captured = system
		return `[{"episode":1,"resolution":"1080P","subgroup":"","title":"T","isSpecial":false}]`, nil
	}
	ani := &domain.Ani{Title: "T", Match: []string{"1080p"}, Exclude: []string{"合集"}}
	_, _ = d.Parse(context.Background(), ani, []string{"x"})
	if !strings.Contains(captured, "简体中文字幕") {
		t.Errorf("开启简中开关时 prompt 应包含简中要求, got: %s", captured)
	}
	if !strings.Contains(captured, "1080p") || !strings.Contains(captured, "合集") {
		t.Errorf("prompt 应包含匹配/排除规则, got: %s", captured)
	}

	// 关闭简中 → 不含简中要求
	d2 := NewDeepSeek(&domain.Config{AiApiKey: "test", AiModel: "deepseek-chat", AiSubtitleSC: false})
	d2.completeFn = func(ctx context.Context, system, user string) (string, error) {
		captured = system
		return `[{"episode":1,"resolution":"1080P","subgroup":"","title":"T","isSpecial":false}]`, nil
	}
	_, _ = d2.Parse(context.Background(), ani, []string{"x"})
	if strings.Contains(captured, "简体中文字幕") {
		t.Errorf("关闭简中开关时 prompt 不应包含简中要求, got: %s", captured)
	}
}