package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// State represents the current state of the TUI
type State int

const (
	StateInput         State = iota
	StateClassifying         // Quick classification with Haiku
	StateThinking            // Full analysis with model based on complexity
	StateAcknowledging       // Processing user's response to clarifying questions
	StateGenerating
	StateValidating
	StateFixing   // Attempting to fix failed code
	StateRevealing // Animated code reveal
)

// Box drawing characters for visual sections
const (
	boxTopLeft     = "╔"
	boxTopRight    = "╗"
	boxBottomLeft  = "╚"
	boxBottomRight = "╝"
	boxHorizontal  = "═"
	boxVertical    = "║"
	treeVert       = "│"
	treeBranch     = "├─"
	treeEnd        = "└─"
)

// Styles for the TUI
type Styles struct {
	Prompt    lipgloss.Style
	Success   lipgloss.Style
	Error     lipgloss.Style
	Warning   lipgloss.Style
	Info      lipgloss.Style
	Accent    lipgloss.Style
	Dim       lipgloss.Style
	Code      lipgloss.Style
	Checkmark lipgloss.Style
	Cross     lipgloss.Style
}

func NewStyles() *Styles {
	return &Styles{
		Prompt:    lipgloss.NewStyle().Foreground(lipgloss.Color("12")), // Blue
		Success:   lipgloss.NewStyle().Foreground(lipgloss.Color("10")), // Green
		Error:     lipgloss.NewStyle().Foreground(lipgloss.Color("9")),  // Red
		Warning:   lipgloss.NewStyle().Foreground(lipgloss.Color("11")), // Yellow
		Info:      lipgloss.NewStyle().Foreground(lipgloss.Color("14")), // Cyan
		Accent:    lipgloss.NewStyle().Foreground(lipgloss.Color("13")), // Magenta
		Dim:       lipgloss.NewStyle().Foreground(lipgloss.Color("8")),  // Gray
		Code:      lipgloss.NewStyle().Foreground(lipgloss.Color("15")), // White
		Checkmark: lipgloss.NewStyle().Foreground(lipgloss.Color("10")), // Green
		Cross:     lipgloss.NewStyle().Foreground(lipgloss.Color("9")),  // Red
	}
}

// Model is the bubbletea model for bjarne
type Model struct {
	// Core components
	textarea textarea.Model
	spinner  spinner.Model
	styles   *Styles

	// State
	state          State
	statusMsg      string
	startTime      time.Time
	tokenCount     int
	currentCode    string
	validated      bool
	analyzed       bool              // True after first analysis, subsequent inputs go to generation
	originalPrompt string            // Store original prompt to parse examples
	examples       *ExampleTests     // Parsed example tests from prompt
	dod            *DefinitionOfDone // Definition of Done for complex tasks
	difficulty     string            // EASY, MEDIUM, COMPLEX from reflection

	// Escalation tracking
	currentIteration   int      // Current fix attempt (1-based)
	currentModelIndex  int      // Index into escalation chain (-1 = generate model)
	lastValidationErrs string   // Last validation errors for fix prompt
	modelsUsed         []string // Track which models we've tried

	// Exit confirmation
	ctrlCPressed bool      // True if Ctrl+C was pressed once
	ctrlCTime    time.Time // When Ctrl+C was pressed (for timeout)

	// Code reveal animation
	revealLines       []string // Lines to reveal
	revealCurrentLine int      // Current line being revealed
	revealTotalTime   float64  // Total validation time to show after reveal

	// Session data
	bedrock      *BedrockClient
	container    *ContainerRuntime
	config       *Config
	tokenTracker *TokenTracker
	conversation []Message

	// For async operations
	ctx      context.Context
	cancelFn context.CancelFunc

	// Terminal size
	width  int
	height int
}

// Messages for async operations
type classificationDoneMsg struct {
	result *GenerateResult
	err    error
}

type thinkingDoneMsg struct {
	result *GenerateResult
	err    error
}

type generatingDoneMsg struct {
	result *GenerateResult
	err    error
}

type acknowledgeDoneMsg struct {
	result *GenerateResult
	err    error
}

type validationDoneMsg struct {
	results []ValidationResult
	err     error
}

type fixDoneMsg struct {
	result *GenerateResult
	err    error
}

type tickMsg time.Time

// codeRevealMsg is sent to reveal code line by line
type codeRevealMsg struct {
	lines       []string
	currentLine int
}

// codeRevealDoneMsg indicates code reveal animation is complete
type codeRevealDoneMsg struct{}

