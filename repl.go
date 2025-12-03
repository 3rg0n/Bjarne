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
	conversation  []Message
	lastCode      string
	lastValidated bool
}

// Run starts the interactive REPL loop
func Run() error {
	ctx := context.Background()

	fmt.Printf("bjarne %s\n", Version)
	fmt.Println("AI-assisted C/C++ code generation with mandatory validation")
	fmt.Println()

	// Initialize container runtime
	fmt.Println("Detecting container runtime...")
	container, err := DetectContainerRuntime()
	if err != nil {
		return fmt.Errorf("failed to detect container runtime: %w", err)
	}
	fmt.Printf("Using container runtime: %s\n", container.GetBinary())

	// Check if validation image exists
	if !container.ImageExists(ctx) {
		fmt.Printf("Validation container not found. Pull %s? [Y/n] ", container.imageName)
		// For now, auto-pull (can make interactive later)
		fmt.Println("Pulling...")
		if err := container.PullImage(ctx); err != nil {
			fmt.Printf("\033[93mWarning:\033[0m Could not pull image: %v\n", err)
			fmt.Println("         Validation will be skipped until container is available.")
		}
	}
	fmt.Println()

	// Initialize Bedrock client
	fmt.Println("Connecting to AWS Bedrock...")
	bedrock, err := NewBedrockClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize Bedrock client: %w", err)
	}
	fmt.Printf("Using model: %s\n", bedrock.GetModelID())
	fmt.Println()
	fmt.Println("Type /help for commands, /quit to exit")
	fmt.Println()

	session := &Session{
		bedrock:      bedrock,
		container:    container,
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
		fmt.Println("Conversation cleared.")

	case "/validate", "/v":
		if len(parts) < 2 {
			fmt.Println("\033[91mUsage:\033[0m /validate <filename>")
			return true
		}
		filename := parts[1]
		fmt.Printf("Validating %s... (not yet implemented - needs container)\n", filename)

	case "/code", "/show":
		if s.lastCode == "" {
			fmt.Println("No code generated yet.")
		} else {
			fmt.Println("\n\033[93mLast generated code:\033[0m")
			fmt.Println("```cpp")
			fmt.Println(s.lastCode)
			fmt.Println("```")
		}

	default:
		fmt.Printf("\033[91mUnknown command:\033[0m %s\n", cmd)
		fmt.Println("Type /help for available commands.")
	}

	return true
}

func printCommandHelp() {
	fmt.Println(`
Available Commands:
  /help, /h              Show this help
  /save <file>, /s       Save last generated code to file
  /clear, /c             Clear conversation history
  /validate <file>, /v   Validate existing file (without generation)
  /code, /show           Show last generated code
  /quit, /q              Exit bjarne

Tips:
  - Just type your request to generate C/C++ code
  - All generated code is automatically validated
  - Use /save to write validated code to a file
`)
}

// handlePrompt processes a code generation request
func (s *Session) handlePrompt(ctx context.Context, prompt string) error {
	fmt.Println("\033[93mbjarne:\033[0m Generating...")

	// Add user message to conversation
	s.conversation = append(s.conversation, Message{
		Role:    "user",
		Content: prompt,
	})

	// Call Bedrock
	response, err := s.bedrock.Generate(ctx, SystemPrompt, s.conversation)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	// Add assistant response to conversation
	s.conversation = append(s.conversation, Message{
		Role:    "assistant",
		Content: response,
	})

	// Extract code from response
	code := extractCode(response)
	if code != "" {
		s.lastCode = code
		s.lastValidated = false

		fmt.Println("\n\033[93mGenerated code:\033[0m")
		fmt.Println("```cpp")
		fmt.Println(code)
		fmt.Println("```")

		// Run validation pipeline
		fmt.Println("\n\033[93mValidating...\033[0m")
		results, err := s.container.ValidateCode(ctx, code, "code.cpp")
		if err != nil {
			fmt.Printf("\033[91mValidation error:\033[0m %v\n", err)
		} else {
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
				s.lastValidated = true
				fmt.Println("Use /save <filename> to save the validated code")
			} else {
				fmt.Println("\033[93mCode failed validation.\033[0m Ask me to fix the issues.")
			}
		}
	} else {
		// No code block found, just display response
		fmt.Printf("\n\033[93mbjarne:\033[0m %s\n", response)
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
	return os.WriteFile(filename, []byte(code), 0644)
}
