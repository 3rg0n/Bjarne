package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
// Use ASCII variants on macOS or when BJARNE_ASCII=1 to avoid terminal width issues
var (
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

func init() {
	// Use ASCII box characters on macOS or when BJARNE_ASCII=1
	// Unicode box-drawing characters cause alignment issues on some terminals
	if shouldUseASCII() {
		boxTopLeft = "+"
		boxTopRight = "+"
		boxBottomLeft = "+"
		boxBottomRight = "+"
		boxHorizontal = "-"
		boxVertical = "|"
		treeVert = "|"
		treeBranch = "+-"
		treeEnd = "+-"
	}
}

func shouldUseASCII() bool {
	// Explicit override via environment variable
	if os.Getenv("BJARNE_ASCII") == "1" {
		return true
	}
	// Disable ASCII override if explicitly disabled
	if os.Getenv("BJARNE_ASCII") == "0" {
		return false
	}
	// Default to ASCII on macOS due to terminal width calculation issues
	// with Unicode box-drawing characters in some terminals (Terminal.app)
	return runtime.GOOS == "darwin"
}

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

	// Review results for display
	lastConfidence int    // Last review confidence score (0-100)
	lastSummary    string // Last review summary

	// Session data
	provider        LLMProvider // Abstract LLM provider (Bedrock, Anthropic, OpenAI, Gemini)
	container       *ContainerRuntime
	config          *Config
	tokenTracker    *TokenTracker
	conversation    []Message
	workspaceIndex  *WorkspaceIndex  // Indexed codebase for context
	vectorIndex     *VectorIndex     // Semantic search index with embeddings
	llmGuard        *LLMGuardClient  // Optional LLM security scanner
	validatorConfig *ValidatorConfig // Domain-specific validator settings

	// For async operations
	ctx      context.Context
	cancelFn context.CancelFunc

	// Terminal size
	width  int
	height int

	// Debug logging
	debugMode    bool   // When true, log validation errors to file
	debugLogPath string // Path to debug log file
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
	result     *GenerateResult
	confidence int    // 0-100 confidence score
	summary    string // One-line summary for user
	err        error
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
		textarea:        ta,
		spinner:         s,
		styles:          NewStyles(),
		state:           StateInput,
		provider:        provider,
		container:       container,
		config:          cfg,
		tokenTracker:    NewTokenTracker(cfg.MaxTotalTokens, cfg.WarnTokenThreshold),
		conversation:    []Message{},
		llmGuard:        NewLLMGuardClient(),
		validatorConfig: DefaultValidatorConfig(),
		ctx:             context.Background(),
		width:           120, // Default, will be updated on WindowSizeMsg
		height:          24,
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
			// Classification failed - default to NEW MEDIUM and continue silently
			m.intent = "NEW"
			m.difficulty = "MEDIUM"
			return m.startThinking(m.getModelForComplexity("MEDIUM"))
		}

		// Parse the classification result (INTENT COMPLEXITY) - internal use only
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

		// Silently continue to analysis - no clinical output
		model := m.getModelForComplexity(m.difficulty)
		return m.startThinking(model)

	case thinkingDoneMsg:
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

		// Parse and clean the response (remove difficulty tag if present)
		_, reflection := parseDifficulty(msg.result.Text)
		cleanText := stripMarkdown(reflection)

		// Handle based on intent
		if m.intent == "QUESTION" {
			// For questions, just show the answer naturally
			m.addOutput("")
			lines := wrapText(cleanText, 76)
			for _, line := range lines {
				m.addOutput(line)
			}
			m.state = StateInput
			m.textarea.Focus()
			return m, textarea.Blink
		}

		// Check if the LLM refused to proceed
		if containsRefusal(reflection) {
			m.addOutput("")
			lines := wrapText(cleanText, 76)
			for _, line := range lines {
				m.addOutput(line)
			}
			m.state = StateInput
			m.textarea.Focus()
			return m, textarea.Blink
		}

		// Auto-proceed for EASY tasks or CONTINUE intent (no questions)
		if (m.difficulty == "EASY" || m.intent == "CONTINUE") && !containsQuestion(reflection) {
			m.conversation = append(m.conversation, Message{Role: "user", Content: GenerateNowPrompt})
			return m.startGenerating()
		}

		// For tasks that need confirmation, show the analysis conversationally
		m.addOutput("")
		lines := wrapText(cleanText, 76)
		for _, line := range lines {
			m.addOutput(line)
		}
		m.addOutput("")

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
			m.debugLog("Validation system error: %s", msg.err.Error())
			m.addOutput(m.styles.Error.Render("Validation error: " + msg.err.Error()))
			m.state = StateInput
			m.textarea.Focus()
			return m, nil
		}

		// Log all validation results to debug file
		m.debugLogValidationResults(msg.results)

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
			// Review failed but sanitizers passed - show code anyway with default confidence
			m.addOutput(m.styles.Warning.Render("Code review unavailable: " + msg.err.Error()))
			m.lastConfidence = 80 // Reasonable default
			m.lastSummary = "Sanitizers passed; review unavailable."
			return m.showValidatedCode()
		}

		// Store confidence and summary for display
		m.lastConfidence = msg.confidence
		m.lastSummary = msg.summary

		// Confidence-based decision:
		// >= 70: Accept the code (user can decide based on summary)
		// < 70: Try to improve if we can, otherwise show anyway
		if msg.confidence >= 70 {
			m.addOutput(m.styles.Success.Render(fmt.Sprintf("  └─ Gate: review... %d%% confidence", msg.confidence)))
			m.reviewFailures = 0
			return m.showValidatedCode()
		}

		// Low confidence - try to fix if possible
		m.reviewFailures++
		m.addOutput(m.styles.Warning.Render(fmt.Sprintf("  └─ Gate: review... %d%% confidence", msg.confidence)))
		m.addOutput(m.styles.Dim.Render("     " + msg.summary))

		// Limit review retries to 2 - don't loop forever on pedantic reviews
		if m.reviewFailures >= 2 {
			m.addOutput("")
			m.addOutput(m.styles.Warning.Render("Review confidence remains low but sanitizers pass."))
			m.addOutput(m.styles.Dim.Render("(Showing code - review the summary and decide if changes are needed)"))
			return m.showValidatedCode()
		}

		m.lastValidationErrs = "Code review (" + fmt.Sprintf("%d%%", msg.confidence) + "): " + msg.summary

		// Try to fix if we can escalate
		if m.config.EscalateOnFailure && m.canEscalate() {
			m.addOutput("")
			m.addOutput("Attempting to improve confidence...")
			return m.startFix()
		}

		// Can't escalate - show code with low confidence (user decides)
		m.addOutput("")
		m.addOutput(m.styles.Warning.Render("Code passed sanitizers. Review the summary below."))
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
		// Claude Code-style status: * Doing something… (esc to interrupt · 3s)
		elapsed := time.Since(m.startTime).Seconds()
		status := fmt.Sprintf("esc to interrupt · %.0fs", elapsed)

		b.WriteString(m.styles.Accent.Render("* "))
		b.WriteString(m.statusMsg)
		b.WriteString(m.styles.Dim.Render(" (" + status + ")"))

	case StateRevealing:
		// Don't show progress - the scrolling code is visual feedback
		b.WriteString("")
	}

	return b.String()
}

