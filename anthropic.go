package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const anthropicAPIURL = "https://api.anthropic.com/v1/messages"

// Ensure AnthropicClient implements LLMProvider
var _ LLMProvider = (*AnthropicClient)(nil)

// AnthropicClient implements LLMProvider for direct Anthropic API
type AnthropicClient struct {
	apiKey       string
	defaultModel string
	httpClient   *http.Client
}

// AnthropicRequest represents a request to the Anthropic Messages API
type AnthropicRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
	Stream    bool      `json:"stream,omitempty"`
}

// AnthropicResponse represents a response from the Anthropic Messages API
type AnthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// NewAnthropicProvider creates an AnthropicClient as an LLMProvider
func NewAnthropicProvider(cfg *ProviderConfig) (LLMProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("Anthropic API key required (set BJARNE_API_KEY)")
	}

	defaultModel := cfg.Models.Generate
	if defaultModel == "" {
		defaultModel = AnthropicModelMap[ModelSonnet]
	}

	return &AnthropicClient{
		apiKey:       cfg.APIKey,
		defaultModel: defaultModel,
		httpClient:   &http.Client{},
	}, nil
}

// Name returns the provider name
func (c *AnthropicClient) Name() string {
	return "Anthropic"
}

// MapModel maps a canonical model name to Anthropic model ID
func (c *AnthropicClient) MapModel(canonical string) string {
	return MapModelGeneric(ProviderAnthropic, canonical)
}

// DefaultModel returns the default model
func (c *AnthropicClient) DefaultModel() string {
	return c.defaultModel
}

// Generate sends a request to the Anthropic API
func (c *AnthropicClient) Generate(ctx context.Context, model, systemPrompt string, messages []Message, maxTokens int) (*GenerateResult, error) {
	// Map canonical model names to Anthropic IDs
	if IsCanonicalModel(model) {
		model = c.MapModel(model)
	}

	req := AnthropicRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Messages:  messages,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

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

	var apiResp AnthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract text from content blocks
	var text string
	for _, content := range apiResp.Content {
		if content.Type == "text" {
			text += content.Text
		}
	}

	if text == "" {
		return nil, fmt.Errorf("model returned no text content (stop_reason: %s)", apiResp.StopReason)
	}

	return &GenerateResult{
		Text:         text,
		InputTokens:  apiResp.Usage.InputTokens,
		OutputTokens: apiResp.Usage.OutputTokens,
	}, nil
}

// GenerateStreaming sends a streaming request to the Anthropic API
func (c *AnthropicClient) GenerateStreaming(ctx context.Context, model, systemPrompt string, messages []Message, maxTokens int, callback StreamCallback) (*GenerateResult, error) {
	// Map canonical model names to Anthropic IDs
	if IsCanonicalModel(model) {
		model = c.MapModel(model)
	}

	req := AnthropicRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Messages:  messages,
		Stream:    true,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Process SSE stream
	var fullText string
	var outputTokens int

	decoder := json.NewDecoder(resp.Body)
	for {
		// Read line from SSE stream
		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}

		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			// SSE format uses "data: " prefix, this is a simplified handler
			continue
		}

		if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" {
			chunk := event.Delta.Text
			fullText += chunk
			if callback != nil {
				callback(chunk)
			}
		}

		if event.Type == "message_delta" && event.Usage.OutputTokens > 0 {
			outputTokens = event.Usage.OutputTokens
		}

		if event.Type == "message_stop" {
			break
		}
	}

	return &GenerateResult{
		Text:         fullText,
		OutputTokens: outputTokens,
	}, nil
}
