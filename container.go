package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ContainerRuntime represents a container runtime (podman or docker)
type ContainerRuntime struct {
	binary    string // "podman" or "docker"
	imageName string // e.g., "bjarne-validator:latest" or "ghcr.io/3rg0n/bjarne-validator:latest"
}

// DetectContainerRuntime finds an available container runtime
// Preference: podman > docker (per ADR-005)
func DetectContainerRuntime() (*ContainerRuntime, error) {
	// Try podman first (preferred - daemonless, rootless)
	if path, err := exec.LookPath("podman"); err == nil {
		return &ContainerRuntime{
			binary:    path,
			imageName: getImageName(),
		}, nil
	}

	// Fall back to docker
	if path, err := exec.LookPath("docker"); err == nil {
		return &ContainerRuntime{
			binary:    path,
			imageName: getImageName(),
		}, nil
	}

	return nil, ErrNoContainerRuntime()
}

// getImageName returns the container image to use
func getImageName() string {
	// Check for local development override
	if img := os.Getenv("BJARNE_VALIDATOR_IMAGE"); img != "" {
		return img
	}
	// Default to ghcr.io hosted image
	return "bjarne-validator:local"
}

// GetBinary returns the container runtime binary name
func (c *ContainerRuntime) GetBinary() string {
	return filepath.Base(c.binary)
}

// ImageExists checks if the validation container image exists locally
func (c *ContainerRuntime) ImageExists(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, c.binary, "image", "inspect", c.imageName)
	return cmd.Run() == nil
}

