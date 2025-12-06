package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	StateFixing    // Attempting to fix failed code
	StateReviewing // LLM code review gate
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
	currentCode    string     // For backwards compatibility and single-file projects
	currentFiles   []CodeFile // Multi-file project support
	validated      bool
	analyzed       bool              // True after first analysis, subsequent inputs go to generation
	originalPrompt string            // Store original prompt to parse examples
	examples       *ExampleTests     // Parsed example tests from prompt
	dod            *DefinitionOfDone // Definition of Done for complex tasks
	difficulty     string            // EASY, MEDIUM, COMPLEX from classification
	intent         string            // NEW, CONTINUE, QUESTION from classification
	savedPath      string            // Path where code was last saved (empty = unsaved)
	historyPath    string            // Path to auto-saved history file

	// Escalation tracking
	currentIteration   int      // Current fix attempt within current model
	currentModelIndex  int      // Index into escalation chain (-1 = generate model)
	totalFixAttempts   int      // Total fix attempts across all models (for display)
	lastValidationErrs string   // Last validation errors for fix prompt
	modelsUsed         []string // Track which models we've tried
	reviewFailures     int      // Count consecutive review failures (max 2 before showing code)

	// Exit confirmation
	ctrlCPressed bool      // True if Ctrl+C was pressed once
	ctrlCTime    time.Time // When Ctrl+C was pressed (for timeout)

	// Code reveal animation
	revealLines       []string // Lines to reveal
	revealCurrentLine int      // Current line being revealed
	revealTotalTime   float64  // Total validation time to show after reveal

	// Session data
	provider       LLMProvider // Abstract LLM provider (Bedrock, Anthropic, OpenAI, Gemini)
	container      *ContainerRuntime
	config         *Config
	tokenTracker   *TokenTracker
	conversation   []Message
	workspaceIndex *WorkspaceIndex // Indexed codebase for context
	vectorIndex    *VectorIndex    // Semantic search index with embeddings
	llmGuard       *LLMGuardClient // Optional LLM security scanner

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

