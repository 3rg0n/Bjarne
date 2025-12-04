package main

import (
	"os"
	"strconv"
)

// Config holds runtime configuration (merged from settings.json + env vars)
type Config struct {
	// Settings (loaded from ~/.bjarne/settings.json)
	Settings *Settings

	// Theme (created from settings)
	Theme *Theme

	// Derived/override values (env vars override settings)
	MaxIterations      int
	MaxTokens          int
	MaxTotalTokens     int
	WarnTokenThreshold int
	ValidatorImage     string

	// Model configuration
	ChatModel         string   // Model for chat/non-code responses
	ReflectionModel   string   // Model for initial prompt analysis
	GenerateModel     string   // Model for initial code generation
	OracleModel       string   // Model for deep analysis (COMPLEX tasks)
	EscalationModels  []string // Models to try on validation failure
	EscalateOnFailure bool
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	settings := DefaultSettings()
	return configFromSettings(settings)
}

// configFromSettings creates a Config from Settings
func configFromSettings(settings *Settings) *Config {
	return &Config{
		Settings:           settings,
		Theme:              NewTheme(&settings.Theme),
		MaxIterations:      settings.Validation.MaxIterations,
		MaxTokens:          settings.Tokens.MaxPerResponse,
		MaxTotalTokens:     settings.Tokens.MaxPerSession,
		WarnTokenThreshold: settings.Tokens.MaxPerSession * 80 / 100,
		ValidatorImage:     settings.Container.Image,
		ChatModel:          settings.Models.Chat,
		ReflectionModel:    settings.Models.Reflection,
		GenerateModel:      settings.Models.Generate,
		OracleModel:        settings.Models.Oracle,
		EscalationModels:   settings.Models.Escalation,
		EscalateOnFailure:  settings.Validation.EscalateOnFailure,
	}
}

// LoadConfig loads configuration from settings.json, then applies env var overrides
func LoadConfig() *Config {
	// Load settings from file (or defaults if not found)
	settings, _ := LoadSettings()
	cfg := configFromSettings(settings)

	// Environment variable overrides
	if val := os.Getenv("BJARNE_MAX_ITERATIONS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			cfg.MaxIterations = n
		}
	}

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

	// Model overrides (single model override applies to generate)
	if val := os.Getenv("BJARNE_MODEL"); val != "" {
		cfg.GenerateModel = val
	}
	if val := os.Getenv("BJARNE_CHAT_MODEL"); val != "" {
		cfg.ChatModel = val
	}

	if val := os.Getenv("BJARNE_VALIDATOR_IMAGE"); val != "" {
		cfg.ValidatorImage = val
	}

	if val := os.Getenv("BJARNE_THEME"); val != "" {
		if _, ok := ThemePresets[val]; ok {
			cfg.Settings.Theme.Name = val
			cfg.Theme = NewTheme(&cfg.Settings.Theme)
		}
	}

	// Recalculate warning threshold if max changed
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
