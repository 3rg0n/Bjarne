package main

import (
	"testing"
)

func TestEscalationLogic(t *testing.T) {
	// Create a minimal model for testing escalation
	cfg := &Config{
		GenerateModel:     "haiku",
		EscalationModels:  []string{"sonnet", "opus"},
		EscalateOnFailure: true,
	}

	m := Model{config: cfg}

	t.Run("initial state", func(t *testing.T) {
		m.resetEscalation()

		if m.currentIteration != 0 {
			t.Errorf("currentIteration = %d, want 0", m.currentIteration)
		}
		if m.currentModelIndex != -1 {
			t.Errorf("currentModelIndex = %d, want -1", m.currentModelIndex)
		}
		if m.getCurrentModel() != "haiku" {
			t.Errorf("getCurrentModel() = %q, want haiku", m.getCurrentModel())
		}
	})

	t.Run("canEscalate with iterations remaining", func(t *testing.T) {
		m.resetEscalation()

		if !m.canEscalate() {
			t.Error("should be able to escalate initially")
		}
	})

	t.Run("advance through escalation chain", func(t *testing.T) {
		m.resetEscalation()

		// First advance - iteration 1 with haiku
		m.advanceEscalation()
		if m.currentIteration != 1 {
			t.Errorf("after 1st advance: currentIteration = %d, want 1", m.currentIteration)
		}
		if m.currentModelIndex != -1 {
			t.Errorf("after 1st advance: currentModelIndex = %d, want -1", m.currentModelIndex)
		}
		if m.getCurrentModel() != "haiku" {
			t.Errorf("after 1st advance: getCurrentModel() = %q, want haiku", m.getCurrentModel())
		}

		// Second advance - iteration 2 with haiku, then escalate
		m.advanceEscalation()
		if m.currentIteration != 0 {
			t.Errorf("after 2nd advance: currentIteration = %d, want 0 (reset after escalation)", m.currentIteration)
		}
		if m.currentModelIndex != 0 {
			t.Errorf("after 2nd advance: currentModelIndex = %d, want 0", m.currentModelIndex)
		}
		if m.getCurrentModel() != "sonnet" {
			t.Errorf("after 2nd advance: getCurrentModel() = %q, want sonnet", m.getCurrentModel())
		}

		// Third advance - iteration 1 with sonnet
		m.advanceEscalation()
		if m.currentIteration != 1 {
			t.Errorf("after 3rd advance: currentIteration = %d, want 1", m.currentIteration)
		}
		if m.getCurrentModel() != "sonnet" {
			t.Errorf("after 3rd advance: getCurrentModel() = %q, want sonnet", m.getCurrentModel())
		}

		// Fourth advance - escalate to opus
		m.advanceEscalation()
		if m.currentModelIndex != 1 {
			t.Errorf("after 4th advance: currentModelIndex = %d, want 1", m.currentModelIndex)
		}
		if m.getCurrentModel() != "opus" {
			t.Errorf("after 4th advance: getCurrentModel() = %q, want opus", m.getCurrentModel())
		}
	})

	t.Run("canEscalate exhaustion", func(t *testing.T) {
		m.resetEscalation()

		// Exhaust haiku (2 iterations)
		m.advanceEscalation() // haiku attempt 1
		if !m.canEscalate() {
			t.Error("should be able to escalate after haiku attempt 1")
		}

		m.advanceEscalation() // haiku attempt 2 -> escalate to sonnet
		if !m.canEscalate() {
			t.Error("should be able to escalate with sonnet available")
		}

		// Exhaust sonnet (2 iterations)
		m.advanceEscalation() // sonnet attempt 1
		if !m.canEscalate() {
			t.Error("should be able to escalate after sonnet attempt 1")
		}

		m.advanceEscalation() // sonnet attempt 2 -> escalate to opus
		if !m.canEscalate() {
			t.Error("should be able to escalate with opus available")
		}

		// Exhaust opus (2 iterations)
		m.advanceEscalation() // opus attempt 1
		if !m.canEscalate() {
			t.Error("should be able to escalate after opus attempt 1")
		}

		m.advanceEscalation() // opus attempt 2 -> no more models

		// Now we should be exhausted
		if m.canEscalate() {
			t.Error("should NOT be able to escalate after exhausting all models")
		}
	})

	t.Run("no escalation models", func(t *testing.T) {
		cfgNoEscalation := &Config{
			GenerateModel:     "haiku",
			EscalationModels:  []string{},
			EscalateOnFailure: true,
		}
		m2 := Model{config: cfgNoEscalation}
		m2.resetEscalation()

		// Should be able to do 2 iterations with haiku
		if !m2.canEscalate() {
			t.Error("should be able to escalate initially")
		}

		m2.advanceEscalation()
		if !m2.canEscalate() {
			t.Error("should be able to do second iteration")
		}

		m2.advanceEscalation() // This would normally escalate, but no models available

		if m2.canEscalate() {
			t.Error("should NOT be able to escalate without escalation models")
		}
	})
}
