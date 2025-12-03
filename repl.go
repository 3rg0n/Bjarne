package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Session holds the state for a REPL session
type Session struct {
	bedrock       *BedrockClient
	container     *ContainerRuntime
	config        *Config
	theme         *Theme
	tokenTracker  *TokenTracker
	conversation  []Message
	lastCode      string
	lastValidated bool
}

// handleFirstRunPull handles the first-run container pull experience
func handleFirstRunPull(ctx context.Context, container *ContainerRuntime) error {
	fmt.Println()
	fmt.Println("\033[93m┌─────────────────────────────────────────────────────────────┐\033[0m")
	fmt.Println("\033[93m│                     First-time Setup                        │\033[0m")
	fmt.Println("\033[93m└─────────────────────────────────────────────────────────────┘\033[0m")
	fmt.Println()
	fmt.Println("bjarne requires a validation container to check your C/C++ code")
	fmt.Println("for memory errors, undefined behavior, and data races.")
	fmt.Println()
	fmt.Printf("Container image: \033[96m%s\033[0m\n", container.imageName)
	fmt.Printf("Size: ~500MB (Wolfi-based, minimal attack surface)\n")
	fmt.Println()
	fmt.Print("Pull the validation container now? [Y/n] ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "" && response != "y" && response != "yes" {
		return fmt.Errorf("container pull declined")
	}

	fmt.Println()
	fmt.Println("\033[93mPulling container image...\033[0m")
	fmt.Println("(This may take a few minutes on first run)")
	fmt.Println()

	if err := container.PullImage(ctx); err != nil {
		return fmt.Errorf("failed to pull container: %w", err)
	}

	fmt.Println()
	fmt.Println("\033[92m✓ Container ready!\033[0m")
	return nil
}

// Run starts the interactive REPL loop
func Run() error {
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
			fmt.Println("         Run bjarne again after installing the container image.")
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
	fmt.Println()

	session := &Session{
		bedrock:      bedrock,
		container:    container,
		config:       cfg,
		theme:        cfg.Theme,
		tokenTracker: NewTokenTracker(cfg.MaxTotalTokens, cfg.WarnTokenThreshold),
		conversation: []Message{},
	}

	inputReader := NewInputReader(cfg.Theme)
	defer inputReader.Close()

	for {
		fmt.Print(session.theme.PromptCode() + ">" + session.theme.Reset() + " ")

		fullInput, displayText, err := inputReader.ReadInput()
		if err != nil {
			break
		}

		input := strings.TrimSpace(fullInput)
		if input == "" {
			continue
		}

		// If this was a multi-line paste, show the collapsed indicator
		lines := strings.Split(input, "\n")
		if len(lines) > 1 {
			// Move up to overwrite the raw pasted lines and show collapsed version
			// Clear previous lines and show collapsed indicator
			for i := 0; i < len(lines); i++ {
				fmt.Print("\033[A\033[K") // Move up and clear line
			}
			fmt.Printf("%s%s%s %s\n",
				session.theme.PromptCode(), ">", session.theme.Reset(),
				displayText)
		}

		// Handle slash commands (only first line for commands)
		if strings.HasPrefix(input, "/") {
			if !session.handleCommand(ctx, input) {
				break
			}
			continue
		}

		// Handle code generation request
		if err := session.handlePrompt(ctx, input); err != nil {
			fmt.Fprintf(os.Stderr, "\n%s %v\n", session.theme.Error("Error:"), err)
		}
	}

	fmt.Println("\nGoodbye!")
	return nil
}