// NewModel creates a new bubbletea model
func NewModel(bedrock *BedrockClient, container *ContainerRuntime, cfg *Config) Model {
	// Create textarea for input
	ta := textarea.New()
	ta.Placeholder = "What would you have me create?"
	ta.Focus()
	ta.CharLimit = 0 // No limit
	ta.SetWidth(100) // Will be resized on WindowSizeMsg
	ta.SetHeight(3)  // Allow multi-line for longer prompts
	ta.ShowLineNumbers = false
	ta.Prompt = ""                                   // No prompt prefix (we draw our own >)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle() // No highlight
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = lipgloss.NewStyle()
	ta.BlurredStyle.Prompt = lipgloss.NewStyle()
	ta.KeyMap.InsertNewline.SetEnabled(false) // Enter submits, Shift+Enter for newlines if needed

	// Create spinner - simple ASCII
	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: []string{"|", "/", "-", "\\"},
		FPS:    time.Millisecond * 100,
	}

	return Model{
		textarea:     ta,
		spinner:      s,
		styles:       NewStyles(),
		state:        StateInput,
		bedrock:      bedrock,
		container:    container,
		config:       cfg,
		tokenTracker: NewTokenTracker(cfg.MaxTotalTokens, cfg.WarnTokenThreshold),
		conversation: []Message{},
		ctx:          context.Background(),
		width:        120, // Default, will be updated on WindowSizeMsg
		height:       24,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Resize textarea to fit terminal width (minus prompt "> ")
		inputWidth := msg.Width - 4 // Account for "> " prefix and some padding
		if inputWidth < 40 {
			inputWidth = 40
		}
		m.textarea.SetWidth(inputWidth)
		return m, nil

	case tea.KeyMsg:
		// Reset Ctrl+C state on any other key press
		if msg.Type != tea.KeyCtrlC {
			m.ctrlCPressed = false
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			// Double Ctrl+C to quit
			if m.ctrlCPressed && time.Since(m.ctrlCTime) < 2*time.Second {
				return m, tea.Quit
			}
			m.ctrlCPressed = true
			m.ctrlCTime = time.Now()
			m.addOutput("")
			m.addOutput(m.styles.Warning.Render("Press Ctrl+C again to exit"))
			return m, nil

		case tea.KeyEsc:
			// Cancel current operation if processing
			if m.state != StateInput {
				if m.cancelFn != nil {
					m.cancelFn()
				}
				m.state = StateInput
				m.addOutput(m.styles.Warning.Render("-- Interrupted --"))
				m.textarea.Focus()
				return m, nil
			}

		case tea.KeyEnter:
			if m.state == StateInput {
				input := strings.TrimSpace(m.textarea.Value())
				if input == "" {
					return m, nil
				}

				// Handle commands
				if strings.HasPrefix(input, "/") {
					return m.handleCommand(input)
				}

				m.textarea.Reset()
				m.textarea.Blur()

				// If already analyzed, user response goes to acknowledgment then generation
				if m.analyzed {
					// Show what the user typed
					m.addOutput("")
					m.addOutput(m.styles.Prompt.Render("> ") + input)
					m.conversation = append(m.conversation, Message{Role: "user", Content: input})
					return m.startAcknowledging()
				}

				// First input - start with classification
				return m.startClassifying(input)
			}
			return m, nil
		}

		// Handle input in input state
		if m.state == StateInput {
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			cmds = append(cmds, cmd)
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case classificationDoneMsg:
		if msg.err != nil {
			if m.ctx.Err() == context.Canceled {
				return m, nil
			}
			// Classification failed - default to MEDIUM and continue
			m.addOutput(m.styles.Warning.Render("Classification failed, defaulting to MEDIUM"))
			m.difficulty = "MEDIUM"
			return m.startThinking(m.getModelForComplexity("MEDIUM"))
		}

		// Parse the classification result (EASY/MEDIUM/COMPLEX)
		m.tokenTracker.Add(msg.result.InputTokens, msg.result.OutputTokens)
		classification := strings.TrimSpace(strings.ToUpper(msg.result.Text))

		// Normalize the classification
		switch {
		case strings.Contains(classification, "EASY"):
			m.difficulty = "EASY"
		case strings.Contains(classification, "COMPLEX"):
			m.difficulty = "COMPLEX"
		default:
			m.difficulty = "MEDIUM"
		}

		// Show classification result
		m.addOutput("")
		var diffDisplay string
		switch m.difficulty {
		case "EASY":
			diffDisplay = m.styles.Success.Render("[EASY]")
		case "MEDIUM":
			diffDisplay = m.styles.Warning.Render("[MEDIUM]")
		case "COMPLEX":
			diffDisplay = m.styles.Error.Render("[COMPLEX]")
		}
		m.addOutput(fmt.Sprintf("Complexity: %s", diffDisplay))

		// Select model based on complexity and start analysis
		model := m.getModelForComplexity(m.difficulty)
		return m.startThinking(model)

	case thinkingDoneMsg:
		if msg.err != nil {
			if m.ctx.Err() == context.Canceled {
				// Already handled by Esc
				return m, nil
			}
			m.addOutput(m.styles.Error.Render("Analysis failed: " + msg.err.Error()))
			m.state = StateInput
			m.textarea.Focus()
			return m, nil
		}
		// Show analysis from the appropriate model (already selected by classification)
		m.tokenTracker.Add(msg.result.InputTokens, msg.result.OutputTokens)
		m.conversation = append(m.conversation, Message{Role: "assistant", Content: msg.result.Text})

		// Parse and clean the response (remove difficulty tag if present)
		_, reflection := parseDifficulty(msg.result.Text)

		// Show analysis box
		m.addOutput("")
		m.drawBox("ANALYSIS", 56)
		m.addOutput("")

		// Display analysis with word wrapping
		cleanText := stripMarkdown(reflection)
		lines := wrapText(cleanText, 76)
		for _, line := range lines {
			m.addOutput(line)
		}
		m.addOutput("")

		if m.difficulty == "EASY" {
			// Skip confirmation for easy tasks - generate immediately
			m.conversation = append(m.conversation, Message{Role: "user", Content: GenerateNowPrompt})
			return m.startGenerating()
		}

		// For MEDIUM/COMPLEX: wait for user input before generating
		m.analyzed = true // Next input goes to acknowledgment
		m.state = StateInput
		m.textarea.Focus()
		return m, textarea.Blink

	case acknowledgeDoneMsg:
		if msg.err != nil {
			if m.ctx.Err() == context.Canceled {
				return m, nil
			}
			m.addOutput(m.styles.Error.Render("Error: " + msg.err.Error()))
			m.state = StateInput
			m.textarea.Focus()
			return m, nil
		}
		m.tokenTracker.Add(msg.result.InputTokens, msg.result.OutputTokens)
		m.conversation = append(m.conversation, Message{Role: "assistant", Content: msg.result.Text})

		// Show acknowledgment
		m.addOutput("")
		m.addOutput(m.styles.Info.Render("bjarne: ") + stripMarkdown(msg.result.Text))

		// Proceed to generation (user clarifications have been acknowledged)
		m.conversation = append(m.conversation, Message{Role: "user", Content: GenerateNowPrompt})
		return m.startGenerating()

	case generatingDoneMsg:
		if msg.err != nil {
			if m.ctx.Err() == context.Canceled {
				return m, nil
			}
			m.addOutput(m.styles.Error.Render("Generation failed: " + msg.err.Error()))
			m.state = StateInput
			m.textarea.Focus()
			return m, nil
		}
		m.tokenTracker.Add(msg.result.InputTokens, msg.result.OutputTokens)
		m.conversation = append(m.conversation, Message{Role: "assistant", Content: msg.result.Text})

		code := extractCode(msg.result.Text)
		if code == "" {
			// No code extracted - show non-code response parts only
			m.addOutput("")
			cleaned := stripMarkdown(msg.result.Text)
			if cleaned != "" {
				m.addOutput(m.styles.Info.Render("bjarne: ") + cleaned)
			} else {
				m.addOutput(m.styles.Warning.Render("No code block found in response."))
			}
			m.state = StateInput
			m.textarea.Focus()
			return m, nil
		}

		// Code extracted - go straight to validation (don't show code yet)
		m.currentCode = code
		return m.startValidation()

	case validationDoneMsg:
		if msg.err != nil {
			if m.ctx.Err() == context.Canceled {
				return m, nil
			}
			m.addOutput(m.styles.Error.Render("Validation error: " + msg.err.Error()))
			m.state = StateInput
			m.textarea.Focus()
			return m, nil
		}

		allPassed := true
		var failedErrors []string
		for _, r := range msg.results {
			if !r.Success {
				allPassed = false
				if r.Error != "" {
					// Use parsed, compact format for LLM instead of raw stderr
					failedErrors = append(failedErrors, FormatErrorForLLM(r.Stage, r.Error))
				}
			}
		}

		if allPassed {
			m.validated = true
			m.analyzed = false // Reset for next prompt
			m.resetEscalation()

			// Start animated code reveal
			totalTime := m.showValidationSuccess(msg.results)
			m.revealTotalTime = totalTime
			m.revealLines = strings.Split(m.currentCode, "\n")
			m.revealCurrentLine = 0
			m.state = StateRevealing

			// Start the reveal animation
			return m, func() tea.Msg {
				return codeRevealMsg{
					lines:       m.revealLines,
					currentLine: 0,
				}
			}
		}

		// Validation failed - check if escalation is enabled and we can retry
		m.lastValidationErrs = strings.Join(failedErrors, "\n")

		canRetry := m.config.EscalateOnFailure && m.canEscalate()
		m.showValidationFailure(msg.results, !canRetry) // isFinal = !canRetry

		if canRetry {
			return m.startFix()
		}

		// No more escalation possible
		m.showEscalationExhausted()
		m.resetEscalation()
		m.state = StateInput
		m.textarea.Focus()
		return m, textarea.Blink

	case fixDoneMsg:
		if msg.err != nil {
			if m.ctx.Err() == context.Canceled {
				return m, nil
			}
			m.addOutput(m.styles.Error.Render("Fix generation failed: " + msg.err.Error()))
			m.state = StateInput
			m.textarea.Focus()
			return m, nil
		}
		m.tokenTracker.Add(msg.result.InputTokens, msg.result.OutputTokens)
		m.conversation = append(m.conversation, Message{Role: "assistant", Content: msg.result.Text})

		code := extractCode(msg.result.Text)
		if code == "" {
			m.addOutput(m.styles.Warning.Render("No code in fix response, retrying..."))
			if m.canEscalate() {
				return m.startFix()
			}
			m.showEscalationExhausted()
			m.resetEscalation()
			m.state = StateInput
			m.textarea.Focus()
			return m, nil
		}

		m.currentCode = code
		return m.startValidation()

	case tickMsg:
		// Update elapsed time display
		return m, tea.Tick(time.Second, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})

	case codeRevealMsg:
		// Reveal next line of code
		if msg.currentLine < len(msg.lines) {
			m.addOutput(msg.lines[msg.currentLine])
			m.revealCurrentLine = msg.currentLine + 1 // Update progress for View
			// Continue with next line after short delay
			return m, tea.Tick(time.Millisecond*30, func(t time.Time) tea.Msg {
				return codeRevealMsg{
					lines:       msg.lines,
					currentLine: msg.currentLine + 1,
				}
			})
		}
		// All lines revealed, send done message
		return m, func() tea.Msg { return codeRevealDoneMsg{} }

	case codeRevealDoneMsg:
		// Animation complete - show footer and return to input
		m.addOutput("```")
		m.addOutput("")
		m.addOutput(fmt.Sprintf("Total validation time: %s", m.styles.Dim.Render(fmt.Sprintf("%.2fs", m.revealTotalTime))))
		m.addOutput(fmt.Sprintf("Use %s to save", m.styles.Accent.Render("/save <filename>")))
		m.state = StateInput
		m.textarea.Focus()
		return m, textarea.Blink
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	var b strings.Builder

	// Only show current input/status line (output is printed directly to stdout)
	switch m.state {
	case StateInput:
		b.WriteString(m.styles.Prompt.Render(">") + " ")
		b.WriteString(m.textarea.View())

	case StateClassifying, StateThinking, StateAcknowledging, StateGenerating, StateValidating, StateFixing:
		elapsed := time.Since(m.startTime).Seconds()
		status := fmt.Sprintf("esc to interrupt · %.0fs", elapsed)
		if m.tokenCount > 0 {
			status += fmt.Sprintf(" · ↓ %d tokens", m.tokenCount)
		}
		b.WriteString(m.styles.Accent.Render(m.spinner.View()) + " ")
		b.WriteString(m.statusMsg + " ")
		b.WriteString(m.styles.Dim.Render("(" + status + ")"))

	case StateRevealing:
		progress := fmt.Sprintf("%d/%d lines", m.revealCurrentLine, len(m.revealLines))
		b.WriteString(m.styles.Success.Render("▌ ") + "Revealing code... ")
		b.WriteString(m.styles.Dim.Render("(" + progress + ")"))
	}

	return b.String()
}