// Helper methods

func (m *Model) addOutput(line string) {
	// Print directly to stdout for permanent history (scrollback)
	fmt.Println(line)
}

// debugLog writes a message to the debug log file if debug mode is enabled
func (m *Model) debugLog(format string, args ...interface{}) {
	if !m.debugMode || m.debugLogPath == "" {
		return
	}

	// Open file in append mode
	f, err := os.OpenFile(m.debugLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	// Write timestamp and message
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(f, "[%s] %s\n", timestamp, msg)
}

// debugLogValidationResults logs detailed validation results to the debug file
func (m *Model) debugLogValidationResults(results []ValidationResult) {
	if !m.debugMode {
		return
	}

	m.debugLog("=== Validation Results ===")
	m.debugLog("Original prompt: %s", m.originalPrompt)
	m.debugLog("Difficulty: %s, Intent: %s", m.difficulty, m.intent)
	m.debugLog("")

	// Log the generated code
	if len(m.currentFiles) > 1 {
		for _, f := range m.currentFiles {
			m.debugLog("--- File: %s ---", f.Filename)
			m.debugLog("%s", f.Content)
		}
	} else if m.currentCode != "" {
		m.debugLog("--- Generated Code ---")
		m.debugLog("%s", m.currentCode)
	}
	m.debugLog("")

	// Log each validation result
	for _, r := range results {
		status := "PASS"
		if !r.Success {
			status = "FAIL"
		}
		m.debugLog("[%s] Stage: %s (%.2fs)", status, r.Stage, r.Duration.Seconds())
		if r.Output != "" {
			m.debugLog("  Output: %s", r.Output)
		}
		if r.Error != "" {
			m.debugLog("  Error: %s", r.Error)
		}
	}
	m.debugLog("=== End Validation Results ===")
	m.debugLog("")
}

func (m *Model) startClassifying(prompt string) (Model, tea.Cmd) {
	m.state = StateClassifying
	m.statusMsg = "Thinking…"
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
	m.statusMsg = "Thinking…"
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
	m.statusMsg = "Thinking…"
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

	m.statusMsg = "Writing code…"
	m.startTime = time.Now()
	m.tokenCount = 0

	// Reset escalation state for fresh generation cycle
	m.resetEscalation()

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
				prompt += "\nIMPORTANT: Generate code that integrates with this codebase:"
				prompt += "\n- Match the naming conventions (case, prefixes, suffixes)"
				prompt += "\n- Use the same include patterns and header structure"
				prompt += "\n- Follow the coding style (braces, spacing, etc.)"
				prompt += "\n- Reuse existing types, utilities, and patterns where applicable"
				return prompt
			}
		}
	}

	// Fall back to workspace index (structural context)
	if m.workspaceIndex != nil && len(m.workspaceIndex.Files) > 0 {
		context := m.workspaceIndex.GetContextForPrompt(2000) // ~2000 tokens max
		if context != "" {
			prompt += "\n\n" + context
			prompt += "\n\nIMPORTANT: Generate code that integrates with this codebase:"
			prompt += "\n- Match existing naming conventions and coding style"
			prompt += "\n- Use compatible types and include patterns"
			prompt += "\n- Code should fit naturally alongside existing files"
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
	m.statusMsg = "Validating…"
	m.startTime = time.Now()

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

		// If core validation passed, run domain-specific validators
		if err == nil && allPassed(results) && m.validatorConfig != nil {
			domainResults := m.runDomainValidators(ctx)
			for _, dr := range domainResults {
				results = append(results, ValidationResult{
					Stage:   string(dr.ValidatorID),
					Success: dr.Success,
					Output:  dr.Output,
				})
			}
		}

		return validationDoneMsg{results: results, err: err}
	}
}