// handleCommand processes slash commands, returns false if should quit
func (s *Session) handleCommand(ctx context.Context, input string) bool {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return true
	}

	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/quit", "/exit", "/q":
		return false

	case "/help", "/h":
		printCommandHelp()

	case "/save", "/s":
		if len(parts) < 2 {
			fmt.Println("\033[91mUsage:\033[0m /save <filename>")
			return true
		}
		if s.lastCode == "" {
			fmt.Println("\033[91mNo code to save.\033[0m Generate code first.")
			return true
		}
		if !s.lastValidated {
			fmt.Println("\033[93mWarning:\033[0m Code has not passed validation yet.")
			fmt.Println("         Saving anyway, but use at your own risk.")
		}
		filename := parts[1]
		if err := saveToFile(filename, s.lastCode); err != nil {
			fmt.Fprintf(os.Stderr, "\033[91mError saving:\033[0m %v\n", err)
		} else {
			fmt.Printf("\033[92mSaved to %s\033[0m\n", filename)
		}

	case "/clear", "/c":
		s.conversation = []Message{}
		s.lastCode = ""
		s.lastValidated = false
		s.tokenTracker.Reset()
		fmt.Println("Conversation and token budget cleared.")

	case "/validate", "/v":
		if len(parts) < 2 {
			fmt.Println("\033[91mUsage:\033[0m /validate <filename>")
			return true
		}
		filename := parts[1]
		s.validateFile(ctx, filename)

	case "/code", "/show":
		if s.lastCode == "" {
			fmt.Println("No code generated yet.")
		} else {
			fmt.Println("\n\033[93mLast generated code:\033[0m")
			fmt.Println("```cpp")
			fmt.Println(s.lastCode)
			fmt.Println("```")
		}

	case "/tokens", "/t":
		input, output, total := s.tokenTracker.GetUsage()
		fmt.Printf("\n%s\n", s.theme.Warning("Token Usage:"))
		fmt.Printf("  Input tokens:  %d\n", input)
		fmt.Printf("  Output tokens: %d\n", output)
		fmt.Printf("  Total tokens:  %d\n", total)
		if s.config.MaxTotalTokens > 0 {
			remaining := s.config.MaxTotalTokens - total
			pct := total * 100 / s.config.MaxTotalTokens
			fmt.Printf("  Budget used:   %d%% (%d remaining)\n", pct, remaining)
		} else {
			fmt.Printf("  Budget:        unlimited\n")
		}
		fmt.Println()

	case "/config":
		s.showConfig()

	case "/theme":
		if len(parts) < 2 {
			fmt.Printf("Current theme: %s\n", s.config.Settings.Theme.Name)
			fmt.Printf("Available themes: %s\n", strings.Join(AvailableThemes(), ", "))
			return true
		}
		themeName := strings.ToLower(parts[1])
		if _, ok := ThemePresets[themeName]; !ok {
			fmt.Printf("%s Unknown theme: %s\n", s.theme.Error("Error:"), themeName)
			fmt.Printf("Available themes: %s\n", strings.Join(AvailableThemes(), ", "))
			return true
		}
		s.config.Settings.Theme.Name = themeName
		s.theme = NewTheme(&s.config.Settings.Theme)
		s.config.Theme = s.theme
		if err := SaveSettings(s.config.Settings); err != nil {
			fmt.Printf("%s Could not save settings: %v\n", s.theme.Warning("Warning:"), err)
		} else {
			fmt.Printf("%s Theme changed to %s (saved)\n", s.theme.Success("✓"), themeName)
		}

	default:
		fmt.Printf("\033[91mUnknown command:\033[0m %s\n", cmd)
		fmt.Println("Type /help for available commands.")
	}

	return true
}

func printCommandHelp() {
	fmt.Println("")
	fmt.Println("Available Commands:")
	fmt.Println("  /help, /h              Show this help")
	fmt.Println("  /save <file>, /s       Save last generated code to file")
	fmt.Println("  /clear, /c             Clear conversation and token budget")
	fmt.Println("  /validate <file>, /v   Validate existing file (without generation)")
	fmt.Println("  /code, /show           Show last generated code")
	fmt.Println("  /tokens, /t            Show token usage and budget")
	fmt.Println("  /config                Show current configuration")
	fmt.Println("  /theme [name]          Show or change theme (default, matrix, solarized, gruvbox, dracula, nord)")
	fmt.Println("  /quit, /q              Exit bjarne")
	fmt.Println("")
	fmt.Println("Tips:")
	fmt.Println("  - Just type your request to generate C/C++ code")
	fmt.Println("  - All generated code is automatically validated")
	fmt.Println("  - Failed validations automatically escalate to more powerful models")
	fmt.Println("  - Use /save to write validated code to a file")
	fmt.Println("")
}

