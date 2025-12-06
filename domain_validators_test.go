package main

import (
	"testing"
)

func TestAllPassed(t *testing.T) {
	tests := []struct {
		name     string
		results  []ValidationResult
		expected bool
	}{
		{
			name:     "empty results",
			results:  []ValidationResult{},
			expected: true,
		},
		{
			name: "all passed",
			results: []ValidationResult{
				{Stage: "compile", Success: true},
				{Stage: "asan", Success: true},
			},
			expected: true,
		},
		{
			name: "one failed",
			results: []ValidationResult{
				{Stage: "compile", Success: true},
				{Stage: "asan", Success: false},
			},
			expected: false,
		},
		{
			name: "first failed",
			results: []ValidationResult{
				{Stage: "compile", Success: false},
				{Stage: "asan", Success: true},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := allPassed(tt.results)
			if result != tt.expected {
				t.Errorf("allPassed() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestValidatorConfig(t *testing.T) {
	cfg := DefaultValidatorConfig()

	// Core validators should be enabled by default
	if !cfg.IsEnabled(ValidatorClangTidy) {
		t.Error("clang-tidy should be enabled by default")
	}
	if !cfg.IsEnabled(ValidatorASAN) {
		t.Error("ASAN should be enabled by default")
	}

	// Domain validators should be disabled by default
	if cfg.IsEnabled(ValidatorFrameTiming) {
		t.Error("frame-timing should be disabled by default")
	}
	if cfg.IsEnabled(ValidatorLatency) {
		t.Error("latency should be disabled by default")
	}
	if cfg.IsEnabled(ValidatorFuzz) {
		t.Error("fuzz should be disabled by default")
	}

	// Test enable category
	cfg.EnableCategory(CategorySecurity)
	if !cfg.IsEnabled(ValidatorFuzz) {
		t.Error("fuzz should be enabled after EnableCategory(security)")
	}
	if !cfg.IsEnabled(ValidatorSecStatic) {
		t.Error("sec-static should be enabled after EnableCategory(security)")
	}

	// Test disable category
	cfg.DisableCategory(CategorySecurity)
	if cfg.IsEnabled(ValidatorFuzz) {
		t.Error("fuzz should be disabled after DisableCategory(security)")
	}

	// Test toggle
	cfg.Toggle(ValidatorLatency)
	if !cfg.IsEnabled(ValidatorLatency) {
		t.Error("latency should be enabled after toggle")
	}
	cfg.Toggle(ValidatorLatency)
	if cfg.IsEnabled(ValidatorLatency) {
		t.Error("latency should be disabled after second toggle")
	}
}

func TestGetValidatorsByCategory(t *testing.T) {
	byCategory := GetValidatorsByCategory()

	// Check core validators
	core := byCategory[CategoryCore]
	if len(core) == 0 {
		t.Error("should have core validators")
	}

	// Check that clang-tidy is in core
	found := false
	for _, v := range core {
		if v.ID == ValidatorClangTidy {
			found = true
			break
		}
	}
	if !found {
		t.Error("clang-tidy should be in core category")
	}

	// Check game validators exist
	game := byCategory[CategoryGame]
	if len(game) != 3 {
		t.Errorf("game category should have 3 validators, got %d", len(game))
	}

	// Check security validators exist
	security := byCategory[CategorySecurity]
	if len(security) != 3 {
		t.Errorf("security category should have 3 validators, got %d", len(security))
	}
}

func TestParseArg(t *testing.T) {
	tests := []struct {
		arg      string
		key      string
		expected int
		wantErr  bool
	}{
		{"max_kb=256", "max_kb", 256, false},
		{"p99_us=100", "p99_us", 100, false},
		{"target_fps=60", "target_fps", 60, false},
		{"wrong_key=100", "max_kb", 0, true},
		{"invalid", "max_kb", 0, true},
		{"max_kb=notanumber", "max_kb", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			result, err := parseArg(tt.arg, tt.key)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("parseArg() = %d, want %d", result, tt.expected)
				}
			}
		})
	}
}
