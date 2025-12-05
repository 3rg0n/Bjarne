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

	t.Run("initial state EASY", func(t *testing.T) {
		m := Model{config: cfg, difficulty: "EASY"}
		m.resetEscalation()

		if m.currentIteration != 0 {
			t.Errorf("currentIteration = %d, want 0", m.currentIteration)
		}
		if m.currentModelIndex != -1 {
			t.Errorf("currentModelIndex = %d, want -1", m.currentModelIndex)
		}
		// EASY uses Haiku
		got := m.getCurrentModel()
		if got != "global.anthropic.claude-haiku-4-5-20251001-v1:0" {
			t.Errorf("getCurrentModel() = %q, want haiku model", got)
		}
	})

	t.Run("initial state COMPLEX", func(t *testing.T) {
		m := Model{config: cfg, difficulty: "COMPLEX"}
		m.resetEscalation()

		// COMPLEX uses Opus
		got := m.getCurrentModel()
		if got != "opus" {
			t.Errorf("getCurrentModel() = %q, want opus", got)
		}
	})

	t.Run("canEscalate with iterations remaining EASY", func(t *testing.T) {
		m := Model{config: cfg, difficulty: "EASY"}
		m.resetEscalation()

		if !m.canEscalate() {
			t.Error("should be able to escalate initially")
		}
	})

	t.Run("EASY escalation chain", func(t *testing.T) {
		m := Model{config: cfg, difficulty: "EASY"}
		m.resetEscalation()

		// First advance - iteration 1 with haiku
		m.advanceEscalation()
		if m.currentIteration != 1 {
			t.Errorf("after 1st advance: currentIteration = %d, want 1", m.currentIteration)
		}
		if m.currentModelIndex != -1 {
			t.Errorf("after 1st advance: currentModelIndex = %d, want -1", m.currentModelIndex)
		}

		// Second advance - iteration 2 with haiku
		m.advanceEscalation()
		if m.currentIteration != 2 {
			t.Errorf("after 2nd advance: currentIteration = %d, want 2", m.currentIteration)
		}

		// Third advance - iteration 3, then escalate to modelIndex 0
		m.advanceEscalation()
		if m.currentIteration != 0 {
			t.Errorf("after 3rd advance: currentIteration = %d, want 0 (reset after escalation)", m.currentIteration)
		}
		if m.currentModelIndex != 0 {
			t.Errorf("after 3rd advance: currentModelIndex = %d, want 0", m.currentModelIndex)
		}
		// EASY escalates to Sonnet
		got := m.getCurrentModel()
		if got != "global.anthropic.claude-sonnet-4-5-20250929-v1:0" {
			t.Errorf("after 3rd advance: getCurrentModel() = %q, want sonnet model", got)
		}
	})

	t.Run("COMPLEX allows 4 attempts", func(t *testing.T) {
		m := Model{config: cfg, difficulty: "COMPLEX"}
		m.resetEscalation()

		// COMPLEX allows 4 total attempts with Opus
		for i := 0; i < 4; i++ {
			if !m.canEscalate() {
				t.Errorf("should be able to do attempt %d", i+1)
			}
			m.advanceEscalation()
		}

		// After 4 attempts, should be exhausted
		if m.canEscalate() {
			t.Error("COMPLEX should be exhausted after 4 attempts")
		}
	})

	t.Run("MEDIUM escalation to opus only", func(t *testing.T) {
		m := Model{config: cfg, difficulty: "MEDIUM"}
		m.resetEscalation()

		// MEDIUM starts with Sonnet
		got := m.getCurrentModel()
		if got != "global.anthropic.claude-sonnet-4-5-20250929-v1:0" {
			t.Errorf("MEDIUM getCurrentModel() = %q, want sonnet model", got)
		}

		// Exhaust Sonnet (3 iterations)
		m.advanceEscalation()
		m.advanceEscalation()
		m.advanceEscalation()

		// Should escalate to Opus
		got = m.getCurrentModel()
		if got != "opus" {
			t.Errorf("after exhausting sonnet: getCurrentModel() = %q, want opus", got)
		}
	})
}