// handlePrompt processes a code generation request with reflection and automatic iteration
func (s *Session) handlePrompt(ctx context.Context, prompt string) error {
	// Visual spacing after user input
	fmt.Println()

	// Phase 1: Reflection - bjarne thinks about the request
	thinkSpinner := NewThinkingSpinner("Thinking…", s.theme)
	thinkSpinner.Start()

	// Add user message to conversation
	s.conversation = append(s.conversation, Message{
		Role:    "user",
		Content: prompt,
	})

	// Call with reflection prompt using chat model
	reflectResult, err := s.bedrock.GenerateWithModel(ctx, s.config.ChatModel, ReflectionSystemPrompt, s.conversation, s.config.MaxTokens)
	if err != nil {
		thinkSpinner.Fail("Reflection failed")
		return fmt.Errorf("reflection failed: %w", err)
	}

	// Update spinner with token count before stopping
	thinkSpinner.UpdateTokens(reflectResult.OutputTokens)
	thinkSpinner.Stop()

	// Track tokens
	ok, tokenMsg := s.tokenTracker.Add(reflectResult.InputTokens, reflectResult.OutputTokens)
	if tokenMsg != "" {
		fmt.Printf("%s\n", s.theme.Warning(tokenMsg))
	}
	if !ok {
		return fmt.Errorf("token budget exceeded")
	}

	// Add reflection response to conversation
	s.conversation = append(s.conversation, Message{
		Role:    "assistant",
		Content: reflectResult.Text,
	})

	// Parse difficulty and display bjarne's reflection with spacing
	difficulty, reflectionText := parseDifficulty(reflectResult.Text)
	fmt.Println()
	fmt.Printf("%s %s\n", s.theme.Accent("bjarne:"), reflectionText)

	// For EASY tasks, skip confirmation and proceed directly
	if difficulty == "EASY" {
		s.conversation = append(s.conversation, Message{
			Role:    "user",
			Content: GenerateNowPrompt,
		})
	} else {
		// Wait for user confirmation on MEDIUM/COMPLEX tasks
		fmt.Println()
		reader := bufio.NewReader(os.Stdin)
		fmt.Print(s.theme.PromptCode() + ">" + s.theme.Reset() + " ")
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}
		response = strings.TrimSpace(strings.ToLower(response))

		// Check for abort
		if response == "n" || response == "no" || response == "abort" || response == "cancel" {
			fmt.Println("\nCancelled.")
			return nil
		}

		// Add user confirmation to conversation
		userConfirm := response
		if userConfirm == "" || userConfirm == "y" || userConfirm == "yes" {
			userConfirm = "Yes, proceed."
		}
		s.conversation = append(s.conversation, Message{
			Role:    "user",
			Content: userConfirm + "\n\n" + GenerateNowPrompt,
		})
		fmt.Println() // Space after confirmation
	}

	// Phase 2: Generation
	fmt.Println() // Space before generation spinner
	genSpinner := NewThinkingSpinner(fmt.Sprintf("Generating with %s…", shortModelName(s.config.GenerateModel)), s.theme)
	genSpinner.Start()

	currentModel := s.config.GenerateModel
	result, err := s.bedrock.GenerateWithModel(ctx, currentModel, GenerationSystemPrompt, s.conversation, s.config.MaxTokens)
	if err != nil {
		genSpinner.Fail("Generation failed")
		return fmt.Errorf("generation failed: %w", err)
	}

	// Update spinner with tokens and stop
	genSpinner.UpdateTokens(result.OutputTokens)
	genSpinner.Success("Code generated")

	// Track tokens
	ok, tokenMsg = s.tokenTracker.Add(result.InputTokens, result.OutputTokens)
	if tokenMsg != "" {
		fmt.Printf("%s\n", s.theme.Warning(tokenMsg))
	}
	if !ok {
		return fmt.Errorf("token budget exceeded")
	}

	// Add assistant response to conversation
	s.conversation = append(s.conversation, Message{
		Role:    "assistant",
		Content: result.Text,
	})

	// Extract code from response
	code := extractCode(result.Text)
	if code == "" {
		// No code block found, just display response
		fmt.Printf("\n%s %s\n", s.theme.Info("bjarne:"), result.Text)
		return nil
	}

	// Validation loop with configurable max iterations and model escalation
	maxIter := s.config.MaxIterations
	escalationIndex := 0 // Track which escalation model to use next

	for iteration := 1; iteration <= maxIter; iteration++ {
		s.lastCode = code
		s.lastValidated = false

		// Run validation silently (code is hidden until validation passes)
		fmt.Printf("\n%s Validating", s.theme.Info("⠋"))
		if iteration > 1 {
			fmt.Printf(" (attempt %d/%d)", iteration, maxIter)
		}
		fmt.Println("...")

		results, failedErrors, allPassed := s.runValidationQuiet(ctx, code)

		if allPassed {
			s.lastValidated = true
			// Show success summary with code
			s.showValidationSuccess(code, results, iteration)
			return nil
		}

		// Check if we have iterations left
		if iteration >= maxIter {
			s.showValidationFailure(code, results, maxIter)
			return nil
		}

		// Show brief failure notice before retrying
		fmt.Printf("%s Validation failed, ", s.theme.Warning("↻"))

		// Determine which model to use for the fix attempt
		fixModel := currentModel
		if s.config.EscalateOnFailure && escalationIndex < len(s.config.EscalationModels) {
			fixModel = s.config.EscalationModels[escalationIndex]
			escalationIndex++
			fmt.Printf("escalating to %s...\n", shortModelName(fixModel))
		} else {
			fmt.Printf("retrying...\n")
		}

		iterationPrompt := fmt.Sprintf(IterationPromptTemplate, failedErrors)

		// Add iteration request to conversation
		s.conversation = append(s.conversation, Message{
			Role:    "user",
			Content: iterationPrompt,
		})

		// Show thinking spinner while generating fix
		fixSpinner := NewThinkingSpinner(fmt.Sprintf("Fixing with %s…", shortModelName(fixModel)), s.theme)
		fixSpinner.Start()

		iterResult, err := s.bedrock.GenerateWithModel(ctx, fixModel, GenerationSystemPrompt, s.conversation, s.config.MaxTokens)
		if err != nil {
			fixSpinner.Fail("Generation failed")
			return fmt.Errorf("iteration failed: %w", err)
		}
		fixSpinner.UpdateTokens(iterResult.OutputTokens)
		fixSpinner.Success("Fix generated")

		// Track tokens
		ok, tokenMsg := s.tokenTracker.Add(iterResult.InputTokens, iterResult.OutputTokens)
		if tokenMsg != "" {
			fmt.Printf("%s\n", s.theme.Warning(tokenMsg))
		}
		if !ok {
			fmt.Println(s.theme.Error("Token budget exceeded during iteration."))
			fmt.Println("Use /clear to start a new conversation.")
			return nil
		}

		// Add response to conversation
		s.conversation = append(s.conversation, Message{
			Role:    "assistant",
			Content: iterResult.Text,
		})

		// Extract new code
		newCode := extractCode(iterResult.Text)
		if newCode == "" {
			fmt.Println(s.theme.Error("No code in iteration response."))
			fmt.Printf("\n%s %s\n", s.theme.Info("bjarne:"), iterResult.Text)
			return nil
		}

		code = newCode
		currentModel = fixModel
	}

	return nil
}

