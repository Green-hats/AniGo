package ai

import (
	"strings"

	"github.com/greenhats/anigo/internal/domain"
)

// Provider 是 AI 提供方工厂，根据配置返回对应的解析/过滤器。
// 目前支持 OpenAI 兼容接口（DeepSeek 等）。返回值可为 nil（未配置/不支持）。
type Provider struct{}

// New 根据配置构建 AI 客户端。provider 名不区分大小写，
// 兼容 openai/deepseek/通义(qwen)/智谱(zhipu) 等 OpenAI 风格服务。
func New(cfg *domain.Config) *DeepSeek {
	if cfg == nil || cfg.AiApiKey == "" {
		return nil
	}
	switch strings.ToLower(cfg.AiProvider) {
	case "", "deepseek", "openai", "qwen", "tongyi", "zhipu", "glm", "custom":
		return NewDeepSeek(cfg)
	default:
		return NewDeepSeek(cfg)
	}
}