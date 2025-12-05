package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// Ensure BedrockClient implements LLMProvider
var _ LLMProvider = (*BedrockClient)(nil)

// BedrockClient wraps the AWS Bedrock Runtime client
type BedrockClient struct {
	client       *bedrockruntime.Client
	defaultModel string
}

// Message represents a conversation message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ClaudeRequest represents the request body for Claude models
type ClaudeRequest struct {
	AnthropicVersion string    `json:"anthropic_version"`
	MaxTokens        int       `json:"max_tokens"`
	Messages         []Message `json:"messages"`
	System           string    `json:"system,omitempty"`
}

// ClaudeResponse represents the response from Claude models
type ClaudeResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// GenerateResult contains the response text and token usage
type GenerateResult struct {
	Text         string
	InputTokens  int
	OutputTokens int
}

// StreamCallback is called for each chunk of streamed text
type StreamCallback func(chunk string)

// StreamEvent represents a streaming event from Claude
type StreamEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index,omitempty"`
	Delta struct {
		Type string `json:"type,omitempty"`
		Text string `json:"text,omitempty"`
	} `json:"delta,omitempty"`
	Usage struct {
		OutputTokens int `json:"output_tokens,omitempty"`
	} `json:"usage,omitempty"`
}

// NewBedrockClient creates a new Bedrock client with configuration from environment
func NewBedrockClient(ctx context.Context, defaultModel string) (*BedrockClient, error) {
	// Load AWS config from environment/credentials
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(getEnvOrDefault("AWS_REGION", "us-east-1")),
	)
	if err != nil {
		return nil, ErrAWSConfig(err)
	}

	client := bedrockruntime.NewFromConfig(cfg)

	return &BedrockClient{
		client:       client,
		defaultModel: defaultModel,
	}, nil
}

// GenerateSimple sends a prompt to Claude and returns the generated code (uses default model)
// This is a convenience method that wraps GenerateWithModel
func (b *BedrockClient) GenerateSimple(ctx context.Context, systemPrompt string, messages []Message) (string, error) {
	result, err := b.GenerateWithModel(ctx, b.defaultModel, systemPrompt, messages, 4096)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// GenerateWithTokens sends a prompt and returns response with token usage (uses default model)
func (b *BedrockClient) GenerateWithTokens(ctx context.Context, systemPrompt string, messages []Message, maxTokens int) (*GenerateResult, error) {
	return b.GenerateWithModel(ctx, b.defaultModel, systemPrompt, messages, maxTokens)
}

// GenerateWithModel sends a prompt to a specific model and returns response with token usage
func (b *BedrockClient) GenerateWithModel(ctx context.Context, modelID, systemPrompt string, messages []Message, maxTokens int) (*GenerateResult, error) {
	request := ClaudeRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        maxTokens,
		Messages:         messages,
		System:           systemPrompt,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	output, err := b.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(modelID),
		Body:        requestBody,
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return nil, ErrBedrockInvoke(err)
	}

	var response ClaudeResponse
	if err := json.Unmarshal(output.Body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Check for empty content array
	if len(response.Content) == 0 {
		return nil, fmt.Errorf("model returned empty content (stop_reason: %s)", response.StopReason)
	}

	// Extract text content from response
	var text string
	for _, content := range response.Content {
		if content.Type == "text" {
			text += content.Text
		}
	}

	// Check for empty text after extraction
	if text == "" {
		return nil, fmt.Errorf("model returned no text content (stop_reason: %s, content_types: %d)", response.StopReason, len(response.Content))
	}

	return &GenerateResult{
		Text:         text,
		InputTokens:  response.Usage.InputTokens,
		OutputTokens: response.Usage.OutputTokens,
	}, nil
}

// GenerateStreaming sends a prompt and streams the response, calling callback for each chunk
func (b *BedrockClient) GenerateStreaming(ctx context.Context, modelID, systemPrompt string, messages []Message, maxTokens int, callback StreamCallback) (*GenerateResult, error) {
	request := ClaudeRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        maxTokens,
		Messages:         messages,
		System:           systemPrompt,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	output, err := b.client.InvokeModelWithResponseStream(ctx, &bedrockruntime.InvokeModelWithResponseStreamInput{
		ModelId:     aws.String(modelID),
		Body:        requestBody,
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return nil, ErrBedrockInvoke(err)
	}

	// Process streaming events
	var fullText string
	var outputTokens int

	stream := output.GetStream()
	defer func() { _ = stream.Close() }()

	for event := range stream.Events() {
		switch v := event.(type) {
		case *types.ResponseStreamMemberChunk:
			// Parse the chunk payload
			var streamEvent StreamEvent
			if err := json.Unmarshal(v.Value.Bytes, &streamEvent); err != nil {
				continue // Skip malformed events
			}

			// Handle text delta
			if streamEvent.Type == "content_block_delta" && streamEvent.Delta.Type == "text_delta" {
				chunk := streamEvent.Delta.Text
				fullText += chunk
				if callback != nil {
					callback(chunk)
				}
			}

			// Capture final usage
			if streamEvent.Type == "message_delta" && streamEvent.Usage.OutputTokens > 0 {
				outputTokens = streamEvent.Usage.OutputTokens
			}
		}
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("stream error: %w", err)
	}

	return &GenerateResult{
		Text:         fullText,
		OutputTokens: outputTokens,
	}, nil
}

// GetDefaultModel returns the configured default model ID
func (b *BedrockClient) GetDefaultModel() string {
	return b.defaultModel
}

// Name returns the provider name (implements LLMProvider)
func (b *BedrockClient) Name() string {
	return "AWS Bedrock"
}

// MapModel maps a canonical model name to Bedrock model ID (implements LLMProvider)
func (b *BedrockClient) MapModel(canonical string) string {
	return MapModelGeneric(ProviderBedrock, canonical)
}

// DefaultModel returns the default model (implements LLMProvider)
func (b *BedrockClient) DefaultModel() string {
	return b.defaultModel
}

// Generate implements LLMProvider interface
func (b *BedrockClient) Generate(ctx context.Context, model, systemPrompt string, messages []Message, maxTokens int) (*GenerateResult, error) {
	return b.GenerateWithModel(ctx, model, systemPrompt, messages, maxTokens)
}

// NewBedrockProvider creates a BedrockClient as an LLMProvider
func NewBedrockProvider(ctx context.Context, cfg *ProviderConfig) (LLMProvider, error) {
	region := cfg.Region
	if region == "" {
		region = getEnvOrDefault("AWS_REGION", "us-east-1")
	}

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
	)
	if err != nil {
		return nil, ErrAWSConfig(err)
	}

	client := bedrockruntime.NewFromConfig(awsCfg)

	// Use configured generate model or default to Sonnet
	defaultModel := cfg.Models.Generate
	if defaultModel == "" {
		defaultModel = BedrockModelMap[ModelSonnet]
	}

	return &BedrockClient{
		client:       client,
		defaultModel: defaultModel,
	}, nil
}

// getEnvOrDefault returns the environment variable value or a default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