// runValidationQuiet runs validation without progress output
func (s *Session) runValidationQuiet(ctx context.Context, code string) ([]ValidationResult, string, bool) {
	results, err := s.container.ValidateCode(ctx, code, "code.cpp")

	if err != nil {
		return nil, err.Error(), false
	}

	// Check results
	allPassed := true
	var failedErrors string
	for _, r := range results {
		if !r.Success {
			allPassed = false
			failedErrors += fmt.Sprintf("Stage %s failed:\n%s\n", r.Stage, r.Error)
		}
	}

	return results, failedErrors, allPassed
}

// showValidationSuccess displays the code and validation summary on success
func (s *Session) showValidationSuccess(code string, results []ValidationResult, attempts int) {
	// Clear the "Validating..." line
	fmt.Printf("\r\033[K")

	// Show the validated code
	fmt.Printf("\n%s\n", s.theme.Success("✓ Validated Code:"))
	fmt.Println("```cpp")
	fmt.Println(code)
	fmt.Println("```")

	// Show validation summary
	fmt.Printf("\n%s\n", s.theme.Success("Validation Summary"))
	fmt.Println(strings.Repeat("─", 50))

	totalTime := 0.0
	for _, r := range results {
		totalTime += r.Duration.Seconds()
		checkmark := s.theme.Success("☑")
		fmt.Printf("  %s %s %s(%.2fs)%s\n", checkmark, r.Stage, s.theme.Dim(""), r.Duration.Seconds(), s.theme.Reset())
	}

	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  %s All %d tests passed", s.theme.Success("✓"), len(results))
	if attempts > 1 {
		fmt.Printf(" (after %d attempts)", attempts)
	}
	fmt.Printf(" %s[%.2fs total]%s\n", s.theme.Dim(""), totalTime, s.theme.Reset())
	fmt.Println()
	fmt.Printf("Use %s to save the validated code\n", s.theme.Accent("/save <filename>"))
}