// Helper methods

func (m *Model) addOutput(line string) {
	// Print directly to stdout for permanent history (scrollback)
	fmt.Println(line)
}

// drawBox creates a bordered box with a title
func (m *Model) drawBox(title string, width int) {
	// Calculate inner width (excluding the border characters)
	innerWidth := width
	titleLen := len(title)

	// If title is longer than width, expand the box
	if titleLen > innerWidth {
		innerWidth = titleLen + 4 // Add some padding
	}

	// Calculate padding for centering
	totalPadding := innerWidth - titleLen
	leftPad := totalPadding / 2
	rightPad := totalPadding - leftPad

	// Draw box
	m.addOutput(m.styles.Warning.Render(boxTopLeft + strings.Repeat(boxHorizontal, innerWidth) + boxTopRight))
	m.addOutput(m.styles.Warning.Render(boxVertical + strings.Repeat(" ", leftPad) + title + strings.Repeat(" ", rightPad) + boxVertical))
	m.addOutput(m.styles.Warning.Render(boxBottomLeft + strings.Repeat(boxHorizontal, innerWidth) + boxBottomRight))
}

func (m *Model) startClassifying(prompt string) (Model, tea.Cmd) {
	m.state = StateClassifying
	m.statusMsg = "Classifying complexity…"
	m.startTime = time.Now()
	m.tokenCount = 0

	// Store original prompt and parse example tests
	m.originalPrompt = prompt
	m.examples = ParseExampleTests(prompt)

	// Show the request
	m.addOutput("")
	m.addOutput(m.styles.Info.Render("Request: ") + fmt.Sprintf("%q", prompt))

	// If we found example tests, show them
	if m.examples != nil && len(m.examples.Tests) > 0 {
		m.addOutput("")
		m.addOutput(m.styles.Success.Render(fmt.Sprintf("Found %d example test(s) - will validate against them", len(m.examples.Tests))))
	}

	m.addOutput("")

	// Add user message to conversation
	m.conversation = append(m.conversation, Message{Role: "user", Content: prompt})

	// Create cancelable context
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancelFn = cancel

	return *m, tea.Batch(
		m.spinner.Tick,
		m.doClassification(ctx),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

func (m *Model) doClassification(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		// Quick classification with Haiku
		result, err := m.bedrock.GenerateWithModel(ctx, m.config.ReflectionModel, ClassificationPrompt, m.conversation, 50)
		return classificationDoneMsg{result: result, err: err}
	}
}

// getModelForComplexity returns the appropriate model based on task complexity
func (m *Model) getModelForComplexity(difficulty string) string {
	switch difficulty {
	case "EASY":
		return "global.anthropic.claude-haiku-4-5-20251001-v1:0"
	case "MEDIUM":
		return "global.anthropic.claude-sonnet-4-5-20250929-v1:0"
	case "COMPLEX":
		return m.config.OracleModel // Opus
	default:
		return "global.anthropic.claude-sonnet-4-5-20250929-v1:0" // Default to Sonnet
	}
}

func (m *Model) startThinking(model string) (Model, tea.Cmd) {
	m.state = StateThinking
	m.statusMsg = "Analyzing…"
	m.startTime = time.Now()
	m.tokenCount = 0

	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancelFn = cancel

	return *m, tea.Batch(
		m.spinner.Tick,
		m.doThinking(ctx, model),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

func (m *Model) doThinking(ctx context.Context, model string) tea.Cmd {
	return func() tea.Msg {
		result, err := m.bedrock.GenerateWithModel(ctx, model, ReflectionSystemPrompt, m.conversation, m.config.MaxTokens)
		return thinkingDoneMsg{result: result, err: err}
	}
}

func (m *Model) startAcknowledging() (Model, tea.Cmd) {
	m.state = StateAcknowledging
	m.statusMsg = "Processing response..."
	m.startTime = time.Now()
	m.tokenCount = 0

	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancelFn = cancel

	return *m, tea.Batch(
		m.spinner.Tick,
		m.doAcknowledging(ctx),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

func (m *Model) doAcknowledging(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		result, err := m.bedrock.GenerateWithModel(ctx, m.config.ChatModel, AcknowledgeSystemPrompt, m.conversation, m.config.MaxTokens)
		return acknowledgeDoneMsg{result: result, err: err}
	}
}

func (m *Model) startGenerating() (Model, tea.Cmd) {
	m.state = StateGenerating

	// Use model based on complexity (EASY=Haiku, MEDIUM=Sonnet, COMPLEX=Opus)
	model := m.getModelForComplexity(m.difficulty)

	m.statusMsg = fmt.Sprintf("Generating code with %s…", shortModelName(model))
	m.startTime = time.Now()
	m.tokenCount = 0

	// Reset escalation state for fresh generation cycle
	m.resetEscalation()

	m.addOutput("")
	m.addOutput(m.styles.Info.Render("Starting code generation..."))
	m.addOutput(fmt.Sprintf("   Model: %s", m.styles.Accent.Render(shortModelName(model))))

	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancelFn = cancel

	return *m, tea.Batch(
		m.spinner.Tick,
		m.doGenerating(ctx, model),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

func (m *Model) doGenerating(ctx context.Context, model string) tea.Cmd {
	return func() tea.Msg {
		result, err := m.bedrock.GenerateWithModel(ctx, model, GenerationSystemPrompt, m.conversation, m.config.MaxTokens)
		return generatingDoneMsg{result: result, err: err}
	}
}

func (m *Model) startValidation() (Model, tea.Cmd) {
	m.state = StateValidating
	m.statusMsg = "Running validation gates…"
	m.startTime = time.Now()

	m.addOutput("")
	m.addOutput(m.styles.Info.Render("Validating code..."))

	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancelFn = cancel

	return *m, tea.Batch(
		m.spinner.Tick,
		m.doValidation(ctx),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

func (m *Model) doValidation(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		// Use full validation with examples and DoD
		results, err := m.container.ValidateCodeWithExamples(ctx, m.currentCode, "code.cpp", m.examples, m.dod)
		return validationDoneMsg{results: results, err: err}
	}
}

// Escalation helper methods

// resetEscalation resets escalation state for a new generation cycle
func (m *Model) resetEscalation() {
	m.currentIteration = 0
	m.currentModelIndex = -1
	m.lastValidationErrs = ""
	m.modelsUsed = nil
}

// canEscalate checks if we can attempt another fix
func (m *Model) canEscalate() bool {
	// Try 2 times with each model before escalating
	maxPerModel := 2

	// Build escalation chain based on complexity
	var escalationChain []string
	switch m.difficulty {
	case "EASY":
		escalationChain = []string{
			"global.anthropic.claude-sonnet-4-5-20250929-v1:0",
			m.config.OracleModel,
		}
	case "MEDIUM":
		escalationChain = []string{
			m.config.OracleModel,
		}
	case "COMPLEX":
		// Already at Opus, no escalation - just retry with Opus
		escalationChain = []string{}
	default:
		escalationChain = m.config.EscalationModels
	}

	// If we're still on the generate model (index -1)
	if m.currentModelIndex == -1 {
		if m.currentIteration < maxPerModel {
			return true
		}
		// Try to escalate to first escalation model
		if len(escalationChain) > 0 {
			return true
		}
		return false
	}

	// Check if we're past the last escalation model
	if m.currentModelIndex >= len(escalationChain) {
		return false
	}

	// We're on a valid escalation model
	if m.currentIteration < maxPerModel {
		return true
	}

	// Try to escalate to next model
	return m.currentModelIndex+1 < len(escalationChain)
}

// getCurrentModel returns the current model to use for fixes
// Uses the complexity-based model as the starting point, then escalates
func (m *Model) getCurrentModel() string {
	// Get the base model based on complexity (same as generation)
	baseModel := m.getModelForComplexity(m.difficulty)

	if m.currentModelIndex == -1 {
		// First fix attempts use same model as generation
		return baseModel
	}

	// Build escalation chain starting from the base model
	// For COMPLEX (Opus), no escalation possible - already at top
	// For MEDIUM (Sonnet), escalate to Opus
	// For EASY (Haiku), escalate to Sonnet then Opus
	var escalationChain []string
	switch m.difficulty {
	case "EASY":
		escalationChain = []string{
			"global.anthropic.claude-sonnet-4-5-20250929-v1:0",
			m.config.OracleModel, // Opus
		}
	case "MEDIUM":
		escalationChain = []string{
			m.config.OracleModel, // Opus
		}
	case "COMPLEX":
		// Already at Opus, no escalation
		escalationChain = []string{}
	default:
		escalationChain = m.config.EscalationModels
	}

	if m.currentModelIndex < len(escalationChain) {
		return escalationChain[m.currentModelIndex]
	}
	return baseModel
}

// advanceEscalation moves to the next iteration/model
func (m *Model) advanceEscalation() {
	maxPerModel := 2

	m.currentIteration++

	// Check if we need to escalate to a more powerful model
	if m.currentIteration >= maxPerModel {
		m.currentIteration = 0
		m.currentModelIndex++
	}
}

func (m *Model) startFix() (Model, tea.Cmd) {
	m.advanceEscalation()

	currentModel := m.getCurrentModel()
	modelName := shortModelName(currentModel)

	// Track which models we've used
	if len(m.modelsUsed) == 0 || m.modelsUsed[len(m.modelsUsed)-1] != modelName {
		m.modelsUsed = append(m.modelsUsed, modelName)
	}

	// Display attempt number (currentIteration is already 1-based after advanceEscalation)
	attemptNum := m.currentIteration
	if attemptNum == 0 {
		attemptNum = 1 // After escalation, iteration resets to 0 but this is attempt 1 with new model
	}

	m.state = StateFixing
	m.statusMsg = fmt.Sprintf("Fixing with %s (attempt %d)…", modelName, attemptNum)
	m.startTime = time.Now()
	m.tokenCount = 0

	m.addOutput("")
	m.addOutput(m.styles.Warning.Render(fmt.Sprintf("Attempting fix with %s (attempt %d)...", modelName, attemptNum)))

	// Show the errors being sent to the LLM
	m.addOutput(m.styles.Dim.Render("Errors to fix:"))
	for _, line := range strings.Split(m.lastValidationErrs, "\n") {
		if line != "" {
			m.addOutput(m.styles.Dim.Render("  " + line))
		}
	}

	// Add fix request to conversation
	fixPrompt := fmt.Sprintf(IterationPromptTemplate, m.lastValidationErrs)
	m.conversation = append(m.conversation, Message{Role: "user", Content: fixPrompt})

	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancelFn = cancel

	return *m, tea.Batch(
		m.spinner.Tick,
		m.doFix(ctx, currentModel),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

func (m *Model) doFix(ctx context.Context, model string) tea.Cmd {
	return func() tea.Msg {
		result, err := m.bedrock.GenerateWithModel(ctx, model, GenerationSystemPrompt, m.conversation, m.config.MaxTokens)
		return fixDoneMsg{result: result, err: err}
	}
}

func (m *Model) showEscalationExhausted() {
	m.addOutput("")
	m.addOutput(m.styles.Error.Render("All fix attempts exhausted."))
	if len(m.modelsUsed) > 0 {
		m.addOutput(m.styles.Dim.Render(fmt.Sprintf("Models tried: %s", strings.Join(m.modelsUsed, " → "))))
	}
	m.addOutput("")
	m.addOutput("You can refine your request or ask bjarne to fix specific issues.")
}

func (m *Model) showValidationSuccess(results []ValidationResult) float64 {
	// Show gate results in tree style
	totalTime := 0.0
	for i, r := range results {
		totalTime += r.Duration.Seconds()
		prefix := treeBranch
		if i == len(results)-1 {
			prefix = treeEnd
		}
		m.addOutput(fmt.Sprintf("  %s Gate %d: %s...", prefix, i+1, r.Stage))
		m.addOutput(fmt.Sprintf("  %s  %s %s", treeVert, m.styles.Success.Render("PASS"), m.styles.Dim.Render(fmt.Sprintf("(%.2fs)", r.Duration.Seconds()))))
	}

	m.addOutput("")
	m.addOutput(fmt.Sprintf("  %s All validation gates passed", m.styles.Success.Render(">>")))
	m.addOutput("")

	// Success box header
	m.addOutput(strings.Repeat("=", 80))
	m.addOutput(m.styles.Success.Render("SUCCESS! Validated code:"))
	m.addOutput(strings.Repeat("=", 80))
	m.addOutput("```cpp")

	// Return total time - animation will handle the rest
	return totalTime
}

func (m *Model) showValidationFailure(results []ValidationResult, isFinal bool) {
	// Show gate results in compact form
	for _, r := range results {
		if r.Success {
			m.addOutput(fmt.Sprintf("  %s %s", m.styles.Success.Render("✓"), r.Stage))
		} else {
			m.addOutput(fmt.Sprintf("  %s %s", m.styles.Error.Render("✗"), r.Stage))
		}
	}

	if !isFinal {
		// Not final - will retry, don't show code
		m.addOutput("")
		m.addOutput(m.styles.Warning.Render("Validation failed, refactoring..."))
		return
	}

	// Final failure - show code
	m.addOutput("")
	m.addOutput(strings.Repeat("=", 80))
	m.addOutput(m.styles.Error.Render("FAILED! Validation did not pass."))
	m.addOutput(strings.Repeat("=", 80))
	m.addOutput("")
	m.addOutput(m.styles.Warning.Render("Generated code (failed validation):"))

	// Show full code
	m.addOutput("```cpp")
	m.addOutput(m.currentCode)
	m.addOutput("```")
	m.addOutput("")
	m.addOutput("You can refine your request or ask bjarne to fix specific issues.")
}

func (m Model) handleCommand(input string) (Model, tea.Cmd) {
	parts := strings.Fields(input)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/quit", "/exit", "/q":
		return m, tea.Quit

	case "/help", "/h":
		m.addOutput("")
		m.addOutput("Available Commands:")
		m.addOutput("  /help, /h              Show this help")
		m.addOutput("  /save <file>, /s       Save last generated code to file")
		m.addOutput("  /clear, /c             Clear conversation")
		m.addOutput("  /code, /show           Show last generated code")
		m.addOutput("  /tokens, /t            Show token usage")
		m.addOutput("  /quit, /q              Exit bjarne")
		m.addOutput("")

	case "/clear", "/c":
		m.conversation = []Message{}
		m.currentCode = ""
		m.validated = false
		m.analyzed = false
		m.originalPrompt = ""
		m.examples = nil
		m.dod = nil
		m.difficulty = ""
		m.resetEscalation()
		m.tokenTracker.Reset()
		m.addOutput("Conversation cleared.")

	case "/code", "/show":
		if m.currentCode == "" {
			m.addOutput("No code generated yet.")
		} else {
			m.addOutput("")
			m.addOutput(m.styles.Warning.Render("Last generated code:"))
			m.addOutput("```cpp")
			m.addOutput(m.currentCode)
			m.addOutput("```")
		}

	case "/save", "/s":
		if len(parts) < 2 {
			m.addOutput(m.styles.Error.Render("Usage: /save <filename>"))
		} else if m.currentCode == "" {
			m.addOutput(m.styles.Error.Render("No code to save."))
		} else {
			filename := parts[1]
			if err := saveToFile(filename, m.currentCode); err != nil {
				m.addOutput(m.styles.Error.Render("Error saving: " + err.Error()))
			} else {
				m.addOutput(m.styles.Success.Render("Saved to " + filename))
			}
		}

	case "/tokens", "/t":
		input, output, total := m.tokenTracker.GetUsage()
		m.addOutput("")
		m.addOutput(m.styles.Warning.Render("Token Usage:"))
		m.addOutput(fmt.Sprintf("  Input tokens:  %d", input))
		m.addOutput(fmt.Sprintf("  Output tokens: %d", output))
		m.addOutput(fmt.Sprintf("  Total tokens:  %d", total))
		m.addOutput("")

	default:
		m.addOutput(m.styles.Error.Render("Unknown command: " + cmd))
	}

	m.textarea.Reset()
	return m, nil
}

// StartTUI initializes everything and starts the bubbletea TUI
func StartTUI() error {
	ctx := context.Background()

	// Load configuration
	cfg := LoadConfig()

	fmt.Printf("bjarne %s\n", Version)
	fmt.Println("AI-assisted C/C++ code generation with mandatory validation")
	fmt.Println()

	// Initialize container runtime
	fmt.Println("Detecting container runtime...")
	container, err := DetectContainerRuntime()
	if err != nil {
		fmt.Print(FormatUserError(err))
		return err
	}
	fmt.Printf("Using container runtime: %s\n", container.GetBinary())

	// Check if validation image exists
	if !container.ImageExists(ctx) {
		if err := handleFirstRunPull(ctx, container); err != nil {
			fmt.Printf("\033[93mWarning:\033[0m %v\n", err)
			fmt.Println("         Code generation will work, but validation will be skipped.")
		}
	} else {
		fmt.Printf("Validation container: %s [OK]\n", container.imageName)
	}
	fmt.Println()

	// Initialize Bedrock client
	fmt.Println("Connecting to AWS Bedrock...")
	bedrock, err := NewBedrockClient(ctx, cfg.GenerateModel)
	if err != nil {
		fmt.Print(FormatUserError(err))
		return err
	}
	fmt.Printf("Reflection: %s\n", shortModelName(cfg.ReflectionModel))
	fmt.Printf("Generation: %s\n", shortModelName(cfg.GenerateModel))
	fmt.Printf("Oracle: %s\n", shortModelName(cfg.OracleModel))
	if cfg.EscalateOnFailure && len(cfg.EscalationModels) > 0 {
		fmt.Printf("Escalation: enabled → %s\n", shortModelName(cfg.EscalationModels[0]))
	}
	fmt.Println()
	fmt.Println("Type /help for commands, /quit to exit")
	fmt.Println("Press Esc to interrupt during processing")
	fmt.Println()

	m := NewModel(bedrock, container, cfg)
	// Don't use WithAltScreen() - keeps normal terminal scrollback history
	p := tea.NewProgram(m)

	_, err = p.Run()
	return err
}
