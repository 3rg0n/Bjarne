package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

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

// Generate sends a prompt to Claude and returns the generated code (uses default model)
func (b *BedrockClient) Generate(ctx context.Context, systemPrompt string, messages []Message) (string, error) {
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

	// Extract text content from response
	var text string
	for _, content := range response.Content {
		if content.Type == "text" {
			text += content.Text
		}
	}

	return &GenerateResult{
		Text:         text,
		InputTokens:  response.Usage.InputTokens,
		OutputTokens: response.Usage.OutputTokens,
	}, nil
}

// GetDefaultModel returns the configured default model ID
func (b *BedrockClient) GetDefaultModel() string {
	return b.defaultModel
}

// getEnvOrDefault returns the environment variable value or a default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
