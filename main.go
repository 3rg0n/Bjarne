package main

import (
	"fmt"
	"os"
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
		case "--version", "-v":
			fmt.Printf("bjarne %s (built %s)\n", Version, BuildDate)
			fmt.Println("AI-assisted C/C++ code generation with mandatory validation")
			os.Exit(0)
		case "--help", "-h":
			printHelp()
			os.Exit(0)
		}
	}

	// Start the REPL
	if err := Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`bjarne - AI-assisted C/C++ code generation with mandatory validation

Usage:
  bjarne [flags]

Flags:
  -h, --help      Show this help message
  -v, --version   Show version information

Interactive Commands (in REPL):
  /help           Show available commands
  /save <file>    Save last generated code to file
  /clear          Clear conversation history
  /quit           Exit bjarne

Environment Variables:
  AWS_ACCESS_KEY_ID       AWS credentials for Bedrock
  AWS_SECRET_ACCESS_KEY   AWS credentials for Bedrock
  AWS_REGION              AWS region (default: us-east-1)
  BJARNE_MODEL            Claude model ID (default: uses inference profile)

Example:
  $ bjarne
  You: write a thread-safe counter in C++
  bjarne: Generating... Validating... ✓
  [validated code displayed]
  You: /save counter.cpp
  Saved to counter.cpp

For more information: https://github.com/ecopelan/bjarne`)
}
