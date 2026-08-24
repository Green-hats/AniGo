package ai

import (
	"context"
	"testing"

	"github.com/greenhats/anigo/internal/domain"
)

// 测试 Parse 的 JSON 解析逻辑（用 completeFn 注入固定回复）。
func TestParseJSON(t *testing.T) {
	d := NewDeepSeek(&domain.Config{AiApiKey: "test", AiModel: "deepseek-chat"})

	// 1. 纯 JSON 数组
	d.completeFn = func(ctx context.Context, system, user string) (string, error) {
		return `[{"episode":3,"resolution":"1080P","subgroup":"ANi","title":"间谍过家家","isSpecial":false}]`, nil
	}
	out, err := d.Parse(context.Background(), []string{"[ANi] 间谍过家家 03 [1080P][Baha][WEB-DL]"})
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
	out2, err := d.Parse(context.Background(), []string{"[喵萌奶茶屋] 间谍过家家 06 [720P]"})
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
	out3, _ := d.Parse(context.Background(), []string{"t1", "t2"})
	if len(out3) != 2 || out3[0].RawTitle != "t1" || out3[1].RawTitle != "t2" {
		t.Fatalf("padding fail: %+v", out3)
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

func TestFilterBoolArray(t *testing.T) {
	d := NewDeepSeek(&domain.Config{AiApiKey: "test", AiModel: "deepseek-chat"})
	d.completeFn = func(ctx context.Context, system, user string) (string, error) {
		return `[true,false,true]`, nil
	}
	ani := &domain.Ani{Title: "间谍过家家"}
	flags, err := d.Filter(context.Background(), ani, []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Filter err: %v", err)
	}
	if len(flags) != 3 || flags[0] != true || flags[1] != false || flags[2] != true {
		t.Fatalf("bad filter: %+v", flags)
	}
}