// startReviewing initiates the LLM code review gate
func (m *Model) startReviewing(results []ValidationResult) (Model, tea.Cmd) {
	m.state = StateReviewing
	m.statusMsg = "Reviewing code…"
	m.startTime = time.Now()

	// Show sanitizer gate results
	m.showValidationSuccess(results)

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

		// Use Haiku for fast review
		result, err := m.provider.Generate(ctx, m.config.ReflectionModel, "", []Message{
			{Role: "user", Content: reviewPrompt},
		}, 200)

		if err != nil {
			return reviewDoneMsg{err: err}
		}

		// Parse response: CONFIDENCE: <score>\nSUMMARY: <text>
		response := strings.TrimSpace(result.Text)
		confidence, summary := parseReviewResponse(response)

		return reviewDoneMsg{result: result, confidence: confidence, summary: summary}
	}
}

// parseReviewResponse extracts confidence score and summary from review output
func parseReviewResponse(response string) (int, string) {
	confidence := 85 // Default to reasonable confidence if parsing fails
	summary := "Code review completed."

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)

		if strings.HasPrefix(upper, "CONFIDENCE:") {
			// Extract number after CONFIDENCE:
			numStr := strings.TrimPrefix(line, line[:len("CONFIDENCE:")])
			numStr = strings.TrimSpace(numStr)
			// Remove any trailing % or text
			for i, c := range numStr {
				if c < '0' || c > '9' {
					numStr = numStr[:i]
					break
				}
			}
			if n, err := fmt.Sscanf(numStr, "%d", &confidence); n == 1 && err == nil {
				// Clamp to 0-100
				if confidence < 0 {
					confidence = 0
				}
				if confidence > 100 {
					confidence = 100
				}
			}
		} else if strings.HasPrefix(upper, "SUMMARY:") {
			summary = strings.TrimPrefix(line, line[:len("SUMMARY:")])
			summary = strings.TrimSpace(summary)
		}
	}

	return confidence, summary
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

	// Show confidence score and summary
	confidenceStyle := m.styles.Success
	if m.lastConfidence < 70 {
		confidenceStyle = m.styles.Warning
	} else if m.lastConfidence < 90 {
		confidenceStyle = m.styles.Info
	}
	m.addOutput("")
	m.addOutput(fmt.Sprintf("  Confidence: %s", confidenceStyle.Render(fmt.Sprintf("%d%%", m.lastConfidence))))
	if m.lastSummary != "" {
		m.addOutput(fmt.Sprintf("  %s", m.styles.Dim.Render(m.lastSummary)))
	}

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
	m.statusMsg = fmt.Sprintf("Fixing issues (%d/15)…", m.totalFixAttempts)
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
		m.addOutput("  /config [category]     Configure validators (game, hft, embedded, security, perf)")
		m.addOutput("  /debug                 Toggle debug logging (saves validation errors to file)")
		m.addOutput("  /init                  Index current directory for context-aware generation")
		m.addOutput("  /validate <file>, /v   Validate existing file without AI generation")
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

		// Auto-download ONNX runtime if not available
		if !IsONNXAvailable() {
			m.addOutput(m.styles.Dim.Render("  ONNX runtime not found, downloading..."))
			if err := EnsureONNXRuntime(func(msg string) {
				m.addOutput(m.styles.Dim.Render("  " + msg))
			}); err != nil {
				m.addOutput(m.styles.Warning.Render("  ONNX download failed: " + err.Error()))
				m.addOutput(m.styles.Info.Render("  Will use pseudo-embeddings instead."))
			}
		}

		cfg := DefaultVectorIndexConfig()
		vecIndex, err := NewVectorIndex(cfg)
		if err != nil {
			m.addOutput(m.styles.Error.Render("Vector index failed: " + err.Error()))
			m.addOutput(m.styles.Info.Render("Structural index will still be used for context."))
			break
		}

		// Download embedding model if needed
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
				m.addOutput(m.styles.Dim.Render("  Using pseudo-embeddings (ONNX unavailable)"))
			}
		}

		m.addOutput("")
		m.addOutput(m.styles.Info.Render("Context will be included in code generation prompts."))

	case "/config":
		m.showValidatorConfig(parts[1:])

	case "/debug":
		m.debugMode = !m.debugMode
		m.addOutput("")
		if m.debugMode {
			// Set up debug log path
			homeDir, err := os.UserHomeDir()
			if err != nil {
				m.debugLogPath = "bjarne-debug.log"
			} else {
				debugDir := filepath.Join(homeDir, ".bjarne")
				_ = os.MkdirAll(debugDir, 0750)
				m.debugLogPath = filepath.Join(debugDir, "debug.log")
			}
			m.addOutput(m.styles.Success.Render("Debug logging enabled"))
			m.addOutput(fmt.Sprintf("Log file: %s", m.styles.Dim.Render(m.debugLogPath)))
		} else {
			m.addOutput(m.styles.Warning.Render("Debug logging disabled"))
		}

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

	case "/validate", "/v":
		// Direct validation without AI generation
		if len(parts) < 2 {
			m.addOutput(m.styles.Error.Render("Usage: /validate <file>"))
			m.addOutput(m.styles.Dim.Render("  Validates an existing file through all gates without AI generation."))
			m.textarea.Reset()
			return m, nil
		}
		filename := parts[1]

		// Read the file
		content, err := os.ReadFile(filename)
		if err != nil {
			m.addOutput(m.styles.Error.Render(fmt.Sprintf("Error reading file: %s", err.Error())))
			m.textarea.Reset()
			return m, nil
		}

		// Store the code for validation
		m.currentCode = string(content)
		m.currentFiles = []CodeFile{{Filename: filepath.Base(filename), Content: string(content)}}
		m.originalPrompt = fmt.Sprintf("(Direct validation of %s)", filename)

		// Show what we're validating
		m.addOutput("")
		m.addOutput(m.styles.Info.Render(fmt.Sprintf("Validating: %s (%d bytes)", filename, len(content))))

		m.textarea.Reset()
		m.textarea.Blur()
		return m.startValidation()

	default:
		m.addOutput(m.styles.Error.Render("Unknown command: " + cmd))
	}

	m.textarea.Reset()
	return m, nil
}