type reviewDoneMsg struct {
	result *GenerateResult
	passed bool
	issues string
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
func NewModel(provider LLMProvider, container *ContainerRuntime, cfg *Config) Model {
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
		provider:     provider,
		container:    container,
		config:       cfg,
		tokenTracker: NewTokenTracker(cfg.MaxTotalTokens, cfg.WarnTokenThreshold),
		conversation: []Message{},
		llmGuard:     NewLLMGuardClient(),
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

				// Handle slash commands
				if strings.HasPrefix(input, "/") {
					return m.handleCommand(input)
				}

				// Handle natural language commands (T-038e)
				if cmd, args, isCmd := parseNaturalCommand(input); isCmd {
					m.textarea.Reset()
					if args != "" {
						return m.handleCommand(cmd + " " + args)
					}
					return m.handleCommand(cmd)
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
			// Classification failed - default to NEW MEDIUM and continue
			m.addOutput(m.styles.Warning.Render("Classification failed, defaulting to NEW MEDIUM"))
			m.intent = "NEW"
			m.difficulty = "MEDIUM"
			return m.startThinking(m.getModelForComplexity("MEDIUM"))
		}

		// Parse the classification result (INTENT COMPLEXITY)
		m.tokenTracker.Add(msg.result.InputTokens, msg.result.OutputTokens)
		classification := strings.TrimSpace(strings.ToUpper(msg.result.Text))
		parts := strings.Fields(classification)

		// Parse intent (first word)
		m.intent = "NEW" // default
		if len(parts) >= 1 {
			switch {
			case strings.Contains(parts[0], "CONTINUE"):
				m.intent = "CONTINUE"
			case strings.Contains(parts[0], "QUESTION"):
				m.intent = "QUESTION"
			default:
				m.intent = "NEW"
			}
		}

		// Parse complexity (second word, or first if only one word for backwards compat)
		m.difficulty = "MEDIUM" // default
		complexityWord := classification
		if len(parts) >= 2 {
			complexityWord = parts[1]
		}
		switch {
		case strings.Contains(complexityWord, "EASY"):
			m.difficulty = "EASY"
		case strings.Contains(complexityWord, "COMPLEX"):
			m.difficulty = "COMPLEX"
		default:
			m.difficulty = "MEDIUM"
		}

		// Show classification result
		m.addOutput("")
		var intentDisplay, diffDisplay string
		switch m.intent {
		case "NEW":
			intentDisplay = m.styles.Info.Render("[NEW]")
		case "CONTINUE":
			intentDisplay = m.styles.Accent.Render("[CONTINUE]")
		case "QUESTION":
			intentDisplay = m.styles.Dim.Render("[QUESTION]")
		}
		switch m.difficulty {
		case "EASY":
			diffDisplay = m.styles.Success.Render("[EASY]")
		case "MEDIUM":
			diffDisplay = m.styles.Warning.Render("[MEDIUM]")
		case "COMPLEX":
			diffDisplay = m.styles.Error.Render("[COMPLEX]")
		}
		m.addOutput(fmt.Sprintf("Intent: %s  Complexity: %s", intentDisplay, diffDisplay))

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

		// Handle based on intent
		if m.intent == "QUESTION" {
			// For questions, the analysis IS the response - no code generation
			m.addOutput("")
			m.addOutput(m.styles.Dim.Render("(Question answered - no code to generate)"))
			m.state = StateInput
			m.textarea.Focus()
			return m, textarea.Blink
		}

		// Auto-proceed conditions (T-038f):
		// - EASY tasks: straightforward, no need to confirm
		// - CONTINUE intent: user is iterating on existing code
		// But only if the analysis doesn't ask clarifying questions
		if (m.difficulty == "EASY" || m.intent == "CONTINUE") && !containsQuestion(reflection) {
			m.conversation = append(m.conversation, Message{Role: "user", Content: GenerateNowPrompt})
			return m.startGenerating()
		}

		// For NEW MEDIUM/COMPLEX: wait for user input before generating
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

		// Check if acknowledgment already contains code (LLM jumped ahead)
		code := extractCode(msg.result.Text)
		if code != "" {
			// LLM already generated code - use it directly, skip generation phase
			// Don't show the text (contains code) - go straight to validation
			m.addOutput("")
			m.addOutput(m.styles.Info.Render("Code received, validating..."))

			// Extract files and go straight to validation
			files := extractMultipleFiles(msg.result.Text)
			if len(files) == 0 {
				// Single file fallback
				files = []CodeFile{{Filename: "code.cpp", Content: code}}
			}
			m.currentFiles = files
			m.currentCode = code
			return m.startValidation()
		}

		// Show acknowledgment (no code - need to generate)
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

		// LLM Guard: Scan generated output for embedded secrets
		if m.llmGuard != nil && m.llmGuard.IsEnabled() {
			scanResult, err := m.llmGuard.ScanOutput(msg.result.Text)
			if err != nil {
				m.addOutput(m.styles.Warning.Render("Output security scan unavailable: ") + err.Error())
			} else if !scanResult.IsValid {
				m.addOutput("")
				m.addOutput(m.styles.Warning.Render("Security scan detected issues in generated code:"))
				m.addOutput(m.llmGuard.FormatSecurityIssues(scanResult))
				m.addOutput(m.styles.Dim.Render("Proceeding with validation - review code carefully."))
			}
		}

		// Extract files (supports both single and multi-file responses)
		files := extractMultipleFiles(msg.result.Text)
		if len(files) == 0 {
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

		// Store files
		m.currentFiles = files
		// For backwards compatibility, also store combined code
		m.currentCode = extractCode(msg.result.Text)

		// Show file count if multi-file
		if len(files) > 1 {
			m.addOutput("")
			m.addOutput(m.styles.Info.Render(fmt.Sprintf("Generated %d files:", len(files))))
			for _, f := range files {
				m.addOutput(fmt.Sprintf("  - %s", f.Filename))
			}
		}

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
			// All sanitizer gates passed - now do LLM code review
			return m.startReviewing(msg.results)
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

	case reviewDoneMsg:
		if msg.err != nil {
			if m.ctx.Err() == context.Canceled {
				return m, nil
			}
			// Review failed but sanitizers passed - show code anyway
			m.addOutput(m.styles.Warning.Render("Code review unavailable: " + msg.err.Error()))
			return m.showValidatedCode()
		}

		if msg.passed {
			// LLM review passed - show the validated code
			m.addOutput(m.styles.Success.Render("  └─ Gate: review... PASS"))
			m.reviewFailures = 0 // Reset on success
			return m.showValidatedCode()
		}

		// LLM review found issues
		m.reviewFailures++
		m.addOutput(m.styles.Error.Render("  └─ Gate: review... FAIL"))
		m.addOutput(m.styles.Dim.Render("     " + msg.issues))

		// Limit review retries to 2 - if sanitizers pass twice but review keeps failing,
		// the review is being too pedantic. Show the code.
		if m.reviewFailures >= 2 {
			m.addOutput("")
			m.addOutput(m.styles.Warning.Render("Review keeps failing but sanitizers pass. Code is likely fine."))
			m.addOutput(m.styles.Dim.Render("(Sanitizers validated memory safety, data races, and undefined behavior)"))
			return m.showValidatedCode()
		}

		m.lastValidationErrs = "Code review: " + msg.issues

		// Try to fix if we can escalate
		if m.config.EscalateOnFailure && m.canEscalate() {
			m.addOutput("")
			m.addOutput("Review failed, refactoring...")
			return m.startFix()
		}

		// Can't escalate - show partial success
		m.addOutput("")
		m.addOutput(m.styles.Warning.Render("Code passed sanitizers but failed review. Showing code anyway."))
		return m.showValidatedCode()

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
		if m.historyPath != "" {
			m.addOutput(fmt.Sprintf("Auto-saved to: %s", m.styles.Dim.Render(m.historyPath)))
		}
		m.addOutput(fmt.Sprintf("Use %s to save to working directory", m.styles.Accent.Render("/save <filename>")))
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
		// Show unsaved indicator if there's validated code not yet saved
		if m.hasUnsavedCode() {
			b.WriteString(m.styles.Warning.Render("[*] "))
		}
		b.WriteString(m.styles.Prompt.Render(">") + " ")
		b.WriteString(m.textarea.View())

	case StateClassifying, StateThinking, StateAcknowledging, StateGenerating, StateValidating, StateFixing, StateReviewing:
		elapsed := time.Since(m.startTime).Seconds()
		status := fmt.Sprintf("esc to interrupt · %.0fs", elapsed)
		if m.tokenCount > 0 {
			status += fmt.Sprintf(" · ↓ %d tokens", m.tokenCount)
		}
		b.WriteString(m.styles.Accent.Render(m.spinner.View()) + " ")
		b.WriteString(m.statusMsg + " ")
		b.WriteString(m.styles.Dim.Render("(" + status + ")"))

	case StateRevealing:
		// Don't show progress - the scrolling code is visual feedback
		// Showing progress here causes display overlap with addOutput()
		b.WriteString("")
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

	// LLM Guard: Scan prompt for security issues (prompt injection, secrets, toxicity)
	if m.llmGuard != nil && m.llmGuard.IsEnabled() {
		scanResult, err := m.llmGuard.ScanPrompt(prompt)
		if err != nil {
			m.addOutput("")
			m.addOutput(m.styles.Warning.Render("Security scan unavailable: ") + err.Error())
		} else if !scanResult.IsValid {
			// Prompt failed security scan - reject it
			m.addOutput("")
			m.addOutput(m.styles.Error.Render("Security scan blocked this request:"))
			m.addOutput(m.llmGuard.FormatSecurityIssues(scanResult))
			m.addOutput("")
			m.addOutput("Please rephrase your request without sensitive content.")
			m.state = StateInput
			return *m, nil
		}
	}

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
		result, err := m.provider.Generate(ctx, m.config.ReflectionModel, ClassificationPrompt, m.conversation, 50)
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
	if m.intent == "QUESTION" {
		m.statusMsg = "Answering…"
	} else {
		m.statusMsg = "Analyzing…"
	}
	m.startTime = time.Now()
	m.tokenCount = 0

	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancelFn = cancel

	return *m, tea.Batch(
		m.spinner.Tick,
		m.doThinking(ctx, model, m.intent),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

func (m *Model) doThinking(ctx context.Context, model string, intent string) tea.Cmd {
	return func() tea.Msg {
		// Use appropriate prompt based on intent
		systemPrompt := ReflectionSystemPrompt
		if intent == "QUESTION" {
			systemPrompt = QuestionSystemPrompt
		}
		result, err := m.provider.Generate(ctx, model, systemPrompt, m.conversation, m.config.MaxTokens)
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
		result, err := m.provider.Generate(ctx, m.config.ChatModel, AcknowledgeSystemPrompt, m.conversation, m.config.MaxTokens)
		return acknowledgeDoneMsg{result: result, err: err}
	}
}

func (m *Model) startGenerating() (Model, tea.Cmd) {
	m.state = StateGenerating

	// Use model based on complexity (EASY=Haiku, MEDIUM=Sonnet, COMPLEX=Opus)
	model := m.getModelForComplexity(m.difficulty)

	m.statusMsg = "Generating code…"
	m.startTime = time.Now()
	m.tokenCount = 0

	// Reset escalation state for fresh generation cycle
	m.resetEscalation()

	m.addOutput("")
	m.addOutput(m.styles.Info.Render("Generating code..."))

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
		systemPrompt := m.buildSystemPrompt()
		result, err := m.provider.Generate(ctx, model, systemPrompt, m.conversation, m.config.MaxTokens)
		return generatingDoneMsg{result: result, err: err}
	}
}

// buildSystemPrompt creates the system prompt, including workspace context if indexed
func (m *Model) buildSystemPrompt() string {
	prompt := GenerationSystemPrompt

	// Try semantic search with vector index first (better context)
	if m.vectorIndex != nil && len(m.conversation) > 0 {
		// Use the last user message as the query
		var query string
		for i := len(m.conversation) - 1; i >= 0; i-- {
			if m.conversation[i].Role == "user" {
				query = m.conversation[i].Content
				break
			}
		}

		if query != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Retrieve up to 20 relevant chunks (similar to Cody)
			chunks, err := m.vectorIndex.SearchSimilar(ctx, query, 20)
			if err == nil && len(chunks) > 0 {
				var contextBuilder strings.Builder
				contextBuilder.WriteString("<relevant_code_context>\n")
				contextBuilder.WriteString("The following code from the project is semantically relevant to the request.\n")
				contextBuilder.WriteString("Use these patterns and styles when generating code:\n\n")

				// Track total size to avoid exceeding token limits (~8000 chars ≈ 2000 tokens)
				const maxContextChars = 8000
				totalChars := 0

				for i, chunk := range chunks {
					// Get file path from chunk
					filePath := m.getChunkFilePath(chunk.FileID)

					header := fmt.Sprintf("// [%d] %s::%s (%s)\n", i+1, filePath, chunk.Name, chunk.Type)
					content := chunk.Content

					// Check if adding this chunk would exceed limit
					chunkSize := len(header) + len(content) + 10
					if totalChars+chunkSize > maxContextChars && totalChars > 0 {
						contextBuilder.WriteString(fmt.Sprintf("\n// ... and %d more relevant chunks (truncated for context window)\n", len(chunks)-i))
						break
					}

					contextBuilder.WriteString(header)
					contextBuilder.WriteString(content)
					contextBuilder.WriteString("\n\n")
					totalChars += chunkSize
				}
				contextBuilder.WriteString("</relevant_code_context>\n")

				prompt += "\n\n" + contextBuilder.String()
				prompt += "\nFollow the patterns and conventions shown in the relevant code context above."
				return prompt
			}
		}
	}

	// Fall back to workspace index (structural context)
	if m.workspaceIndex != nil && len(m.workspaceIndex.Files) > 0 {
		context := m.workspaceIndex.GetContextForPrompt(2000) // ~2000 tokens max
		if context != "" {
			prompt += "\n\n" + context + "\n\nIntegrate with the existing codebase where appropriate."
		}
	}

	return prompt
}

// getChunkFilePath retrieves the file path for a chunk's file ID
func (m *Model) getChunkFilePath(fileID int64) string {
	if m.vectorIndex == nil {
		return "unknown"
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	path, err := m.vectorIndex.GetFilePath(ctx, fileID)
	if err != nil {
		return "unknown"
	}
	return path
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
		var results []ValidationResult
		var err error

		// Use multi-file validation if we have multiple files
		if len(m.currentFiles) > 1 {
			results, err = m.container.ValidateMultiFileCodeWithExamples(ctx, m.currentFiles, m.examples, m.dod)
		} else {
			// Single file validation (backwards compatible)
			results, err = m.container.ValidateCodeWithExamples(ctx, m.currentCode, "code.cpp", m.examples, m.dod)
		}
		return validationDoneMsg{results: results, err: err}
	}
}

// startReviewing initiates the LLM code review gate
func (m *Model) startReviewing(results []ValidationResult) (Model, tea.Cmd) {
	m.state = StateReviewing
	m.statusMsg = "Code review…"
	m.startTime = time.Now()

	// Show sanitizer gate results first
	m.showValidationSuccess(results)
	m.addOutput(m.styles.Info.Render("  ├─ Gate: review..."))

	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancelFn = cancel

	return *m, tea.Batch(
		m.spinner.Tick,
		m.doReview(ctx),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

// doReview performs the LLM code review
func (m *Model) doReview(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		// Build review prompt with original request and generated code
		reviewPrompt := fmt.Sprintf(CodeReviewPrompt, m.originalPrompt, m.currentCode)

		// Use Haiku for fast review (it's a simple pass/fail check)
		result, err := m.provider.Generate(ctx, m.config.ReflectionModel, "", []Message{
			{Role: "user", Content: reviewPrompt},
		}, 200)

		if err != nil {
			return reviewDoneMsg{err: err}
		}

		response := strings.TrimSpace(result.Text)
		passed := strings.HasPrefix(strings.ToUpper(response), "PASS")
		issues := ""
		if !passed && strings.HasPrefix(strings.ToUpper(response), "FAIL:") {
			issues = strings.TrimPrefix(response, "FAIL:")
			issues = strings.TrimPrefix(issues, "Fail:")
			issues = strings.TrimPrefix(issues, "fail:")
			issues = strings.TrimSpace(issues)
		} else if !passed {
			issues = response // Use full response as issues if no FAIL: prefix
		}

		return reviewDoneMsg{result: result, passed: passed, issues: issues}
	}
}

// showValidatedCode displays the final validated code and transitions to reveal
func (m *Model) showValidatedCode() (Model, tea.Cmd) {
	m.validated = true
	m.analyzed = false // Reset for next prompt
	m.savedPath = ""   // Reset saved state for new code
	m.resetEscalation()

	// Auto-save to history
	m.historyPath = m.autoSaveToHistory()

	m.addOutput("")
	m.addOutput(m.styles.Success.Render("  >> All validation gates passed"))

	// Build reveal lines with file separators for multi-file projects
	m.revealLines = m.buildRevealLines()
	m.revealCurrentLine = 0
	m.state = StateRevealing

	// Start the reveal animation
	return *m, func() tea.Msg {
		return codeRevealMsg{
			lines:       m.revealLines,
			currentLine: 0,
		}
	}
}

// Escalation helper methods

// resetEscalation resets escalation state for a new generation cycle
func (m *Model) resetEscalation() {
	m.currentIteration = 0
	m.currentModelIndex = -1
	m.totalFixAttempts = 0
	m.lastValidationErrs = ""
	m.modelsUsed = nil
	m.reviewFailures = 0
}

// canEscalate checks if we can attempt another fix
func (m *Model) canEscalate() bool {
	// Maximum total fix attempts across all models
	const maxTotalAttempts = 15

	return m.totalFixAttempts < maxTotalAttempts
}

// getCurrentModel returns the current model to use for fixes
// Escalates to more powerful models after several attempts
func (m *Model) getCurrentModel() string {
	// Escalation thresholds: try base model first, then escalate
	// Attempts 1-5: base model (based on complexity)
	// Attempts 6-10: Sonnet (if not already)
	// Attempts 11-15: Opus

	baseModel := m.getModelForComplexity(m.difficulty)
	sonnet := "global.anthropic.claude-sonnet-4-5-20250929-v1:0"
	opus := m.config.OracleModel

	if m.totalFixAttempts <= 5 {
		return baseModel
	} else if m.totalFixAttempts <= 10 {
		// Escalate to at least Sonnet
		if m.difficulty == "EASY" {
			return sonnet
		}
		return baseModel // MEDIUM/COMPLEX already at Sonnet or Opus
	} else {
		// Final escalation to Opus
		return opus
	}
}

// advanceEscalation increments the fix attempt counter
func (m *Model) advanceEscalation() {
	m.totalFixAttempts++
}

func (m *Model) startFix() (Model, tea.Cmd) {
	m.advanceEscalation()

	currentModel := m.getCurrentModel()

	m.state = StateFixing
	m.statusMsg = fmt.Sprintf("Fixing code (attempt %d/15)…", m.totalFixAttempts)
	m.startTime = time.Now()
	m.tokenCount = 0

	// Add fix request to conversation with current code and errors
	fixPrompt := fmt.Sprintf(IterationPromptTemplate, m.currentCode, m.lastValidationErrs)
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
		systemPrompt := m.buildSystemPrompt()
		result, err := m.provider.Generate(ctx, model, systemPrompt, m.conversation, m.config.MaxTokens)
		return fixDoneMsg{result: result, err: err}
	}
}

func (m *Model) showEscalationExhausted() {
	m.addOutput("")
	m.addOutput(m.styles.Error.Render("All fix attempts exhausted."))
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

	// Show full code (multi-file aware)
	if len(m.currentFiles) > 1 {
		for _, f := range m.currentFiles {
			m.addOutput("")
			m.addOutput(m.styles.Info.Render(fmt.Sprintf("// === %s ===", f.Filename)))
			m.addOutput("```cpp")
			m.addOutput(f.Content)
			m.addOutput("```")
		}
	} else {
		m.addOutput("```cpp")
		m.addOutput(m.currentCode)
		m.addOutput("```")
	}
	m.addOutput("")
	m.addOutput("You can refine your request or ask bjarne to fix specific issues.")
}

// buildRevealLines creates the lines to reveal, with file separators for multi-file projects
func (m *Model) buildRevealLines() []string {
	if len(m.currentFiles) <= 1 {
		// Single file - just split by lines
		return strings.Split(m.currentCode, "\n")
	}

	// Multi-file project - add file headers
	var lines []string
	for i, f := range m.currentFiles {
		if i > 0 {
			lines = append(lines, "```")
			lines = append(lines, "")
		}
		lines = append(lines, m.styles.Info.Render(fmt.Sprintf("// === %s ===", f.Filename)))
		if i > 0 {
			lines = append(lines, "```cpp")
		}
		lines = append(lines, strings.Split(f.Content, "\n")...)
	}
	return lines
}

// autoSaveToHistory saves validated code to ~/.bjarne/history/ with timestamp
func (m *Model) autoSaveToHistory() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	historyDir := filepath.Join(homeDir, ".bjarne", "history")
	if err := os.MkdirAll(historyDir, 0750); err != nil {
		return ""
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("2006-01-02_150405")
	var filename string

	if len(m.currentFiles) > 1 {
		// Multi-file: save as directory
		dirName := fmt.Sprintf("%s_project", timestamp)
		dirPath := filepath.Join(historyDir, dirName)
		if err := os.MkdirAll(dirPath, 0750); err != nil {
			return ""
		}
		for _, f := range m.currentFiles {
			filePath := filepath.Join(dirPath, f.Filename)
			_ = os.WriteFile(filePath, []byte(f.Content), 0600)
		}
		return dirPath
	}

	// Single file
	filename = fmt.Sprintf("%s.cpp", timestamp)
	filePath := filepath.Join(historyDir, filename)
	if err := os.WriteFile(filePath, []byte(m.currentCode), 0600); err != nil {
		return ""
	}
	return filePath
}

// hasUnsavedCode returns true if there's validated code that hasn't been explicitly saved
func (m *Model) hasUnsavedCode() bool {
	return m.validated && m.savedPath == ""
}

// parseNaturalCommand converts natural language to commands
// Returns (command, args, isCommand)
func parseNaturalCommand(input string) (string, string, bool) {
	lower := strings.ToLower(input)

	// "save as <filename>" or "save to <filename>"
	if strings.HasPrefix(lower, "save as ") {
		return "/save", strings.TrimPrefix(input, input[:8]), true
	}
	if strings.HasPrefix(lower, "save to ") {
		return "/save", strings.TrimPrefix(input, input[:8]), true
	}
	if lower == "save" || lower == "save it" || lower == "save this" {
		return "/save", "", true
	}

	// "start fresh" or "start over" or "new task"
	if lower == "start fresh" || lower == "start over" || lower == "new task" || lower == "clear" {
		return "/clear", "", true
	}

	// "show code" or "show the code"
	if lower == "show code" || lower == "show the code" || lower == "show it" {
		return "/code", "", true
	}

	// "quit" or "exit"
	if lower == "quit" || lower == "exit" || lower == "bye" {
		return "/quit", "", true
	}

	return "", "", false
}

func (m Model) handleCommand(input string) (Model, tea.Cmd) {
	parts := strings.Fields(input)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/quit", "/exit", "/q":
		return m, tea.Quit

	case "/help", "/h":
		m.addOutput("")
		m.addOutput("Commands:")
		m.addOutput("  /help, /h              Show this help")
		m.addOutput("  /init                  Index current directory for context-aware generation")
		m.addOutput("  /save [file|dir], /s   Save code (multi-file: /save dir/ or /save)")
		m.addOutput("  /clear, /c             Clear conversation and start fresh")
		m.addOutput("  /code, /show           Show last generated code")
		m.addOutput("  /tokens, /t            Show token usage")
		m.addOutput("  /quit, /q              Exit bjarne")
		m.addOutput("")
		m.addOutput("Natural Language:")
		m.addOutput("  \"save as <file>\"       Same as /save <file>")
		m.addOutput("  \"start fresh\"          Same as /clear")
		m.addOutput("  \"show code\"            Same as /code")
		m.addOutput("")
		m.addOutput("Indicators:")
		m.addOutput("  [*] >                  Unsaved validated code (auto-saved to ~/.bjarne/history/)")
		m.addOutput("")

	case "/init":
		m.addOutput("")
		m.addOutput(m.styles.Warning.Render("Indexing workspace..."))

		cwd, err := os.Getwd()
		if err != nil {
			m.addOutput(m.styles.Error.Render("Error: " + err.Error()))
			break
		}

		// Try to load existing index first
		existingIndex, err := LoadIndex(cwd)
		if err == nil {
			m.addOutput(m.styles.Dim.Render(fmt.Sprintf("Found existing index (%d files)", existingIndex.Summary.TotalFiles)))
			m.addOutput("Re-indexing...")
		}

		// Index the workspace (structural)
		fileCount := 0
		index, err := IndexWorkspace(cwd, func(path string) {
			fileCount++
			if fileCount%10 == 0 {
				m.addOutput(m.styles.Dim.Render(fmt.Sprintf("  Scanned %d files...", fileCount)))
			}
		})

		if err != nil {
			m.addOutput(m.styles.Error.Render("Indexing failed: " + err.Error()))
			break
		}

		// Save the structural index
		if err := SaveIndex(index, cwd); err != nil {
			m.addOutput(m.styles.Error.Render("Failed to save index: " + err.Error()))
			break
		}

		m.workspaceIndex = index
		m.addOutput("")
		m.addOutput(m.styles.Success.Render("✓ Workspace indexed!"))
		m.addOutput(fmt.Sprintf("  Files:     %d", index.Summary.TotalFiles))
		m.addOutput(fmt.Sprintf("  Functions: %d", index.Summary.TotalFunctions))
		m.addOutput(fmt.Sprintf("  Classes:   %d", index.Summary.TotalClasses))
		m.addOutput(fmt.Sprintf("  Structs:   %d", index.Summary.TotalStructs))
		m.addOutput(fmt.Sprintf("  Lines:     %d", index.Summary.TotalLines))
		m.addOutput("")
		m.addOutput(m.styles.Dim.Render("Saved to " + IndexFileName))

		// Build vector index for semantic search
		m.addOutput("")
		m.addOutput(m.styles.Warning.Render("Building semantic index..."))

		cfg := DefaultVectorIndexConfig()
		vecIndex, err := NewVectorIndex(cfg)
		if err != nil {
			m.addOutput(m.styles.Error.Render("Vector index failed: " + err.Error()))
			m.addOutput(m.styles.Info.Render("Structural index will still be used for context."))
			break
		}

		// Download model if needed
		ctx := context.Background()
		if err := vecIndex.EnsureModel(ctx, func(msg string) {
			m.addOutput(m.styles.Dim.Render("  " + msg))
		}); err != nil {
			m.addOutput(m.styles.Warning.Render("Model download failed: " + err.Error()))
			m.addOutput(m.styles.Info.Render("Using pseudo-embeddings for testing."))
		}

		// Index with embeddings
		if err := vecIndex.IndexWorkspaceWithEmbeddings(ctx, cwd, func(msg string) {
			m.addOutput(m.styles.Dim.Render("  " + msg))
		}); err != nil {
			m.addOutput(m.styles.Warning.Render("Embedding failed: " + err.Error()))
			_ = vecIndex.Close()
		} else {
			// Get stats
			files, chunks, embeddings, _ := vecIndex.GetStats(ctx)
			m.vectorIndex = vecIndex
			m.addOutput("")
			m.addOutput(m.styles.Success.Render("✓ Semantic index built!"))
			m.addOutput(fmt.Sprintf("  Files:      %d", files))
			m.addOutput(fmt.Sprintf("  Chunks:     %d", chunks))
			m.addOutput(fmt.Sprintf("  Embeddings: %d", embeddings))
			if IsONNXAvailable() {
				m.addOutput(m.styles.Dim.Render("  Using ONNX embeddings"))
			} else {
				m.addOutput(m.styles.Dim.Render("  Using pseudo-embeddings (install ONNX for better results)"))
			}
		}

		m.addOutput("")
		m.addOutput(m.styles.Info.Render("Context will be included in code generation prompts."))

	case "/clear", "/c":
		m.conversation = []Message{}
		m.currentCode = ""
		m.currentFiles = nil
		m.validated = false
		m.analyzed = false
		m.originalPrompt = ""
		m.examples = nil
		m.dod = nil
		m.difficulty = ""
		m.intent = ""
		m.savedPath = ""
		m.historyPath = ""
		m.resetEscalation()
		m.tokenTracker.Reset()
		m.workspaceIndex = nil // Also clear the index on /clear
		if m.vectorIndex != nil {
			_ = m.vectorIndex.Close()
			m.vectorIndex = nil
		}
		m.addOutput("Conversation cleared.")

	case "/code", "/show":
		if m.currentCode == "" && len(m.currentFiles) == 0 {
			m.addOutput("No code generated yet.")
		} else if len(m.currentFiles) > 1 {
			// Multi-file project
			m.addOutput("")
			m.addOutput(m.styles.Warning.Render(fmt.Sprintf("Last generated code (%d files):", len(m.currentFiles))))
			for _, f := range m.currentFiles {
				m.addOutput("")
				m.addOutput(m.styles.Info.Render(fmt.Sprintf("// === %s ===", f.Filename)))
				m.addOutput("```cpp")
				m.addOutput(f.Content)
				m.addOutput("```")
			}
		} else {
			m.addOutput("")
			m.addOutput(m.styles.Warning.Render("Last generated code:"))
			m.addOutput("```cpp")
			m.addOutput(m.currentCode)
			m.addOutput("```")
		}

	case "/save", "/s":
		if m.currentCode == "" && len(m.currentFiles) == 0 {
			m.addOutput(m.styles.Error.Render("No code to save."))
		} else if len(m.currentFiles) > 1 {
			// Multi-file project - save all files
			if len(parts) >= 2 {
				// User specified a directory or single filename
				targetDir := parts[1]
				// Check if it looks like a directory
				if strings.HasSuffix(targetDir, "/") || strings.HasSuffix(targetDir, "\\") || !strings.Contains(targetDir, ".") {
					// Save all files to this directory
					if err := os.MkdirAll(targetDir, 0750); err != nil {
						m.addOutput(m.styles.Error.Render("Error creating directory: " + err.Error()))
						break
					}
					m.addOutput("")
					savedCount := 0
					for _, f := range m.currentFiles {
						filePath := filepath.Join(targetDir, f.Filename)
						if err := saveToFile(filePath, f.Content); err != nil {
							m.addOutput(m.styles.Error.Render(fmt.Sprintf("Error saving %s: %s", f.Filename, err.Error())))
						} else {
							m.addOutput(m.styles.Success.Render(fmt.Sprintf("✓ Saved %s", filePath)))
							savedCount++
						}
					}
					if savedCount == len(m.currentFiles) {
						m.savedPath = targetDir // Mark as saved
					}
				} else {
					// Single filename - save combined (backwards compatible)
					if err := saveToFile(targetDir, m.currentCode); err != nil {
						m.addOutput(m.styles.Error.Render("Error saving: " + err.Error()))
					} else {
						m.addOutput("")
						m.addOutput(m.styles.Success.Render("✓ Saved to " + targetDir))
						m.addOutput(m.styles.Dim.Render("  (all files combined into single file)"))
						m.savedPath = targetDir // Mark as saved
					}
				}
			} else {
				// No target specified - save to current directory with original filenames
				m.addOutput("")
				savedCount := 0
				for _, f := range m.currentFiles {
					if err := saveToFile(f.Filename, f.Content); err != nil {
						m.addOutput(m.styles.Error.Render(fmt.Sprintf("Error saving %s: %s", f.Filename, err.Error())))
					} else {
						m.addOutput(m.styles.Success.Render(fmt.Sprintf("✓ Saved %s", f.Filename)))
						savedCount++
					}
				}
				if savedCount == len(m.currentFiles) {
					m.savedPath = "." // Mark as saved to current dir
				}
			}
		} else {
			// Single file
			if len(parts) < 2 {
				m.addOutput(m.styles.Error.Render("Usage: /save <filename>"))
			} else {
				filename := parts[1]
				if err := saveToFile(filename, m.currentCode); err != nil {
					m.addOutput(m.styles.Error.Render("Error saving: " + err.Error()))
				} else {
					m.addOutput("")
					m.addOutput(m.styles.Success.Render("✓ Saved to " + filename))
					// Show file size for confirmation
					if info, err := os.Stat(filename); err == nil {
						m.addOutput(m.styles.Dim.Render(fmt.Sprintf("  %d bytes written", info.Size())))
					}
					m.savedPath = filename // Mark as saved
				}
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
		// Image exists, check for updates
		fmt.Printf("Validation container: %s\n", container.imageName)
		fmt.Print("Checking for updates... ")
		if container.CheckForUpdate(ctx) {
			fmt.Println("\033[93mupdate available!\033[0m")
			fmt.Print("Pull the latest container image? [Y/n] ")
			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))
			if response == "" || response == "y" || response == "yes" {
				fmt.Println("Pulling latest image...")
				if err := container.PullImage(ctx); err != nil {
					fmt.Printf("\033[93mWarning:\033[0m Failed to update: %v\n", err)
				} else {
					fmt.Println("\033[92mContainer updated!\033[0m")
				}
			}
		} else {
			fmt.Println("\033[92mup to date\033[0m")
		}
	}
	fmt.Println()

	// Initialize LLM provider
	providerCfg := cfg.GetProviderConfig()
	fmt.Printf("Connecting to %s...\n", providerDisplayName(cfg.Provider))
	provider, err := NewProvider(ctx, providerCfg)
	if err != nil {
		fmt.Print(FormatUserError(err))
		return err
	}
	fmt.Printf("Provider: %s\n", provider.Name())
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

	// Try to load existing workspace index
	var workspaceIndex *WorkspaceIndex
	cwd, _ := os.Getwd()
	if idx, err := LoadIndex(cwd); err == nil {
		workspaceIndex = idx
		fmt.Printf("Workspace index: %d files, %d functions, %d classes\n",
			idx.Summary.TotalFiles, idx.Summary.TotalFunctions, idx.Summary.TotalClasses)
	}

	// Try to load existing vector index for semantic search
	var vectorIndex *VectorIndex
	vecCfg := DefaultVectorIndexConfig()
	if vi, err := NewVectorIndex(vecCfg); err == nil {
		// Check if index has embeddings
		_, chunks, embeddings, _ := vi.GetStats(ctx)
		if embeddings > 0 {
			// Initialize embedder for search
			_ = vi.EnsureModel(ctx, nil)
			vectorIndex = vi
			fmt.Printf("Semantic index: %d chunks, %d embeddings\n", chunks, embeddings)

			// Quick incremental update for changed files (non-blocking)
			go func() {
				updateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				_ = vi.IndexWorkspaceWithEmbeddings(updateCtx, cwd, nil)
			}()
		} else {
			_ = vi.Close()
		}
	}
	fmt.Println()

	m := NewModel(provider, container, cfg)
	m.workspaceIndex = workspaceIndex
	m.vectorIndex = vectorIndex
	// Don't use WithAltScreen() - keeps normal terminal scrollback history
	p := tea.NewProgram(m)

	_, err = p.Run()
	return err
}

// providerDisplayName returns a human-readable name for the provider
func providerDisplayName(p ProviderType) string {
	switch p {
	case ProviderBedrock:
		return "AWS Bedrock"
	case ProviderAnthropic:
		return "Anthropic API"
	case ProviderOpenAI:
		return "OpenAI API"
	case ProviderGemini:
		return "Google Gemini API"
	default:
		return string(p)
	}
}
