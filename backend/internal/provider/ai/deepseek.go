package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/util"
)

// DeepSeek 是 DeepSeek 大模型客户端，使用 OpenAI 兼容的 chat completions 接口。
// 同时兼容其他 OpenAI 风格的服务（OpenAI/通义/智谱等只需改 baseURL/model）。
type DeepSeek struct {
	cfg *domain.Config
	// completeFn 是可替换的底层调用函数（便于测试注入）。
	completeFn func(ctx context.Context, system, user string) (string, error)
}

// NewDeepSeek 创建 DeepSeek 客户端。cfg 用于读取 aiApiKey/aiBaseURL/aiModel。
func NewDeepSeek(cfg *domain.Config) *DeepSeek {
	d := &DeepSeek{cfg: cfg}
	d.completeFn = d.complete
	return d
}

// chatMessage 是 OpenAI 兼容的对话消息。
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest 是 OpenAI 兼容的 chat completions 请求体。
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatChoice 是响应中的一个选择。
type chatChoice struct {
	Message chatMessage `json:"message"`
}

// chatResponse 是 OpenAI 兼容的响应体。
type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

// complete 调用 chat completions 接口，返回助手回复文本。
func (d *DeepSeek) complete(ctx context.Context, system, user string) (string, error) {
	if d.cfg == nil || d.cfg.AiApiKey == "" {
		return "", errors.New("AI 未配置 apiKey")
	}
	base := d.cfg.AiBaseURL
	if base == "" {
		base = "https://api.deepseek.com"
	}
	model := d.cfg.AiModel
	if model == "" {
		model = "deepseek-chat"
	}
	url := base + "/chat/completions"
	body, _ := json.Marshal(chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.cfg.AiApiKey)
	req.Header.Set("User-Agent", util.UserAgent())

	client := util.ClientFor(d.cfg, 60)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("AI http %s: %s", resp.Status, string(b))
	}
	var cr chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", err
	}
	if len(cr.Choices) == 0 {
		return "", errors.New("AI 返回空结果")
	}
	return cr.Choices[0].Message.Content, nil
}

// Ping 测试 AI 服务连通性与密钥有效性，返回模型回复。
func (d *DeepSeek) Ping(ctx context.Context) (string, error) {
	return d.completeFn(ctx, "你是一个连通性测试助手", "请回复：ok")
}

// parseSystemPrompt 组装标题解析提示词。
// 规则部分固定使用内置默认（不允许用户修改），以保证 AI 输出格式稳定。
func (d *DeepSeek) parseSystemPrompt() string {
	rules := domain.DEFAULT_AI_PROMPT()
	return "你是一个动漫BT资源标题解析器。用户会给你一批动漫下载资源标题，请把每个标题解析为结构化信息。\n\n" +
		rules + "\n\n" +
		"只输出 JSON，不要任何其他文字。格式如下（数组，顺序与输入一致）：\n" +
		`[{"rawTitle":"原样返回标题","episode":3,"resolution":"1080P","subgroup":"ANi","title":"间谍过家家","isSpecial":false,"subtitleEmbed":"内嵌","videoCodec":"HEVC","source":"WebRip","colorDepth":"10bit","subtitleLang":"简繁日"}]`
}

// Parse 批量解析标题（TitleParser 接口实现）。
func (d *DeepSeek) Parse(ctx context.Context, titles []string) ([]domain.ParsedTitle, error) {
	if len(titles) == 0 {
		return []domain.ParsedTitle{}, nil
	}
	user, _ := json.Marshal(titles)
	raw, err := d.completeFn(ctx, d.parseSystemPrompt(), string(user))
	if err != nil {
		return nil, err
	}
	raw = trimJSONFence(raw)
	var out []domain.ParsedTitle
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("AI 返回 JSON 解析失败: %w", err)
	}
	// 补全 rawTitle（模型可能省略），确保与输入对齐
	result := make([]domain.ParsedTitle, len(titles))
	for i := range result {
		result[i] = domain.ParsedTitle{RawTitle: titles[i]}
	}
	for i, pt := range out {
		if i >= len(result) {
			break
		}
		pt.RawTitle = titles[i]
		result[i] = pt
	}
	return result, nil
}

// Filter 用 AI 判断标题是否匹配订阅（保留）。
func (d *DeepSeek) Filter(ctx context.Context, ani *domain.Ani, titles []string) ([]bool, error) {
	if len(titles) == 0 {
		return []bool{}, nil
	}
	matchRules := "无（全部接受）"
	if len(ani.Match) > 0 {
		matchRules = joinRules(ani.Match)
	}
	excludeRules := "无"
	if len(ani.Exclude) > 0 {
		excludeRules = joinRules(ani.Exclude)
	}
	subtitleRule := ""
	if d.cfg != nil && d.cfg.AiSubtitleSC {
		subtitleRule = "另外，仅保留包含简体中文字幕的资源（简中字幕或简中双语均视为满足）；纯繁体中文、无中文字幕或仅外挂英文/日文字幕的标题应排除。"
	}
	system := fmt.Sprintf(`你是动漫资源订阅过滤助手。番剧名：%s。匹配规则（标题需满足）：%s。排除规则（标题不得命中）：%s。
%s
对用户给出的每个标题，判断是否应保留（满足匹配规则且不命中排除规则）。只输出 JSON 布尔数组，如 [true,false,true]。不要输出任何其他文字。`,
		ani.Title, matchRules, excludeRules, subtitleRule)
	user, _ := json.Marshal(titles)
	raw, err := d.completeFn(ctx, system, string(user))
	if err != nil {
		return nil, err
	}
	raw = trimJSONFence(raw)
	var flags []bool
	if err := json.Unmarshal([]byte(raw), &flags); err != nil {
		return nil, fmt.Errorf("AI 过滤返回 JSON 解析失败: %w", err)
	}
	result := make([]bool, len(titles))
	for i := range result {
		result[i] = true // 默认保留
	}
	for i, v := range flags {
		if i >= len(result) {
			break
		}
		result[i] = v
	}
	return result, nil
}

// trimJSONFence 去除模型可能包裹的 ```json ... ``` 代码围栏。
func trimJSONFence(s string) string {
	str := strings.TrimSpace(s)
	const fence = "```"
	if strings.HasPrefix(str, fence) {
		if idx := strings.Index(str, "\n"); idx >= 0 {
			str = str[idx+1:]
		}
		if idx := strings.LastIndex(str, fence); idx >= 0 {
			str = str[:idx]
		}
	}
	return strings.TrimSpace(str)
}

func joinRules(rules []string) string {
	out := ""
	for i, r := range rules {
		if i > 0 {
			out += "；"
		}
		out += r
	}
	return out
}

// 确保 DeepSeek 实现端口接口。
var _ domain.TitleParser = (*DeepSeek)(nil)
var _ domain.TitleFilter = (*DeepSeek)(nil)