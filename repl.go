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
	tokenTracker  *TokenTracker
	conversation  []Message
	lastCode      string
	lastValidated bool
}

// handleFirstRunPull handles the first-run container pull experience
func handleFirstRunPull(ctx context.Context, container *ContainerRuntime) error {
	fmt.Println()
	fmt.Println("\033[93m┌─────────────────────────────────────────────────────────────┐\033[0m")
	fmt.Println("\033[93m│                    First-time Setup                          │\033[0m")
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
	bedrock, err := NewBedrockClient(ctx)
	if err != nil {
		fmt.Print(FormatUserError(err))
		return err
	}
	fmt.Printf("Using model: %s\n", bedrock.GetModelID())
	fmt.Println()
	fmt.Println("Type /help for commands, /quit to exit")
	fmt.Println()

	session := &Session{
		bedrock:      bedrock,
		container:    container,
		config:       cfg,
		tokenTracker: NewTokenTracker(cfg.MaxTotalTokens, cfg.WarnTokenThreshold),
		conversation: []Message{},
	}

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("\033[94mYou:\033[0m ")

		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Handle slash commands
		if strings.HasPrefix(input, "/") {
			if !session.handleCommand(ctx, input) {
				break
			}
			continue
		}

		// Handle code generation request
		if err := session.handlePrompt(ctx, input); err != nil {
			fmt.Fprintf(os.Stderr, "\033[91mError:\033[0m %v\n", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("input error: %w", err)
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
		fmt.Printf("\n\033[93mToken Usage:\033[0m\n")
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
	fmt.Println("  /quit, /q              Exit bjarne")
	fmt.Println("")
	fmt.Println("Tips:")
	fmt.Println("  - Just type your request to generate C/C++ code")
	fmt.Println("  - All generated code is automatically validated")
	fmt.Println("  - Use /save to write validated code to a file")
	fmt.Println("")
}

// handlePrompt processes a code generation request with automatic iteration
func (s *Session) handlePrompt(ctx context.Context, prompt string) error {
	fmt.Println("\033[93mbjarne:\033[0m Generating...")

	// Add user message to conversation
	s.conversation = append(s.conversation, Message{
		Role:    "user",
		Content: prompt,
	})

	// Call Bedrock with token tracking
	result, err := s.bedrock.GenerateWithTokens(ctx, SystemPrompt, s.conversation, s.config.MaxTokens)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	// Track tokens
	ok, tokenMsg := s.tokenTracker.Add(result.InputTokens, result.OutputTokens)
	if tokenMsg != "" {
		fmt.Printf("\033[93m%s\033[0m\n", tokenMsg)
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
		fmt.Printf("\n\033[93mbjarne:\033[0m %s\n", result.Text)
		return nil
	}

	// Validation loop with configurable max iterations
	maxIter := s.config.MaxIterations
	for iteration := 1; iteration <= maxIter; iteration++ {
		s.lastCode = code
		s.lastValidated = false

		if iteration > 1 {
			fmt.Printf("\n\033[93mIteration %d/%d:\033[0m\n", iteration, maxIter)
		}

		fmt.Println("\n\033[93mGenerated code:\033[0m")
		fmt.Println("```cpp")
		fmt.Println(code)
		fmt.Println("```")

		// Run validation pipeline
		fmt.Println("\n\033[93mValidating...\033[0m")
		results, err := s.container.ValidateCode(ctx, code, "code.cpp")
		if err != nil {
			fmt.Printf("\033[91mValidation error:\033[0m %v\n", err)
			return nil
		}

		fmt.Println(FormatResults(results))

		// Check if all passed
		allPassed := true
		var failedErrors string
		for _, r := range results {
			if !r.Success {
				allPassed = false
				failedErrors += fmt.Sprintf("Stage %s failed:\n%s\n", r.Stage, r.Error)
			}
		}

		if allPassed {
			s.lastValidated = true
			fmt.Println("\033[92m✓ All validation passed!\033[0m")
			fmt.Println("Use /save <filename> to save the validated code")
			return nil
		}

		// Check if we have iterations left
		if iteration >= maxIter {
			fmt.Printf("\033[91mValidation failed after %d attempts.\033[0m\n", maxIter)
			fmt.Println("You can manually ask me to fix specific issues.")
			return nil
		}

		// Automatically iterate: ask Claude to fix the errors
		fmt.Printf("\033[93mValidation failed. Attempting fix (%d/%d)...\033[0m\n", iteration+1, maxIter)

		iterationPrompt := fmt.Sprintf(IterationPromptTemplate, failedErrors)

		// Add iteration request to conversation
		s.conversation = append(s.conversation, Message{
			Role:    "user",
			Content: iterationPrompt,
		})

		// Call Bedrock for fix with token tracking
		iterResult, err := s.bedrock.GenerateWithTokens(ctx, SystemPrompt, s.conversation, s.config.MaxTokens)
		if err != nil {
			return fmt.Errorf("iteration failed: %w", err)
		}

		// Track tokens
		ok, tokenMsg := s.tokenTracker.Add(iterResult.InputTokens, iterResult.OutputTokens)
		if tokenMsg != "" {
			fmt.Printf("\033[93m%s\033[0m\n", tokenMsg)
		}
		if !ok {
			fmt.Println("\033[91mToken budget exceeded during iteration.\033[0m")
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
			fmt.Println("\033[91mNo code in iteration response.\033[0m")
			fmt.Printf("\n\033[93mbjarne:\033[0m %s\n", iterResult.Text)
			return nil
		}

		code = newCode
	}

	return nil
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