// printSplashScreen displays the bjarne logo and version
func printSplashScreen() {
	// ASCII art logo - stylized "bjarne" text
	// Use dynamic box characters to handle macOS terminal issues
	top := boxTopLeft + strings.Repeat(boxHorizontal, 62) + boxTopRight
	bot := boxBottomLeft + strings.Repeat(boxHorizontal, 62) + boxBottomRight
	v := boxVertical
	logo := fmt.Sprintf(`
    %s
    %s   _     _                                                    %s
    %s  | |__ (_) __ _ _ __ _ __   ___                              %s
    %s  | '_ \| |/ _`+"`"+` | '__| '_ \ / _ \                             %s
    %s  | |_) | | (_| | |  | | | |  __/                             %s
    %s  |_.__// |\__,_|_|  |_| |_|\___|                             %s
    %s      |__/                                                    %s
    %s`, top, v, v, v, v, v, v, v, v, v, v, v, v, bot)
	fmt.Println("\033[96m" + logo + "\033[0m")
	fmt.Printf("    \033[90m%s - AI-assisted C/C++ with mandatory validation\033[0m\n\n", Version)
}

// StartTUI initializes everything and starts the bubbletea TUI
func StartTUI() error {
	ctx := context.Background()

	// Load configuration (fast, from disk)
	cfg := LoadConfig()

	// Show splash screen immediately
	printSplashScreen()

	// These checks are fast - do them synchronously
	container, err := DetectContainerRuntime()
	if err != nil {
		fmt.Print(FormatUserError(err))
		return err
	}

	providerCfg := cfg.GetProviderConfig()
	provider, err := NewProvider(ctx, providerCfg)
	if err != nil {
		fmt.Print(FormatUserError(err))
		return err
	}

	// Show status line
	fmt.Printf("    \033[92m●\033[0m %s  \033[92m●\033[0m %s", container.GetBinary(), provider.Name())

	// Load workspace index (fast, from disk cache)
	var workspaceIndex *WorkspaceIndex
	cwd, _ := os.Getwd()
	if idx, err := LoadIndex(cwd); err == nil {
		workspaceIndex = idx
		fmt.Printf("  \033[92m●\033[0m %d files indexed", idx.Summary.TotalFiles)
	}
	fmt.Println()
	fmt.Println()
	fmt.Println("    Type your request or /help for commands")

	// Create model and start TUI immediately
	m := NewModel(provider, container, cfg)
	m.workspaceIndex = workspaceIndex

	// Do slow operations in background AFTER TUI starts
	go func() {
		// Check for updates silently
		PrintUpdateNotice()

		// Check container image in background
		if !container.ImageExists(ctx) {
			// Will prompt on first validation attempt
		}

		// Load vector index in background
		vecCfg := DefaultVectorIndexConfig()
		if vi, errVec := NewVectorIndex(vecCfg); errVec == nil {
			_, _, embeddings, _ := vi.GetStats(ctx)
			if embeddings > 0 {
				modelCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				_ = vi.EnsureModel(modelCtx, nil)
				cancel()
				m.vectorIndex = vi
			} else {
				_ = vi.Close()
			}
		}
	}()

	// Don't use WithAltScreen() - keeps normal terminal scrollback history
	p := tea.NewProgram(m, tea.WithInputTTY())

	_, err = p.Run()
	return err
}

