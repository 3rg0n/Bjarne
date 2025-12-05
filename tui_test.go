package main

import (
	"testing"
)

func TestEscalationLogic(t *testing.T) {
	// Create a minimal model for testing escalation
	cfg := &Config{
		GenerateModel:     "haiku",
		OracleModel:       "opus",
		EscalationModels:  []string{"sonnet", "opus"},
		EscalateOnFailure: true,
	}

	t.Run("initial state", func(t *testing.T) {
		m := Model{config: cfg, difficulty: "EASY"}
		m.resetEscalation()

		if m.totalFixAttempts != 0 {
			t.Errorf("totalFixAttempts = %d, want 0", m.totalFixAttempts)
		}
		// EASY uses Haiku initially
		got := m.getCurrentModel()
		if got != "global.anthropic.claude-haiku-4-5-20251001-v1:0" {
			t.Errorf("getCurrentModel() = %q, want haiku model", got)
		}
	})

	t.Run("COMPLEX uses Opus", func(t *testing.T) {
		m := Model{config: cfg, difficulty: "COMPLEX"}
		m.resetEscalation()

		got := m.getCurrentModel()
		if got != "opus" {
			t.Errorf("getCurrentModel() = %q, want opus", got)
		}
	})

	t.Run("canEscalate allows 15 attempts", func(t *testing.T) {
		m := Model{config: cfg, difficulty: "EASY"}
		m.resetEscalation()

		// Should allow 15 attempts
		for i := 0; i < 15; i++ {
			if !m.canEscalate() {
				t.Errorf("should be able to do attempt %d", i+1)
			}
			m.advanceEscalation()
		}

		// After 15 attempts, should be exhausted
		if m.canEscalate() {
			t.Error("should be exhausted after 15 attempts")
		}
	})

	t.Run("EASY escalation thresholds", func(t *testing.T) {
		m := Model{config: cfg, difficulty: "EASY"}
		m.resetEscalation()

		// Attempts 1-5: Haiku
		for i := 0; i < 5; i++ {
			m.advanceEscalation()
			got := m.getCurrentModel()
			if got != "global.anthropic.claude-haiku-4-5-20251001-v1:0" {
				t.Errorf("attempt %d: getCurrentModel() = %q, want haiku", m.totalFixAttempts, got)
			}
		}

		// Attempts 6-10: Sonnet
		m.advanceEscalation() // attempt 6
		got := m.getCurrentModel()
		if got != "global.anthropic.claude-sonnet-4-5-20250929-v1:0" {
			t.Errorf("attempt 6: getCurrentModel() = %q, want sonnet", got)
		}

		// Attempts 11-15: Opus
		m.totalFixAttempts = 11
		got = m.getCurrentModel()
		if got != "opus" {
			t.Errorf("attempt 11: getCurrentModel() = %q, want opus", got)
		}
	})

	t.Run("MEDIUM stays at Sonnet then Opus", func(t *testing.T) {
		m := Model{config: cfg, difficulty: "MEDIUM"}
		m.resetEscalation()

		// MEDIUM starts with Sonnet
		got := m.getCurrentModel()
		if got != "global.anthropic.claude-sonnet-4-5-20250929-v1:0" {
			t.Errorf("MEDIUM getCurrentModel() = %q, want sonnet model", got)
		}

		// At attempt 11+, should use Opus
		m.totalFixAttempts = 11
		got = m.getCurrentModel()
		if got != "opus" {
			t.Errorf("attempt 11: getCurrentModel() = %q, want opus", got)
		}
	})
}
