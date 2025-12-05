package main

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestOpenAIProvider tests the OpenAI provider with a real API call
// Requires OPENAI_KEY environment variable
func TestOpenAIProvider(t *testing.T) {
	apiKey := os.Getenv("OPENAI_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_KEY not set, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := &ProviderConfig{
		Provider: ProviderOpenAI,
		APIKey:   apiKey,
		Models:   ModelSettings{Generate: "gpt-4o-mini"},
	}

	provider, err := NewProvider(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create OpenAI provider: %v", err)
	}

	t.Logf("Provider: %s", provider.Name())

	messages := []Message{
		{Role: "user", Content: "Say 'Hello from bjarne!' - respond with exactly those 3 words."},
	}

	result, err := provider.Generate(ctx, "gpt-4o-mini", "You are a helpful assistant. Follow instructions exactly.", messages, 50)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	t.Logf("Response: %s", result.Text)
	t.Logf("Tokens: %d in, %d out", result.InputTokens, result.OutputTokens)

	if result.Text == "" {
		t.Error("Expected non-empty response")
	}
}

// TestGeminiProvider tests the Gemini provider with a real API call
// Requires GEMINI_KEY environment variable
func TestGeminiProvider(t *testing.T) {
	apiKey := os.Getenv("GEMINI_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_KEY not set, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := &ProviderConfig{
		Provider: ProviderGemini,
		APIKey:   apiKey,
		Models:   ModelSettings{Generate: "gemini-2.0-flash"},
	}

	provider, err := NewProvider(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create Gemini provider: %v", err)
	}

	t.Logf("Provider: %s", provider.Name())

	messages := []Message{
		{Role: "user", Content: "Say 'Hello from bjarne!' - respond with exactly those 3 words."},
	}

	result, err := provider.Generate(ctx, "gemini-2.0-flash", "You are a helpful assistant. Follow instructions exactly.", messages, 50)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	t.Logf("Response: %s", result.Text)
	t.Logf("Tokens: %d in, %d out", result.InputTokens, result.OutputTokens)

	if result.Text == "" {
		t.Error("Expected non-empty response")
	}
}
