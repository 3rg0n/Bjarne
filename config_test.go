package main

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxIterations != 3 {
		t.Errorf("MaxIterations = %d, want 3", cfg.MaxIterations)
	}
	if cfg.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d, want 8192", cfg.MaxTokens)
	}
	if cfg.MaxTotalTokens != 150000 {
		t.Errorf("MaxTotalTokens = %d, want 150000", cfg.MaxTotalTokens)
	}
	if cfg.Theme == nil {
		t.Error("Theme should not be nil")
	}
	if cfg.Settings == nil {
		t.Error("Settings should not be nil")
	}
}

func TestLoadConfig(t *testing.T) {
	// Test with environment overrides
	t.Setenv("BJARNE_MAX_ITERATIONS", "5")
	t.Setenv("BJARNE_MAX_TOKENS", "8192")
	t.Setenv("BJARNE_MAX_TOTAL_TOKENS", "50000")
	t.Setenv("BJARNE_MODEL", "test-model")
	t.Setenv("BJARNE_VALIDATOR_IMAGE", "test:image")
	t.Setenv("BJARNE_THEME", "matrix")

	cfg := LoadConfig()

	if cfg.MaxIterations != 5 {
		t.Errorf("MaxIterations = %d, want 5", cfg.MaxIterations)
	}
	if cfg.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d, want 8192", cfg.MaxTokens)
	}
	if cfg.MaxTotalTokens != 50000 {
		t.Errorf("MaxTotalTokens = %d, want 50000", cfg.MaxTotalTokens)
	}
	if cfg.GenerateModel != "test-model" {
		t.Errorf("GenerateModel = %q, want test-model", cfg.GenerateModel)
	}
	if cfg.ValidatorImage != "test:image" {
		t.Errorf("ValidatorImage = %q, want test:image", cfg.ValidatorImage)
	}
	// WarnAt should be 80% of 50000 = 40000
	if cfg.WarnTokenThreshold != 40000 {
		t.Errorf("WarnTokenThreshold = %d, want 40000", cfg.WarnTokenThreshold)
	}
	if cfg.Settings.Theme.Name != "matrix" {
		t.Errorf("Theme.Name = %q, want matrix", cfg.Settings.Theme.Name)
	}
}

func TestLoadConfigUnlimited(t *testing.T) {
	t.Setenv("BJARNE_MAX_TOTAL_TOKENS", "0")

	cfg := LoadConfig()

	if cfg.MaxTotalTokens != 0 {
		t.Errorf("MaxTotalTokens = %d, want 0 (unlimited)", cfg.MaxTotalTokens)
	}
}

func TestTokenTracker(t *testing.T) {
	t.Run("basic tracking", func(t *testing.T) {
		tracker := NewTokenTracker(1000, 800)

		ok, warn := tracker.Add(100, 200)
		if !ok {
			t.Error("Add should succeed")
		}
		if warn != "" {
			t.Error("Should not warn yet")
		}

		input, output, total := tracker.GetUsage()
		if input != 100 || output != 200 || total != 300 {
			t.Errorf("GetUsage = (%d, %d, %d), want (100, 200, 300)", input, output, total)
		}
	})

	t.Run("warning at threshold", func(t *testing.T) {
		tracker := NewTokenTracker(1000, 800)

		// Add enough to trigger warning
		ok, warn := tracker.Add(500, 400) // 900 total, above 800 threshold
		if !ok {
			t.Error("Add should succeed")
		}
		if warn == "" {
			t.Error("Should warn at threshold")
		}

		// Should not warn again
		ok, warn = tracker.Add(10, 10)
		if !ok {
			t.Error("Add should succeed")
		}
		if warn != "" {
			t.Error("Should not warn again")
		}
	})

	t.Run("exceed limit", func(t *testing.T) {
		tracker := NewTokenTracker(1000, 800)

		tracker.Add(500, 400)
		ok, warn := tracker.Add(200, 200) // 1100 total, exceeds 1000

		if ok {
			t.Error("Add should fail when exceeding limit")
		}
		if warn == "" {
			t.Error("Should return error message")
		}
	})

	t.Run("unlimited mode", func(t *testing.T) {
		tracker := NewTokenTracker(0, 0) // 0 = unlimited

		ok, warn := tracker.Add(50000, 50000)
		if !ok {
			t.Error("Add should always succeed in unlimited mode")
		}
		if warn != "" {
			t.Error("Should not warn in unlimited mode")
		}
	})

	t.Run("reset", func(t *testing.T) {
		tracker := NewTokenTracker(1000, 800)
		tracker.Add(500, 400)
		tracker.Reset()

		input, output, total := tracker.GetUsage()
		if input != 0 || output != 0 || total != 0 {
			t.Errorf("After reset: GetUsage = (%d, %d, %d), want (0, 0, 0)", input, output, total)
		}
	})
}