// showValidationFailure displays the failure summary
func (s *Session) showValidationFailure(code string, results []ValidationResult, attempts int) {
	fmt.Printf("\r\033[K")
	fmt.Printf("\n%s\n", s.theme.Error(fmt.Sprintf("✗ Validation failed after %d attempts", attempts)))
	fmt.Println(strings.Repeat("─", 50))

	for _, r := range results {
		if r.Success {
			fmt.Printf("  %s %s %s(%.2fs)%s\n", s.theme.Success("☑"), r.Stage, s.theme.Dim(""), r.Duration.Seconds(), s.theme.Reset())
		} else {
			fmt.Printf("  %s %s %s(%.2fs)%s\n", s.theme.Error("☒"), r.Stage, s.theme.Dim(""), r.Duration.Seconds(), s.theme.Reset())
			// Show truncated error
			errLines := strings.Split(r.Error, "\n")
			if len(errLines) > 0 && errLines[0] != "" {
				fmt.Printf("      %s%s%s\n", s.theme.Dim(""), truncateError(errLines[0], 60), s.theme.Reset())
			}
		}
	}

	fmt.Println(strings.Repeat("─", 50))
	fmt.Println()

	// Still show the code so user can see what failed
	fmt.Printf("%s\n", s.theme.Warning("Last generated code (failed validation):"))
	fmt.Println("```cpp")
	fmt.Println(code)
	fmt.Println("```")
	fmt.Println()
	fmt.Println("You can ask me to fix specific issues.")
}

// runValidationWithSpinner runs validation with a spinner UI for each stage
func (s *Session) runValidationWithSpinner(ctx context.Context, code string) ([]ValidationResult, string, bool) {
	results, err := s.container.ValidateCodeWithProgress(ctx, code, "code.cpp", func(stage string, running bool, result *ValidationResult) {
		if running {
			// Stage starting - show spinner would go here but we use simpler approach
			fmt.Printf("  %s %s...", s.theme.Info("⠋"), stage)
		} else if result != nil {
			// Stage complete
			fmt.Printf("\r\033[K") // Clear line
			if result.Success {
				fmt.Printf("  %s %s (%.2fs)\n", s.theme.Success("✓"), stage, result.Duration.Seconds())
			} else {
				fmt.Printf("  %s %s (%.2fs)\n", s.theme.Error("✗"), stage, result.Duration.Seconds())
			}
		}
	})

	if err != nil {
		fmt.Printf("  %s validation error: %v\n", s.theme.Error("✗"), err)
		return nil, err.Error(), false
	}

	// Check results
	allPassed := true
	var failedErrors string
	for _, r := range results {
		if !r.Success {
			allPassed = false
			failedErrors += fmt.Sprintf("Stage %s failed:\n%s\n", r.Stage, r.Error)
		}
	}

	return results, failedErrors, allPassed
}

// truncateError truncates an error message to maxLen characters
func truncateError(err string, maxLen int) string {
	// Get first line only
	if idx := strings.Index(err, "\n"); idx > 0 {
		err = err[:idx]
	}
	if len(err) > maxLen {
		return err[:maxLen-3] + "..."
	}
	return err
}

// parseDifficulty extracts the difficulty tag from bjarne's reflection
// Returns the difficulty level (EASY, MEDIUM, COMPLEX) and the text without the tag
func parseDifficulty(text string) (string, string) {
	text = strings.TrimSpace(text)

	// Check for difficulty tags at the start
	for _, level := range []string{"EASY", "MEDIUM", "COMPLEX"} {
		tag := "[" + level + "]"
		if strings.HasPrefix(text, tag) {
			// Remove the tag and any following whitespace/newline
			remainder := strings.TrimPrefix(text, tag)
			remainder = strings.TrimLeft(remainder, " \t\n")
			return level, remainder
		}
	}

	// No tag found - default to MEDIUM (requires confirmation)
	return "MEDIUM", text
}

