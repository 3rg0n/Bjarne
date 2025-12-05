package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const openaiAPIURL = "https://api.openai.com/v1/chat/completions"

// Ensure OpenAIClient implements LLMProvider
var _ LLMProvider = (*OpenAIClient)(nil)

// OpenAIClient implements LLMProvider for OpenAI API
type OpenAIClient struct {
	apiKey       string
	defaultModel string
	httpClient   *http.Client
}

// OpenAIRequest represents a request to the OpenAI Chat Completions API
type OpenAIRequest struct {
	Model               string          `json:"model"`
	Messages            []OpenAIMessage `json:"messages"`
	MaxTokens           int             `json:"max_tokens,omitempty"`            // For older models
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"` // For GPT-5.1+, o1, o3
	Temperature         float64         `json:"temperature,omitempty"`
	Stream              bool            `json:"stream,omitempty"`
	ReasoningEffort     string          `json:"reasoning_effort,omitempty"` // For GPT-5.1: "medium", "high", "xhigh"
}

// OpenAIMessage represents a message in the OpenAI format
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIResponse represents a response from the OpenAI Chat Completions API
type OpenAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// NewOpenAIProvider creates an OpenAIClient as an LLMProvider
func NewOpenAIProvider(cfg *ProviderConfig) (LLMProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("OpenAI API key required (set BJARNE_API_KEY)")
	}

	defaultModel := cfg.Models.Generate
	if defaultModel == "" {
		defaultModel = OpenAIModelMap[ModelSonnet]
	}

	return &OpenAIClient{
		apiKey:       cfg.APIKey,
		defaultModel: defaultModel,
		httpClient:   &http.Client{},
	}, nil
}

// Name returns the provider name
func (c *OpenAIClient) Name() string {
	return "OpenAI"
}

// MapModel maps a canonical model name to OpenAI model ID
func (c *OpenAIClient) MapModel(canonical string) string {
	return MapModelGeneric(ProviderOpenAI, canonical)
}

// DefaultModel returns the default model
func (c *OpenAIClient) DefaultModel() string {
	return c.defaultModel
}

// convertMessages converts bjarne Messages to OpenAI format
func convertMessagesToOpenAI(systemPrompt string, messages []Message) []OpenAIMessage {
	var result []OpenAIMessage

	// Add system message first
	if systemPrompt != "" {
		result = append(result, OpenAIMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	// Convert user/assistant messages
	for _, msg := range messages {
		result = append(result, OpenAIMessage(msg))
	}

	return result
}

// getReasoningEffort returns the reasoning effort level based on model
func getReasoningEffort(model string) string {
	// GPT-5.1 and o-series models support reasoning effort levels
	if strings.HasPrefix(model, "gpt-5") || strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3") {
		return "high" // Default to high for complex code generation
	}
	return "" // Standard models don't use reasoning effort
}

// usesMaxCompletionTokens returns true if the model uses max_completion_tokens instead of max_tokens
func usesMaxCompletionTokens(model string) bool {
	// GPT-5+, o1, o3 models use max_completion_tokens
	return strings.HasPrefix(model, "gpt-5") || strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3")
}

// Generate sends a request to the OpenAI API
func (c *OpenAIClient) Generate(ctx context.Context, model, systemPrompt string, messages []Message, maxTokens int) (*GenerateResult, error) {
	// Map canonical model names to OpenAI IDs
	if IsCanonicalModel(model) {
		model = c.MapModel(model)
	}

	req := OpenAIRequest{
		Model:           model,
		Messages:        convertMessagesToOpenAI(systemPrompt, messages),
		ReasoningEffort: getReasoningEffort(model),
	}

	// Use appropriate token limit parameter based on model
	if usesMaxCompletionTokens(model) {
		req.MaxCompletionTokens = maxTokens
	} else {
		req.MaxTokens = maxTokens
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", openaiAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

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

	var apiResp OpenAIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("model returned no choices")
	}

	text := apiResp.Choices[0].Message.Content
	if text == "" {
		return nil, fmt.Errorf("model returned empty content (finish_reason: %s)", apiResp.Choices[0].FinishReason)
	}

	return &GenerateResult{
		Text:         text,
		InputTokens:  apiResp.Usage.PromptTokens,
		OutputTokens: apiResp.Usage.CompletionTokens,
	}, nil
}

// GenerateStreaming sends a streaming request to the OpenAI API
func (c *OpenAIClient) GenerateStreaming(ctx context.Context, model, systemPrompt string, messages []Message, maxTokens int, callback StreamCallback) (*GenerateResult, error) {
	// Map canonical model names to OpenAI IDs
	if IsCanonicalModel(model) {
		model = c.MapModel(model)
	}

	req := OpenAIRequest{
		Model:           model,
		Messages:        convertMessagesToOpenAI(systemPrompt, messages),
		Stream:          true,
		ReasoningEffort: getReasoningEffort(model),
	}

	// Use appropriate token limit parameter based on model
	if usesMaxCompletionTokens(model) {
		req.MaxCompletionTokens = maxTokens
	} else {
		req.MaxTokens = maxTokens
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", openaiAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

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
	decoder := json.NewDecoder(resp.Body)

	for {
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}

		if err := decoder.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			continue
		}

		if len(chunk.Choices) > 0 {
			content := chunk.Choices[0].Delta.Content
			if content != "" {
				fullText += content
				if callback != nil {
					callback(content)
				}
			}
			if chunk.Choices[0].FinishReason != "" {
				break
			}
		}
	}

	return &GenerateResult{
		Text: fullText,
		// Token counts not available in streaming responses
	}, nil
}
