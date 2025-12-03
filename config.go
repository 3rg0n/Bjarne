package main

import (
	"os"
	"strconv"
)

// Config holds runtime configuration
type Config struct {
	// Iteration limits
	MaxIterations int // Maximum validation retry attempts (default: 3)

	// Token budget
	MaxTokens          int // Maximum tokens per response (default: 4096)
	MaxTotalTokens     int // Maximum total tokens per session (default: 100000, 0 = unlimited)
	WarnTokenThreshold int // Warn when approaching limit (default: 80% of max)

	// Model configuration
	ModelID string // Claude model ID

	// Container configuration
	ValidatorImage string // Container image for validation
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		MaxIterations:      3,
		MaxTokens:          8192,   // Max tokens per response
		MaxTotalTokens:     150000, // Conservative budget within 200k context window
		WarnTokenThreshold: 120000, // Warn at 80% of budget
		ModelID:            "global.anthropic.claude-sonnet-4-20250514-v1:0",
		ValidatorImage:     "ghcr.io/ecopelan/bjarne-validator:latest",
	}
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	cfg := DefaultConfig()

	// Iteration limits
	if val := os.Getenv("BJARNE_MAX_ITERATIONS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			cfg.MaxIterations = n
		}
	}

	// Token budget
	if val := os.Getenv("BJARNE_MAX_TOKENS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			cfg.MaxTokens = n
		}
	}

	if val := os.Getenv("BJARNE_MAX_TOTAL_TOKENS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n >= 0 {
			cfg.MaxTotalTokens = n // 0 = unlimited
		}
	}

	// Model configuration
	if val := os.Getenv("BJARNE_MODEL"); val != "" {
		cfg.ModelID = val
	}

	// Container configuration
	if val := os.Getenv("BJARNE_VALIDATOR_IMAGE"); val != "" {
		cfg.ValidatorImage = val
	}

	// Calculate warning threshold (80% of max)
	if cfg.MaxTotalTokens > 0 {
		cfg.WarnTokenThreshold = cfg.MaxTotalTokens * 80 / 100
	}

	return cfg
}

// TokenTracker tracks token usage across the session
type TokenTracker struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	MaxTokens    int
	WarnAt       int
	warned       bool
}

// NewTokenTracker creates a new token tracker with the given limits
func NewTokenTracker(maxTokens, warnAt int) *TokenTracker {
	return &TokenTracker{
		MaxTokens: maxTokens,
		WarnAt:    warnAt,
	}
}

// Add adds tokens to the tracker and returns (ok, warning message)
func (t *TokenTracker) Add(input, output int) (bool, string) {
	t.InputTokens += input
	t.OutputTokens += output
	t.TotalTokens = t.InputTokens + t.OutputTokens

	// Check if unlimited
	if t.MaxTokens == 0 {
		return true, ""
	}

	// Check if exceeded
	if t.TotalTokens > t.MaxTokens {
		return false, "Token budget exceeded. Use /clear to start a new conversation."
	}

	// Check if approaching limit (warn once)
	if !t.warned && t.WarnAt > 0 && t.TotalTokens >= t.WarnAt {
		t.warned = true
		remaining := t.MaxTokens - t.TotalTokens
		return true, formatTokenWarning(remaining, t.MaxTokens)
	}

	return true, ""
}

// GetUsage returns current token usage
func (t *TokenTracker) GetUsage() (input, output, total int) {
	return t.InputTokens, t.OutputTokens, t.TotalTokens
}

// Reset resets the token tracker
func (t *TokenTracker) Reset() {
	t.InputTokens = 0
	t.OutputTokens = 0
	t.TotalTokens = 0
	t.warned = false
}

func formatTokenWarning(remaining, max int) string {
	pct := (max - remaining) * 100 / max
	return "Warning: " + strconv.Itoa(pct) + "% of token budget used (" + strconv.Itoa(remaining) + " tokens remaining). Use /clear to reset."
}