// extractCode extracts code from a markdown code block
func extractCode(response string) string {
	// Match ```cpp ... ``` or ```c ... ``` or ``` ... ```
	re := regexp.MustCompile("(?s)```(?:cpp|c|c\\+\\+)?\\s*\\n(.*?)\\n```")
	matches := re.FindStringSubmatch(response)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// saveToFile writes code to a file
func saveToFile(filename, code string) error {
	return os.WriteFile(filename, []byte(code), 0600)
}

// showConfig displays current configuration
func (s *Session) showConfig() {
	fmt.Println()
	fmt.Println(s.theme.Warning("Current Configuration:"))
	fmt.Println()

	fmt.Println("  Models:")
	fmt.Printf("    Chat:     %s\n", shortModelName(s.config.ChatModel))
	fmt.Printf("    Generate: %s\n", shortModelName(s.config.GenerateModel))
	if len(s.config.EscalationModels) > 0 {
		fmt.Println("    Escalation:")
		for i, m := range s.config.EscalationModels {
			fmt.Printf("      %d. %s\n", i+1, shortModelName(m))
		}
	}
	fmt.Println()

	fmt.Println("  Validation:")
	fmt.Printf("    Max iterations:  %d\n", s.config.MaxIterations)
	fmt.Printf("    Escalate on fail: %v\n", s.config.EscalateOnFailure)
	fmt.Printf("    Container image: %s\n", s.config.ValidatorImage)
	fmt.Println()

	fmt.Println("  Tokens:")
	fmt.Printf("    Max per response: %d\n", s.config.MaxTokens)
	if s.config.MaxTotalTokens > 0 {
		fmt.Printf("    Max per session:  %d\n", s.config.MaxTotalTokens)
	} else {
		fmt.Printf("    Max per session:  unlimited\n")
	}
	fmt.Println()

	fmt.Println("  Theme:")
	fmt.Printf("    Current: %s\n", s.config.Settings.Theme.Name)
	fmt.Printf("    Available: %s\n", strings.Join(AvailableThemes(), ", "))
	fmt.Println()

	path, err := SettingsPath()
	if err == nil {
		fmt.Printf("  Settings file: %s\n", path)
	}
	fmt.Println()
}

// shortModelName extracts a readable model name from the full ID
func shortModelName(modelID string) string {
	// global.anthropic.claude-haiku-4-5-20251001-v1:0 -> claude-haiku-4-5
	parts := strings.Split(modelID, ".")
	if len(parts) >= 3 {
		modelPart := parts[2] // claude-haiku-4-5-20251001-v1:0
		// Remove version suffix like -20251001-v1:0
		if idx := strings.Index(modelPart, "-202"); idx > 0 {
			return modelPart[:idx]
		}
		return modelPart
	}
	return modelID
}

// validateFile validates an existing file without AI generation
func (s *Session) validateFile(ctx context.Context, filename string) {
	// Read the file
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("\033[91mError:\033[0m Could not read file: %v\n", err)
		return
	}

	code := string(content)
	if code == "" {
		fmt.Println("\033[91mError:\033[0m File is empty")
		return
	}

	fmt.Printf("\n\033[93mValidating %s...\033[0m\n", filename)

	// Run validation pipeline
	results, err := s.container.ValidateCode(ctx, code, filename)
	if err != nil {
		fmt.Printf("\033[91mValidation error:\033[0m %v\n", err)
		return
	}

	fmt.Println(FormatResults(results))

	// Check if all passed
	allPassed := true
	for _, r := range results {
		if !r.Success {
			allPassed = false
			break
		}
	}

	if allPassed {
		s.lastCode = code
		s.lastValidated = true
		fmt.Printf("\033[92m✓ %s passed all validation!\033[0m\n", filename)
	} else {
		s.lastCode = code
		s.lastValidated = false
		fmt.Println("\033[93mValidation failed.\033[0m Fix the issues and try again.")
	}
}
