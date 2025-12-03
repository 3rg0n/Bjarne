package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Run starts the interactive REPL loop
func Run() error {
	fmt.Printf("bjarne %s\n", Version)
	fmt.Println("AI-assisted C/C++ code generation with mandatory validation")
	fmt.Println("Type /help for commands, /quit to exit")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)

	// Conversation state
	var lastGeneratedCode string
	_ = lastGeneratedCode // Will be used when generation is implemented

	for {
		fmt.Print("\033[94mYou:\033[0m ")

		if !scanner.Scan() {
			// EOF or error
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Handle slash commands
		if strings.HasPrefix(input, "/") {
			if handleCommand(input, &lastGeneratedCode) {
				continue
			}
			// If handleCommand returns false, it means /quit
			break
		}

		// Handle code generation request
		if err := handlePrompt(input, &lastGeneratedCode); err != nil {
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
func handleCommand(input string, lastCode *string) bool {
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
		if *lastCode == "" {
			fmt.Println("\033[91mNo code to save.\033[0m Generate code first.")
			return true
		}
		filename := parts[1]
		if err := saveToFile(filename, *lastCode); err != nil {
			fmt.Fprintf(os.Stderr, "\033[91mError saving:\033[0m %v\n", err)
		} else {
			fmt.Printf("\033[92mSaved to %s\033[0m\n", filename)
		}

	case "/clear", "/c":
		// Clear conversation history (will be implemented with Bedrock integration)
		fmt.Println("Conversation cleared.")

	case "/validate", "/v":
		if len(parts) < 2 {
			fmt.Println("\033[91mUsage:\033[0m /validate <filename>")
			return true
		}
		filename := parts[1]
		fmt.Printf("Validating %s... (not yet implemented)\n", filename)
		// TODO: Implement validate-only mode

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
  /quit, /q              Exit bjarne

Tips:
  - Just type your request to generate C/C++ code
  - All generated code is automatically validated
  - Use /save to write validated code to a file
`)
}

// handlePrompt processes a code generation request
func handlePrompt(prompt string, lastCode *string) error {
	fmt.Println("\033[93mbjarne:\033[0m Generating...")

	// TODO: Call Bedrock to generate code
	// TODO: Validate generated code
	// TODO: Iterate if validation fails
	// TODO: Display validated code

	// Placeholder response
	fmt.Println("\033[93mbjarne:\033[0m Code generation not yet implemented.")
	fmt.Println("       Waiting for T-006 (Bedrock client) and T-010 (validation pipeline)")

	return nil
}

// saveToFile writes code to a file
func saveToFile(filename, code string) error {
	return os.WriteFile(filename, []byte(code), 0644)
}