// showValidatorConfig displays and manages validator configuration
func (m *Model) showValidatorConfig(args []string) {
	m.addOutput("")

	// Map short category names to full names
	categoryMap := map[string]ValidatorCategory{
		"game":     CategoryGame,
		"hft":      CategoryHFT,
		"embedded": CategoryEmbedded,
		"security": CategorySecurity,
		"perf":     CategoryPerformance,
		"core":     CategoryCore,
	}

	// If arg provided, toggle that category or specific validator
	if len(args) > 0 {
		arg := strings.ToLower(args[0])

		// Check if it's a category
		if cat, ok := categoryMap[arg]; ok {
			// Toggle entire category
			validators := GetValidatorsByCategory()[cat]
			// Check if any are enabled
			anyEnabled := false
			for _, v := range validators {
				if m.validatorConfig.IsEnabled(v.ID) {
					anyEnabled = true
					break
				}
			}

			if anyEnabled {
				m.validatorConfig.DisableCategory(cat)
				m.addOutput(m.styles.Warning.Render(fmt.Sprintf("Disabled all %s validators", arg)))
			} else {
				m.validatorConfig.EnableCategory(cat)
				m.addOutput(m.styles.Success.Render(fmt.Sprintf("Enabled all %s validators", arg)))
			}
			m.addOutput("")
		} else {
			// Try to find validator by ID
			found := false
			for _, v := range AllValidators() {
				if strings.EqualFold(string(v.ID), arg) {
					newState := m.validatorConfig.Toggle(v.ID)
					if newState {
						m.addOutput(m.styles.Success.Render(fmt.Sprintf("Enabled: %s", v.Name)))
					} else {
						m.addOutput(m.styles.Warning.Render(fmt.Sprintf("Disabled: %s", v.Name)))
					}
					found = true
					break
				}
			}
			if !found {
				m.addOutput(m.styles.Error.Render(fmt.Sprintf("Unknown validator or category: %s", arg)))
			}
			m.addOutput("")
		}
	}

	// Show current configuration
	m.addOutput(m.styles.Accent.Render("Validator Configuration"))
	m.addOutput("")

	byCategory := GetValidatorsByCategory()
	categoryOrder := []ValidatorCategory{CategoryCore, CategoryGame, CategoryHFT, CategoryEmbedded, CategorySecurity, CategoryPerformance}
	categoryNames := map[ValidatorCategory]string{
		CategoryCore:        "Core (always run)",
		CategoryGame:        "Game Development (/config game)",
		CategoryHFT:         "HFT (/config hft)",
		CategoryEmbedded:    "Embedded Systems (/config embedded)",
		CategorySecurity:    "Security (/config security)",
		CategoryPerformance: "Performance (/config perf)",
	}

	for _, cat := range categoryOrder {
		validators := byCategory[cat]
		if len(validators) == 0 {
			continue
		}

		m.addOutput(m.styles.Info.Render(categoryNames[cat]))
		for _, v := range validators {
			status := "[ ]"
			style := m.styles.Dim
			if m.validatorConfig.IsEnabled(v.ID) {
				status = "[✓]"
				style = m.styles.Success
			}
			line := fmt.Sprintf("  %s %s - %s", status, v.Name, v.Description)
			m.addOutput(style.Render(line))
		}
		m.addOutput("")
	}

	m.addOutput(m.styles.Dim.Render("Usage: /config <category|validator> to toggle"))
}

// allPassed checks if all validation results passed
func allPassed(results []ValidationResult) bool {
	for _, r := range results {
		if !r.Success {
			return false
		}
	}
	return true
}

// runDomainValidators executes enabled domain-specific validators
func (m *Model) runDomainValidators(ctx context.Context) []DomainValidationResult {
	// Create temp directory for validation
	tmpDir, err := os.MkdirTemp("", "bjarne-domain-*")
	if err != nil {
		return nil
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Determine code and filename
	var code, filename string
	if len(m.currentFiles) > 0 {
		// Use main file for domain validation
		code = m.currentFiles[0].Content
		filename = m.currentFiles[0].Filename
	} else {
		code = m.currentCode
		filename = "code.cpp"
	}

	// Write code to temp directory
	codePath := filepath.Join(tmpDir, filename)
	if err := os.WriteFile(codePath, []byte(code), 0600); err != nil {
		return nil
	}

	// Run domain validators
	return m.container.RunDomainValidators(ctx, tmpDir, code, filename, m.validatorConfig)
}