// PullImage pulls the validation container image
func (c *ContainerRuntime) PullImage(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.binary, "pull", c.imageName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ValidationResult holds the result of a validation run
type ValidationResult struct {
	Stage    string // "clang-tidy", "compile", "asan", "ubsan", "tsan", "run"
	Success  bool
	Output   string
	Error    string
	Duration time.Duration
}

// ProgressCallback is called during validation to report progress
type ProgressCallback func(stage string, running bool, result *ValidationResult)

// ValidateCode runs the full validation pipeline on a code string
func (c *ContainerRuntime) ValidateCode(ctx context.Context, code string, filename string) ([]ValidationResult, error) {
	return c.ValidateCodeWithProgress(ctx, code, filename, nil)
}

// ValidateCodeWithExamples runs validation including example-based tests
func (c *ContainerRuntime) ValidateCodeWithExamples(ctx context.Context, code string, filename string, examples *ExampleTests, progress ProgressCallback) ([]ValidationResult, error) {
	// If we have example tests, generate a test harness and add example validation
	if examples != nil && len(examples.Tests) > 0 {
		// First, validate the original code through normal pipeline
		results, err := c.ValidateCodeWithProgress(ctx, code, filename, progress)
		if err != nil {
			return results, err
		}

		// Check if normal validation passed
		allPassed := true
		for _, r := range results {
			if !r.Success {
				allPassed = false
				break
			}
		}
		if !allPassed {
			return results, nil // Fail fast on normal validation
		}

		// Generate test harness and run example tests
		harness := GenerateTestHarness(code, examples)
		harnessFilename := "test_harness.cpp"

		// Create temp directory for harness
		tmpDir, err := os.MkdirTemp("", "bjarne-examples-*")
		if err != nil {
			return results, fmt.Errorf("failed to create temp dir for examples: %w", err)
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()

		// Write harness
		harnessPath := filepath.Join(tmpDir, harnessFilename)
		if err := os.WriteFile(harnessPath, []byte(harness), 0600); err != nil {
			return results, fmt.Errorf("failed to write harness: %w", err)
		}

		// Run example tests
		if progress != nil {
			progress("examples", true, nil)
		}
		result := c.runValidationStage(ctx, tmpDir, "examples",
			"sh", "-c",
			"clang++ -std=c++17 -o /tmp/test_harness /src/"+harnessFilename+" && /tmp/test_harness")
		if progress != nil {
			progress("examples", false, &result)
		}
		results = append(results, result)

		return results, nil
	}

	// No examples - run normal validation
	return c.ValidateCodeWithProgress(ctx, code, filename, progress)
}

// ValidateCodeWithProgress runs the full validation pipeline with progress callbacks
func (c *ContainerRuntime) ValidateCodeWithProgress(ctx context.Context, code string, filename string, progress ProgressCallback) ([]ValidationResult, error) {
	// Create temp directory for the code
	tmpDir, err := os.MkdirTemp("", "bjarne-validate-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Write code to temp file
	codePath := filepath.Join(tmpDir, filename)
	if err := os.WriteFile(codePath, []byte(code), 0600); err != nil {
		return nil, fmt.Errorf("failed to write code file: %w", err)
	}

	var results []ValidationResult

	// Helper to run a stage with progress
	runStage := func(stage string, command ...string) ValidationResult {
		if progress != nil {
			progress(stage, true, nil)
		}
		result := c.runValidationStage(ctx, tmpDir, stage, command...)
		if progress != nil {
			progress(stage, false, &result)
		}
		return result
	}

	// Stage 1: clang-tidy (static analysis)
	result := runStage("clang-tidy",
		"clang-tidy", "-header-filter=.*", "/src/"+filename, "--", "-std=c++17", "-Wall", "-Wextra")
	results = append(results, result)
	if !result.Success {
		return results, nil // Fail fast
	}

	// Stage 2: cppcheck (deep static analysis - catches things clang-tidy misses)
	result = runStage("cppcheck",
		"cppcheck", "--enable=all", "--error-exitcode=1", "--suppress=missingIncludeSystem",
		"--std=c++17", "/src/"+filename)
	results = append(results, result)
	if !result.Success {
		return results, nil
	}

	// Stage 3: IWYU (Include What You Use) - check header hygiene
	// IWYU always returns non-zero, so we check for actual suggestions in output
	result = runStage("iwyu",
		"sh", "-c",
		"include-what-you-use -std=c++17 /src/"+filename+" 2>&1; exit 0")
	// IWYU is advisory - we mark success if it ran, the suggestions are informational
	result.Success = true
	results = append(results, result)

	// Stage 4: Complexity metrics (lizard)
	// Fail if cyclomatic complexity > 15 or function length > 100 lines
	result = runStage("complexity",
		"sh", "-c",
		"lizard -C 15 -L 100 -w /src/"+filename)
	results = append(results, result)
	if !result.Success {
		return results, nil
	}

	// Stage 5: Compile with strict warnings
	result = runStage("compile",
		"clang++", "-std=c++17", "-Wall", "-Wextra", "-Werror",
		"-fstack-protector-all", "-D_FORTIFY_SOURCE=2",
		"-o", "/tmp/test", "/src/"+filename)
	results = append(results, result)
	if !result.Success {
		return results, nil
	}

	// Stage 6: ASAN (AddressSanitizer)
	result = runStage("asan",
		"sh", "-c",
		"clang++ -std=c++17 -fsanitize=address -fno-omit-frame-pointer -g -o /tmp/test /src/"+filename+" && /tmp/test")
	results = append(results, result)
	if !result.Success {
		return results, nil
	}

	// Stage 7: UBSAN (UndefinedBehaviorSanitizer)
	result = runStage("ubsan",
		"sh", "-c",
		"clang++ -std=c++17 -fsanitize=undefined -fno-omit-frame-pointer -g -o /tmp/test /src/"+filename+" && /tmp/test")
	results = append(results, result)
	if !result.Success {
		return results, nil
	}

	// Stage 8: Check if code uses threads, run TSAN if so
	if codeUsesThreads(code) {
		result = runStage("tsan",
			"sh", "-c",
			"clang++ -std=c++17 -fsanitize=thread -fno-omit-frame-pointer -g -o /tmp/test /src/"+filename+" && /tmp/test")
		results = append(results, result)
		if !result.Success {
			return results, nil
		}
	}

	// Stage 9: Final run (clean execution)
	result = runStage("run",
		"sh", "-c",
		"clang++ -std=c++17 -O2 -o /tmp/test /src/"+filename+" && /tmp/test")
	results = append(results, result)

	return results, nil
}

// runValidationStage runs a single validation stage in the container
func (c *ContainerRuntime) runValidationStage(ctx context.Context, tmpDir, stage string, command ...string) ValidationResult {
	start := time.Now()

	// Build container run command
	// Note: We don't use --read-only because sanitizers need to write to /tmp
	// Security is maintained via --network none and read-only source mount
	// seccomp=unconfined is required for TSAN to work (needs ptrace/ASLR control)
	args := []string{
		"run", "--rm",
		"--network", "none", // No network access
		"--security-opt", "seccomp=unconfined", // Required for TSAN
		"-v", tmpDir + ":/src:ro", // Mount code read-only
		"--timeout", "120", // 2 minute timeout
		c.imageName,
	}
	args = append(args, command...)

	cmd := exec.CommandContext(ctx, c.binary, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	result := ValidationResult{
		Stage:    stage,
		Duration: duration,
		Output:   stdout.String(),
	}

	if err != nil {
		result.Success = false
		result.Error = stderr.String()
		if result.Error == "" {
			result.Error = err.Error()
		}
	} else {
		result.Success = true
	}

	return result
}

// codeUsesThreads checks if the code appears to use threading
func codeUsesThreads(code string) bool {
	threadIndicators := []string{
		"<thread>",
		"<pthread.h>",
		"std::thread",
		"std::mutex",
		"std::atomic",
		"std::async",
		"std::future",
		"pthread_create",
		"pthread_mutex",
	}

	for _, indicator := range threadIndicators {
		if strings.Contains(code, indicator) {
			return true
		}
	}
	return false
}

// FormatResults formats validation results for display
func FormatResults(results []ValidationResult) string {
	var sb strings.Builder

	allPassed := true
	for _, r := range results {
		if r.Success {
			sb.WriteString(fmt.Sprintf("PASS %s (%.2fs)\n", r.Stage, r.Duration.Seconds()))
		} else {
			allPassed = false
			sb.WriteString(fmt.Sprintf("FAIL %s (%.2fs)\n", r.Stage, r.Duration.Seconds()))
			if r.Error != "" {
				// Parse and format diagnostics based on stage type
				formatted := formatStageError(r.Stage, r.Error)
				sb.WriteString(formatted)
			}
		}
	}

	if allPassed {
		sb.WriteString("\nAll validation stages passed!\n")
	}

	return sb.String()
}

// formatStageError parses and formats error output based on stage type
func formatStageError(stage, errorOutput string) string {
	switch stage {
	case "clang-tidy":
		diags := ParseClangTidyOutput(errorOutput)
		if len(diags) > 0 {
			return FormatDiagnostics(diags)
		}
	case "cppcheck":
		diags := ParseCppcheckOutput(errorOutput)
		if len(diags) > 0 {
			return FormatDiagnostics(diags)
		}
	case "complexity":
		// Lizard output is already human-readable, just indent it
		// No special parsing needed
	case "asan":
		diags := ParseSanitizerOutput(errorOutput, "asan")
		if len(diags) > 0 {
			return FormatDiagnostics(diags)
		}
	case "ubsan":
		diags := ParseSanitizerOutput(errorOutput, "ubsan")
		if len(diags) > 0 {
			return FormatDiagnostics(diags)
		}
	case "tsan":
		diags := ParseSanitizerOutput(errorOutput, "tsan")
		if len(diags) > 0 {
			return FormatDiagnostics(diags)
		}
	}

	// Fallback: indent raw output
	var sb strings.Builder
	lines := strings.Split(strings.TrimSpace(errorOutput), "\n")
	for _, line := range lines {
		sb.WriteString(fmt.Sprintf("  %s\n", line))
	}
	return sb.String()
}
