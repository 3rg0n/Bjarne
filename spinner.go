package main

import (
	"fmt"
	"sync"
	"time"
)

// Spinner displays an animated spinner with a message
type Spinner struct {
	message  string
	frames   []string
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}
	mu       sync.Mutex
	theme    *Theme
}

// NewSpinner creates a new spinner with a message
func NewSpinner(message string, theme *Theme) *Spinner {
	return &Spinner{
		message:  message,
		frames:   []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		interval: 80 * time.Millisecond,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		theme:    theme,
	}
}

// Start begins the spinner animation
func (s *Spinner) Start() {
	go func() {
		defer close(s.done)
		i := 0
		for {
			select {
			case <-s.stop:
				return
			default:
				s.mu.Lock()
				// Clear line and print spinner
				fmt.Printf("\r%s %s", s.theme.Info(s.frames[i]), s.message)
				s.mu.Unlock()
				i = (i + 1) % len(s.frames)
				time.Sleep(s.interval)
			}
		}
	}()
}

// Update changes the spinner message
func (s *Spinner) Update(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Clear the line first
	fmt.Printf("\r\033[K")
	s.message = message
}

// Success stops the spinner and shows a success message
func (s *Spinner) Success(message string) {
	close(s.stop)
	<-s.done
	fmt.Printf("\r\033[K%s %s\n", s.theme.Success("✓"), message)
}

// Fail stops the spinner and shows a failure message
func (s *Spinner) Fail(message string) {
	close(s.stop)
	<-s.done
	fmt.Printf("\r\033[K%s %s\n", s.theme.Error("✗"), message)
}

// Stop stops the spinner without a final message
func (s *Spinner) Stop() {
	close(s.stop)
	<-s.done
	fmt.Printf("\r\033[K")
}

// ThinkingSpinner displays an animated thinking indicator like Claude Code
// Example: ✽ Thinking… (esc to interrupt · 12s · ↓ 1.2k tokens)
type ThinkingSpinner struct {
	message   string
	frames    []string
	interval  time.Duration
	stop      chan struct{}
	done      chan struct{}
	mu        sync.Mutex
	theme     *Theme
	startTime time.Time
	tokens    int
}

// NewThinkingSpinner creates a new thinking spinner
func NewThinkingSpinner(message string, theme *Theme) *ThinkingSpinner {
	return &ThinkingSpinner{
		message:   message,
		frames:    []string{"✽", "✻", "✼", "✽", "✻", "✼"},
		interval:  150 * time.Millisecond,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
		theme:     theme,
		startTime: time.Now(),
		tokens:    0,
	}
}

// Start begins the thinking animation
func (t *ThinkingSpinner) Start() {
	t.startTime = time.Now()
	go func() {
		defer close(t.done)
		i := 0
		for {
			select {
			case <-t.stop:
				return
			default:
				t.mu.Lock()
				elapsed := time.Since(t.startTime)
				line := t.formatLine(t.frames[i], elapsed)
				fmt.Printf("\r\033[K%s", line)
				t.mu.Unlock()
				i = (i + 1) % len(t.frames)
				time.Sleep(t.interval)
			}
		}
	}()
}

// formatLine formats the thinking indicator line
func (t *ThinkingSpinner) formatLine(frame string, elapsed time.Duration) string {
	// Format elapsed time
	secs := int(elapsed.Seconds())

	// Build the status parts
	parts := []string{fmt.Sprintf("%ds", secs)}

	if t.tokens > 0 {
		parts = append(parts, fmt.Sprintf("↓ %s tokens", formatTokenCount(t.tokens)))
	}

	status := t.theme.Dim("(" + joinParts(parts) + ")")

	return fmt.Sprintf("%s %s %s", t.theme.Accent(frame), t.message, status)
}

// UpdateTokens updates the token count display
func (t *ThinkingSpinner) UpdateTokens(tokens int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tokens = tokens
}

// Success stops with a success indicator
func (t *ThinkingSpinner) Success(message string) {
	close(t.stop)
	<-t.done
	elapsed := time.Since(t.startTime)
	fmt.Printf("\r\033[K%s %s %s\n",
		t.theme.Success("✓"),
		message,
		t.theme.Dim(fmt.Sprintf("(%.1fs)", elapsed.Seconds())))
}

// Fail stops with a failure indicator
func (t *ThinkingSpinner) Fail(message string) {
	close(t.stop)
	<-t.done
	fmt.Printf("\r\033[K%s %s\n", t.theme.Error("✗"), message)
}

// Stop stops the spinner without a message
func (t *ThinkingSpinner) Stop() {
	close(t.stop)
	<-t.done
	fmt.Printf("\r\033[K")
}

// formatTokenCount formats token count with k suffix for thousands
func formatTokenCount(tokens int) string {
	if tokens >= 1000 {
		return fmt.Sprintf("%.1fk", float64(tokens)/1000)
	}
	return fmt.Sprintf("%d", tokens)
}

// joinParts joins string parts with " · "
func joinParts(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " · "
		}
		result += p
	}
	return result
}

// ValidationSpinner manages spinners for the validation pipeline
type ValidationSpinner struct {
	theme   *Theme
	current *Spinner
}

// NewValidationSpinner creates a new validation spinner manager
func NewValidationSpinner(theme *Theme) *ValidationSpinner {
	return &ValidationSpinner{theme: theme}
}

// StartStage begins a new validation stage with a spinner
func (v *ValidationSpinner) StartStage(stage string) {
	v.current = NewSpinner(fmt.Sprintf("Running %s...", stage), v.theme)
	v.current.Start()
}

// StageSuccess marks the current stage as successful
func (v *ValidationSpinner) StageSuccess(stage string, duration time.Duration) {
	if v.current != nil {
		v.current.Success(fmt.Sprintf("%s (%.2fs)", stage, duration.Seconds()))
	}
}

// StageFail marks the current stage as failed
func (v *ValidationSpinner) StageFail(stage string, duration time.Duration) {
	if v.current != nil {
		v.current.Fail(fmt.Sprintf("%s (%.2fs)", stage, duration.Seconds()))
	}
}

// ShowIterating displays an iteration message
func (v *ValidationSpinner) ShowIterating(attempt, max int, model string) {
	fmt.Printf("%s Iterating... (attempt %d/%d, using %s)\n",
		v.theme.Warning("↻"), attempt, max, model)
}

// ShowEscalating displays an escalation message
func (v *ValidationSpinner) ShowEscalating(model string) {
	fmt.Printf("%s Escalating to %s...\n", v.theme.Warning("⬆"), model)
}
