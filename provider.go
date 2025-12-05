package main

import (
	"context"
	"fmt"
	"strings"
)

// ProviderType represents the LLM provider
type ProviderType string

const (
	ProviderBedrock   ProviderType = "bedrock"
	ProviderAnthropic ProviderType = "anthropic"
	ProviderOpenAI    ProviderType = "openai"
	ProviderGemini    ProviderType = "gemini"
)

// LLMProvider is the abstract interface for LLM providers
type LLMProvider interface {
	// Generate sends a prompt to the LLM and returns the response
	Generate(ctx context.Context, model, systemPrompt string, messages []Message, maxTokens int) (*GenerateResult, error)

	// GenerateStreaming sends a prompt and streams the response
	GenerateStreaming(ctx context.Context, model, systemPrompt string, messages []Message, maxTokens int, callback StreamCallback) (*GenerateResult, error)

	// Name returns the provider name for display
	Name() string

	// MapModel maps a canonical model name (haiku/sonnet/opus) to provider-specific ID
	MapModel(canonical string) string

	// DefaultModel returns the provider's default model
	DefaultModel() string
}

// ProviderConfig holds configuration for initializing providers
type ProviderConfig struct {
	Provider ProviderType
	APIKey   string // For non-Bedrock providers
	Region   string // For Bedrock
	Models   ModelSettings
}

// NewProvider creates an LLM provider based on configuration
func NewProvider(ctx context.Context, cfg *ProviderConfig) (LLMProvider, error) {
	switch cfg.Provider {
	case ProviderBedrock:
		return NewBedrockProvider(ctx, cfg)
	case ProviderAnthropic:
		return NewAnthropicProvider(cfg)
	case ProviderOpenAI:
		return NewOpenAIProvider(cfg)
	case ProviderGemini:
		return NewGeminiProvider(cfg)
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
	}
}

// ParseProviderType converts a string to ProviderType
func ParseProviderType(s string) ProviderType {
	switch strings.ToLower(s) {
	case "bedrock", "aws":
		return ProviderBedrock
	case "anthropic", "claude":
		return ProviderAnthropic
	case "openai", "gpt":
		return ProviderOpenAI
	case "gemini", "google":
		return ProviderGemini
	default:
		return ProviderBedrock // Default to Bedrock
	}
}

// CanonicalModels are the abstract model tiers used throughout bjarne
const (
	ModelHaiku  = "haiku"
	ModelSonnet = "sonnet"
	ModelOpus   = "opus"
)

// BedrockModelMap maps canonical names to Bedrock model IDs
var BedrockModelMap = map[string]string{
	ModelHaiku:  "global.anthropic.claude-haiku-4-5-20251001-v1:0",
	ModelSonnet: "global.anthropic.claude-sonnet-4-5-20250929-v1:0",
	ModelOpus:   "global.anthropic.claude-opus-4-5-20251101-v1:0",
}

// AnthropicModelMap maps canonical names to Anthropic API model IDs
var AnthropicModelMap = map[string]string{
	ModelHaiku:  "claude-3-5-haiku-latest",
	ModelSonnet: "claude-sonnet-4-5-20250929",
	ModelOpus:   "claude-opus-4-5-20251101",
}

// OpenAIModelMap maps canonical names to OpenAI model IDs
var OpenAIModelMap = map[string]string{
	ModelHaiku:  "gpt-5-mini-2025-08-07", // Fast, cost-effective
	ModelSonnet: "gpt-5.1-2025-11-13",    // Balanced performance
	ModelOpus:   "gpt-5.1-codex-max",     // Most capable, agentic coding
}

// GeminiModelMap maps canonical names to Gemini model IDs
// Using currently available models
var GeminiModelMap = map[string]string{
	ModelHaiku:  "gemini-2.0-flash-lite", // Fast, cost-effective
	ModelSonnet: "gemini-2.0-flash",      // Balanced performance
	ModelOpus:   "gemini-2.0-pro",        // Most capable
}

// MapModelGeneric maps a canonical model name using the appropriate provider map
func MapModelGeneric(provider ProviderType, canonical string) string {
	var modelMap map[string]string
	switch provider {
	case ProviderBedrock:
		modelMap = BedrockModelMap
	case ProviderAnthropic:
		modelMap = AnthropicModelMap
	case ProviderOpenAI:
		modelMap = OpenAIModelMap
	case ProviderGemini:
		modelMap = GeminiModelMap
	default:
		modelMap = BedrockModelMap
	}

	if mapped, ok := modelMap[canonical]; ok {
		return mapped
	}
	// If not a canonical name, return as-is (might be a full model ID)
	return canonical
}

// IsCanonicalModel checks if a model name is a canonical name
func IsCanonicalModel(model string) bool {
	switch model {
	case ModelHaiku, ModelSonnet, ModelOpus:
		return true
	default:
		return false
	}
}
