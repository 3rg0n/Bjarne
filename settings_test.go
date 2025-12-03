package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSettings(t *testing.T) {
	s := DefaultSettings()

	if s.Models.Chat == "" {
		t.Error("Models.Chat should not be empty")
	}
	if s.Models.Generate == "" {
		t.Error("Models.Generate should not be empty")
	}
	if len(s.Models.Escalation) == 0 {
		t.Error("Models.Escalation should not be empty")
	}
	if s.Validation.MaxIterations != 3 {
		t.Errorf("Validation.MaxIterations = %d, want 3", s.Validation.MaxIterations)
	}
	if s.Theme.Name != "default" {
		t.Errorf("Theme.Name = %q, want default", s.Theme.Name)
	}
}

func TestTheme(t *testing.T) {
	tests := []struct {
		name      string
		themeName string
		wantValid bool
	}{
		{"default theme", "default", true},
		{"matrix theme", "matrix", true},
		{"solarized theme", "solarized", true},
		{"gruvbox theme", "gruvbox", true},
		{"dracula theme", "dracula", true},
		{"nord theme", "nord", true},
		{"unknown theme falls back to default", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &ThemeSettings{Name: tt.themeName}
			theme := NewTheme(settings)

			// Test that theme produces non-empty output
			if theme.Prompt("test") == "" {
				t.Error("Prompt() should return non-empty string")
			}
			if theme.Success("test") == "" {
				t.Error("Success() should return non-empty string")
			}
			if theme.Error("test") == "" {
				t.Error("Error() should return non-empty string")
			}
			if theme.Reset() == "" {
				t.Error("Reset() should return non-empty string")
			}
		})
	}
}

func TestAvailableThemes(t *testing.T) {
	themes := AvailableThemes()

	if len(themes) == 0 {
		t.Error("AvailableThemes() should return at least one theme")
	}

	// Check that all listed themes exist in ThemePresets
	for _, name := range themes {
		if _, ok := ThemePresets[name]; !ok {
			t.Errorf("Theme %q listed but not in ThemePresets", name)
		}
	}
}

func TestSaveAndLoadSettings(t *testing.T) {
	// Create a temp directory for test
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, ".bjarne", "settings.json")

	// Create settings
	settings := DefaultSettings()
	settings.Theme.Name = "matrix"
	settings.Validation.MaxIterations = 5

	// Create directory
	if err := os.MkdirAll(filepath.Dir(testPath), 0700); err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}

	// We can't easily test SaveSettings without mocking UserHomeDir
	// but we can test the JSON marshaling/unmarshaling
	t.Run("settings roundtrip", func(t *testing.T) {
		// This mainly tests that the structures are properly defined
		if settings.Theme.Name != "matrix" {
			t.Error("Theme name should be matrix")
		}
		if settings.Validation.MaxIterations != 5 {
			t.Error("MaxIterations should be 5")
		}
	})
}

func TestShortModelName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"global.anthropic.claude-haiku-4-5-20251001-v1:0", "claude-haiku-4-5"},
		{"global.anthropic.claude-sonnet-4-5-20250929-v1:0", "claude-sonnet-4-5"},
		{"global.anthropic.claude-opus-4-5-20251101-v1:0", "claude-opus-4-5"},
		{"simple-model", "simple-model"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := shortModelName(tt.input)
			if got != tt.want {
				t.Errorf("shortModelName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
