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
	client  *bedrockruntime.Client
	modelID string
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

// NewBedrockClient creates a new Bedrock client with configuration from environment
func NewBedrockClient(ctx context.Context) (*BedrockClient, error) {
	// Load AWS config from environment/credentials
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(getEnvOrDefault("AWS_REGION", "us-east-1")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Get model ID from environment or use default
	// Note: Use global. prefix for cross-region inference profiles
	modelID := getEnvOrDefault("BJARNE_MODEL", "global.anthropic.claude-sonnet-4-20250514-v1:0")

	client := bedrockruntime.NewFromConfig(cfg)

	return &BedrockClient{
		client:  client,
		modelID: modelID,
	}, nil
}

// Generate sends a prompt to Claude and returns the generated code
func (b *BedrockClient) Generate(ctx context.Context, systemPrompt string, messages []Message) (string, error) {
	request := ClaudeRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        4096,
		Messages:         messages,
		System:           systemPrompt,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	output, err := b.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(b.modelID),
		Body:        requestBody,
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return "", fmt.Errorf("bedrock invoke failed: %w", err)
	}

	var response ClaudeResponse
	if err := json.Unmarshal(output.Body, &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Extract text content from response
	var result string
	for _, content := range response.Content {
		if content.Type == "text" {
			result += content.Text
		}
	}

	return result, nil
}

// GetModelID returns the configured model ID
func (b *BedrockClient) GetModelID() string {
	return b.modelID
}

// getEnvOrDefault returns the environment variable value or a default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
