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
	StateInput State = iota
	StateThinking
	StateGenerating
	StateValidating
	StateConfirm
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
	state       State
	statusMsg   string
	startTime   time.Time
	tokenCount  int
	currentCode string
	validated   bool

	// Session data
	bedrock      *BedrockClient
	container    *ContainerRuntime
	config       *Config
	tokenTracker *TokenTracker
	conversation []Message

	// For async operations
	ctx      context.Context
	cancelFn context.CancelFunc
	output   []string // Lines of output to display
}

// Messages for async operations
type thinkingDoneMsg struct {
	result *GenerateResult
	err    error
}

type generatingDoneMsg struct {
	result *GenerateResult
	err    error
}

type validationDoneMsg struct {
	results []ValidationResult
	err     error
}

type tickMsg time.Time

// NewModel creates a new bubbletea model
func NewModel(bedrock *BedrockClient, container *ContainerRuntime, cfg *Config) Model {
	// Create textarea for input
	ta := textarea.New()
	ta.Placeholder = "Enter your request..."
	ta.Focus()
	ta.CharLimit = 0 // No limit
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false) // Enter submits

	// Create spinner
	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: []string{"✽", "✻", "✼", "✽", "✻", "✼"},
		FPS:    time.Millisecond * 150,
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
		output:       []string{},
		ctx:          context.Background(),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			// Always quit on Ctrl+C
			return m, tea.Quit

		case tea.KeyEsc:
			// Cancel current operation if processing
			if m.state != StateInput {
				if m.cancelFn != nil {
					m.cancelFn()
				}
				m.state = StateInput
				m.addOutput(m.styles.Warning.Render("⚠ Interrupted"))
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

				// Start thinking
				m.textarea.Reset()
				m.textarea.Blur()
				return m.startThinking(input)
			} else if m.state == StateConfirm {
				// User confirmed, start generating
				return m.startGenerating()
			}
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

	case thinkingDoneMsg:
		if msg.err != nil {
			if m.ctx.Err() == context.Canceled {
				// Already handled by Esc
				return m, nil
			}
			m.addOutput(m.styles.Error.Render("✗ Thinking failed: " + msg.err.Error()))
			m.state = StateInput
			m.textarea.Focus()
			return m, nil
		}
		// Show reflection and ask for confirmation
		m.tokenTracker.Add(msg.result.InputTokens, msg.result.OutputTokens)
		m.conversation = append(m.conversation, Message{Role: "assistant", Content: msg.result.Text})

		difficulty, reflection := parseDifficulty(msg.result.Text)
		m.addOutput("")
		m.addOutput(m.styles.Accent.Render("bjarne: ") + reflection)

		if difficulty == "EASY" {
			// Skip confirmation for easy tasks
			m.conversation = append(m.conversation, Message{Role: "user", Content: GenerateNowPrompt})
			return m.startGenerating()
		}

		m.state = StateConfirm
		m.addOutput("")
		m.addOutput(m.styles.Dim.Render("Press Enter to proceed, Esc to cancel"))
		return m, nil

	case generatingDoneMsg:
		if msg.err != nil {
			if m.ctx.Err() == context.Canceled {
				return m, nil
			}
			m.addOutput(m.styles.Error.Render("✗ Generation failed: " + msg.err.Error()))
			m.state = StateInput
			m.textarea.Focus()
			return m, nil
		}
		m.tokenTracker.Add(msg.result.InputTokens, msg.result.OutputTokens)
		m.conversation = append(m.conversation, Message{Role: "assistant", Content: msg.result.Text})

		code := extractCode(msg.result.Text)
		if code == "" {
			m.addOutput("")
			m.addOutput(m.styles.Info.Render("bjarne: ") + msg.result.Text)
			m.state = StateInput
			m.textarea.Focus()
			return m, nil
		}

		m.currentCode = code
		return m.startValidation()

	case validationDoneMsg:
		if msg.err != nil {
			if m.ctx.Err() == context.Canceled {
				return m, nil
			}
			m.addOutput(m.styles.Error.Render("✗ Validation error: " + msg.err.Error()))
			m.state = StateInput
			m.textarea.Focus()
			return m, nil
		}

		allPassed := true
		for _, r := range msg.results {
			if !r.Success {
				allPassed = false
				break
			}
		}

		if allPassed {
			m.validated = true
			m.showValidationSuccess(msg.results)
		} else {
			m.validated = false
			m.showValidationFailure(msg.results)
		}

		m.state = StateInput
		m.textarea.Focus()
		return m, nil

	case tickMsg:
		// Update elapsed time display
		return m, tea.Tick(time.Second, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	var b strings.Builder

	// Show output history
	for _, line := range m.output {
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Show current state
	switch m.state {
	case StateInput:
		b.WriteString(m.styles.Prompt.Render(">") + " ")
		b.WriteString(m.textarea.View())

	case StateThinking, StateGenerating, StateValidating:
		elapsed := time.Since(m.startTime).Seconds()
		status := fmt.Sprintf("esc to interrupt · %.0fs", elapsed)
		if m.tokenCount > 0 {
			status += fmt.Sprintf(" · ↓ %d tokens", m.tokenCount)
		}
		b.WriteString(m.styles.Accent.Render(m.spinner.View()) + " ")
		b.WriteString(m.statusMsg + " ")
		b.WriteString(m.styles.Dim.Render("(" + status + ")"))

	case StateConfirm:
		b.WriteString(m.styles.Prompt.Render(">") + " ")
	}

	return b.String()
}

// Helper methods

func (m *Model) addOutput(line string) {
	m.output = append(m.output, line)
}

func (m *Model) startThinking(prompt string) (Model, tea.Cmd) {
	m.state = StateThinking
	m.statusMsg = "Thinking…"
	m.startTime = time.Now()
	m.tokenCount = 0

	// Add user message
	m.conversation = append(m.conversation, Message{Role: "user", Content: prompt})

	// Create cancelable context
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancelFn = cancel

	return *m, tea.Batch(
		m.spinner.Tick,
		m.doThinking(ctx),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

func (m *Model) doThinking(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		result, err := m.bedrock.GenerateWithModel(ctx, m.config.ChatModel, ReflectionSystemPrompt, m.conversation, m.config.MaxTokens)
		return thinkingDoneMsg{result: result, err: err}
	}
}

func (m *Model) startGenerating() (Model, tea.Cmd) {
	m.state = StateGenerating
	m.statusMsg = fmt.Sprintf("Generating with %s…", shortModelName(m.config.GenerateModel))
	m.startTime = time.Now()
	m.tokenCount = 0

	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancelFn = cancel

	return *m, tea.Batch(
		m.spinner.Tick,
		m.doGenerating(ctx),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

func (m *Model) doGenerating(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		result, err := m.bedrock.GenerateWithModel(ctx, m.config.GenerateModel, GenerationSystemPrompt, m.conversation, m.config.MaxTokens)
		return generatingDoneMsg{result: result, err: err}
	}
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
		results, err := m.container.ValidateCode(ctx, m.currentCode, "code.cpp")
		return validationDoneMsg{results: results, err: err}
	}
}

func (m *Model) showValidationSuccess(results []ValidationResult) {
	m.addOutput("")
	m.addOutput(m.styles.Success.Render("✓ Validated Code:"))
	m.addOutput("```cpp")
	m.addOutput(m.currentCode)
	m.addOutput("```")
	m.addOutput("")
	m.addOutput(m.styles.Success.Render("Validation Summary"))
	m.addOutput(strings.Repeat("─", 50))

	totalTime := 0.0
	for _, r := range results {
		totalTime += r.Duration.Seconds()
		m.addOutput(fmt.Sprintf("  %s %s %s",
			m.styles.Checkmark.Render("☑"),
			r.Stage,
			m.styles.Dim.Render(fmt.Sprintf("(%.2fs)", r.Duration.Seconds()))))
	}

	m.addOutput(strings.Repeat("─", 50))
	m.addOutput(fmt.Sprintf("  %s All %d stages passed %s",
		m.styles.Success.Render("✓"),
		len(results),
		m.styles.Dim.Render(fmt.Sprintf("[%.2fs total]", totalTime))))
	m.addOutput("")
	m.addOutput(fmt.Sprintf("Use %s to save the validated code", m.styles.Accent.Render("/save <filename>")))
}

func (m *Model) showValidationFailure(results []ValidationResult) {
	m.addOutput("")
	m.addOutput(m.styles.Error.Render("✗ Validation failed"))
	m.addOutput(strings.Repeat("─", 50))

	for _, r := range results {
		if r.Success {
			m.addOutput(fmt.Sprintf("  %s %s %s",
				m.styles.Checkmark.Render("☑"),
				r.Stage,
				m.styles.Dim.Render(fmt.Sprintf("(%.2fs)", r.Duration.Seconds()))))
		} else {
			m.addOutput(fmt.Sprintf("  %s %s %s",
				m.styles.Cross.Render("☒"),
				r.Stage,
				m.styles.Dim.Render(fmt.Sprintf("(%.2fs)", r.Duration.Seconds()))))
			// Show first line of error
			if r.Error != "" {
				errLines := strings.Split(r.Error, "\n")
				if len(errLines) > 0 {
					m.addOutput(fmt.Sprintf("      %s", m.styles.Dim.Render(truncateError(errLines[0], 60))))
				}
			}
		}
	}

	m.addOutput(strings.Repeat("─", 50))
	m.addOutput("")
	m.addOutput(m.styles.Warning.Render("Last generated code (failed validation):"))
	m.addOutput("```cpp")
	m.addOutput(m.currentCode)
	m.addOutput("```")
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
				m.addOutput(m.styles.Success.Render("✓ Saved to " + filename))
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
		fmt.Printf("Validation container: %s ✓\n", container.imageName)
	}
	fmt.Println()

	// Initialize Bedrock client
	fmt.Println("Connecting to AWS Bedrock...")
	bedrock, err := NewBedrockClient(ctx, cfg.GenerateModel)
	if err != nil {
		fmt.Print(FormatUserError(err))
		return err
	}
	fmt.Printf("Chat model: %s\n", shortModelName(cfg.ChatModel))
	fmt.Printf("Generate model: %s\n", shortModelName(cfg.GenerateModel))
	if cfg.EscalateOnFailure && len(cfg.EscalationModels) > 0 {
		fmt.Printf("Escalation: enabled (%d models)\n", len(cfg.EscalationModels))
	}
	fmt.Println()
	fmt.Println("Type /help for commands, /quit to exit")
	fmt.Println("Press Esc to interrupt during processing")
	fmt.Println()

	m := NewModel(bedrock, container, cfg)
	p := tea.NewProgram(m)

	_, err = p.Run()
	return err
}
