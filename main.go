package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Version information (set via ldflags during build)
var (
	Version   = "dev"
	BuildDate = "unknown"
)

func main() {
	// Handle --version and --help flags
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-V":
			fmt.Printf("bjarne %s (built %s)\n", Version, BuildDate)
			fmt.Println("AI-assisted C/C++ code generation with mandatory validation")
			os.Exit(0)
		case "--help", "-h":
			printHelp()
			os.Exit(0)
		case "--validate", "-v":
			// Validate-only mode
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "Usage: bjarne --validate <file1.cpp> [file2.cpp ...]")
				os.Exit(1)
			}
			os.Exit(runValidateOnly(os.Args[2:]))
		}
	}

	// Start the REPL
	if err := Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runValidateOnly validates files without entering the REPL
func runValidateOnly(files []string) int {
	ctx := context.Background()

	// Initialize container runtime
	container, err := DetectContainerRuntime()
	if err != nil {
		fmt.Print(FormatUserError(err))
		return 1
	}
	fmt.Printf("Using container runtime: %s\n", container.GetBinary())

	// Check if validation image exists
	if !container.ImageExists(ctx) {
		fmt.Printf("\033[91mError:\033[0m Validation container not found.\n")
		fmt.Printf("       Run 'bjarne' interactively to pull the container first.\n")
		return 1
	}

	allPassed := true

	for _, filename := range files {
		// Read the file
		content, err := os.ReadFile(filename)
		if err != nil {
			fmt.Printf("\033[91m✗ %s:\033[0m %v\n", filename, err)
			allPassed = false
			continue
		}

		code := string(content)
		if code == "" {
			fmt.Printf("\033[91m✗ %s:\033[0m File is empty\n", filename)
			allPassed = false
			continue
		}

		fmt.Printf("\n\033[93mValidating %s...\033[0m\n", filename)

		// Get base filename for container
		baseName := filepath.Base(filename)

		// Run validation pipeline
		results, err := container.ValidateCode(ctx, code, baseName)
		if err != nil {
			fmt.Printf("\033[91m✗ %s:\033[0m %v\n", filename, err)
			allPassed = false
			continue
		}

		fmt.Print(FormatResults(results))

		// Check if all passed
		filePassed := true
		for _, r := range results {
			if !r.Success {
				filePassed = false
				allPassed = false
				break
			}
		}

		if filePassed {
			fmt.Printf("\033[92m✓ %s passed all validation!\033[0m\n", filename)
		}
	}

	if allPassed {
		fmt.Printf("\n\033[92m✓ All files passed validation!\033[0m\n")
		return 0
	}
	fmt.Printf("\n\033[91m✗ Some files failed validation.\033[0m\n")
	return 1
}

func printHelp() {
	fmt.Println(`bjarne - AI-assisted C/C++ code generation with mandatory validation

Usage:
  bjarne [flags]
  bjarne --validate <file1.cpp> [file2.cpp ...]

Flags:
  -h, --help           Show this help message
  -V, --version        Show version information
  -v, --validate       Validate files without entering REPL

Interactive Commands (in REPL):
  /help                Show available commands
  /save <file>         Save last generated code to file
  /validate <file>     Validate existing file without generation
  /clear               Clear conversation history
  /quit                Exit bjarne

Environment Variables:
  AWS_ACCESS_KEY_ID       AWS credentials for Bedrock
  AWS_SECRET_ACCESS_KEY   AWS credentials for Bedrock
  AWS_REGION              AWS region (default: us-east-1)
  BJARNE_MODEL            Claude model ID (default: uses inference profile)
  BJARNE_VALIDATOR_IMAGE  Custom validator container image
  BJARNE_MAX_ITERATIONS   Max validation retry attempts (default: 3)
  BJARNE_MAX_TOKENS       Max tokens per response (default: 8192)
  BJARNE_MAX_TOTAL_TOKENS Session token budget (default: 150000, 0=unlimited)

Examples:
  # Interactive mode
  $ bjarne
  You: write a thread-safe counter in C++
  bjarne: Generating... Validating... ✓
  [validated code displayed]
  You: /save counter.cpp

  # Validate-only mode
  $ bjarne --validate mycode.cpp
  $ bjarne -v file1.cpp file2.cpp file3.cpp

For more information: https://github.com/ecopelan/bjarne`)
}
