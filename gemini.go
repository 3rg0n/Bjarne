package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Gemini API URL template (model is inserted)
const geminiAPIURLTemplate = "https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent"

// Ensure GeminiClient implements LLMProvider
var _ LLMProvider = (*GeminiClient)(nil)

// GeminiClient implements LLMProvider for Google Gemini API
type GeminiClient struct {
	apiKey       string
	defaultModel string
	httpClient   *http.Client
}

// GeminiRequest represents a request to the Gemini API
type GeminiRequest struct {
	Contents         []GeminiContent         `json:"contents"`
	SystemInstruct   *GeminiSystemInstruct   `json:"systemInstruction,omitempty"`
	GenerationConfig *GeminiGenerationConfig `json:"generationConfig,omitempty"`
	ThinkingConfig   *GeminiThinkingConfig   `json:"thinkingConfig,omitempty"`
}

// GeminiContent represents a content block in Gemini format
type GeminiContent struct {
	Role  string       `json:"role"`
	Parts []GeminiPart `json:"parts"`
}

// GeminiPart represents a part of content (text, etc.)
type GeminiPart struct {
	Text string `json:"text"`
}

// GeminiSystemInstruct represents system instruction
type GeminiSystemInstruct struct {
	Parts []GeminiPart `json:"parts"`
}

// GeminiGenerationConfig contains generation parameters
type GeminiGenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

// GeminiThinkingConfig configures thinking/reasoning for Gemini 3 Pro
type GeminiThinkingConfig struct {
	ThinkingBudget int `json:"thinkingBudget,omitempty"` // -1 for dynamic, or specific token count
}

// GeminiResponse represents a response from the Gemini API
type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text           string `json:"text"`
				ThoughtSummary string `json:"thoughtSummary,omitempty"`
			} `json:"parts"`
			Role string `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
		ThoughtsTokenCount   int `json:"thoughtsTokenCount,omitempty"`
	} `json:"usageMetadata"`
}

// NewGeminiProvider creates a GeminiClient as an LLMProvider
func NewGeminiProvider(cfg *ProviderConfig) (LLMProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("Gemini API key required (set BJARNE_API_KEY)")
	}

	defaultModel := cfg.Models.Generate
	if defaultModel == "" {
		defaultModel = GeminiModelMap[ModelSonnet]
	}

	return &GeminiClient{
		apiKey:       cfg.APIKey,
		defaultModel: defaultModel,
		httpClient:   &http.Client{},
	}, nil
}

// Name returns the provider name
func (c *GeminiClient) Name() string {
	return "Google Gemini"
}

// MapModel maps a canonical model name to Gemini model ID
func (c *GeminiClient) MapModel(canonical string) string {
	return MapModelGeneric(ProviderGemini, canonical)
}

// DefaultModel returns the default model
func (c *GeminiClient) DefaultModel() string {
	return c.defaultModel
}

// convertMessagesToGemini converts bjarne Messages to Gemini format
func convertMessagesToGemini(messages []Message) []GeminiContent {
	var result []GeminiContent

	for _, msg := range messages {
		role := msg.Role
		// Gemini uses "user" and "model" roles
		if role == "assistant" {
			role = "model"
		}

		result = append(result, GeminiContent{
			Role: role,
			Parts: []GeminiPart{
				{Text: msg.Content},
			},
		})
	}

	return result
}

// getThinkingConfig returns thinking configuration based on complexity
// Gemini 3 Pro supports "low" (fast) and "high" (deep reasoning) - NO medium
func getThinkingConfig(model string, isComplex bool) *GeminiThinkingConfig {
	// Only gemini-3-pro-preview supports thinking
	if model != "gemini-3-pro-preview" {
		return nil
	}

	// Use dynamic budget (-1) for thinking
	// The actual level (low/high) is determined by the prompt complexity
	// For complex tasks, we want deeper reasoning
	if isComplex {
		return &GeminiThinkingConfig{
			ThinkingBudget: -1, // Dynamic (high reasoning)
		}
	}

	// For simpler tasks, don't enable extended thinking
	return nil
}

// Generate sends a request to the Gemini API
func (c *GeminiClient) Generate(ctx context.Context, model, systemPrompt string, messages []Message, maxTokens int) (*GenerateResult, error) {
	// Map canonical model names to Gemini IDs
	isComplex := model == ModelOpus
	if IsCanonicalModel(model) {
		model = c.MapModel(model)
	}

	url := fmt.Sprintf(geminiAPIURLTemplate, model) + "?key=" + c.apiKey

	req := GeminiRequest{
		Contents: convertMessagesToGemini(messages),
		GenerationConfig: &GeminiGenerationConfig{
			Temperature:     1.0, // Required for reasoning in Gemini 3
			MaxOutputTokens: maxTokens,
		},
		ThinkingConfig: getThinkingConfig(model, isComplex),
	}

	// Add system instruction if provided
	if systemPrompt != "" {
		req.SystemInstruct = &GeminiSystemInstruct{
			Parts: []GeminiPart{{Text: systemPrompt}},
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp GeminiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(apiResp.Candidates) == 0 {
		return nil, fmt.Errorf("model returned no candidates")
	}

	// Extract text from content parts
	var text string
	for _, part := range apiResp.Candidates[0].Content.Parts {
		if part.Text != "" {
			text += part.Text
		}
	}

	if text == "" {
		return nil, fmt.Errorf("model returned empty content (finish_reason: %s)", apiResp.Candidates[0].FinishReason)
	}

	return &GenerateResult{
		Text:         text,
		InputTokens:  apiResp.UsageMetadata.PromptTokenCount,
		OutputTokens: apiResp.UsageMetadata.CandidatesTokenCount,
	}, nil
}

// GenerateStreaming sends a streaming request to the Gemini API
func (c *GeminiClient) GenerateStreaming(ctx context.Context, model, systemPrompt string, messages []Message, maxTokens int, callback StreamCallback) (*GenerateResult, error) {
	// Map canonical model names to Gemini IDs
	isComplex := model == ModelOpus
	if IsCanonicalModel(model) {
		model = c.MapModel(model)
	}

	// Use streamGenerateContent endpoint
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:streamGenerateContent", model) + "?key=" + c.apiKey

	req := GeminiRequest{
		Contents: convertMessagesToGemini(messages),
		GenerationConfig: &GeminiGenerationConfig{
			Temperature:     1.0,
			MaxOutputTokens: maxTokens,
		},
		ThinkingConfig: getThinkingConfig(model, isComplex),
	}

	if systemPrompt != "" {
		req.SystemInstruct = &GeminiSystemInstruct{
			Parts: []GeminiPart{{Text: systemPrompt}},
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Process streaming response (Gemini returns newline-delimited JSON)
	var fullText string
	decoder := json.NewDecoder(resp.Body)

	for {
		var chunk GeminiResponse
		if err := decoder.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			continue
		}

		if len(chunk.Candidates) > 0 {
			for _, part := range chunk.Candidates[0].Content.Parts {
				if part.Text != "" {
					fullText += part.Text
					if callback != nil {
						callback(part.Text)
					}
				}
			}
		}
	}

	return &GenerateResult{
		Text: fullText,
	}, nil
}
