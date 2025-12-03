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
	imageName string // e.g., "bjarne-validator:latest" or "ghcr.io/ecopelan/bjarne-validator:latest"
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

	return nil, fmt.Errorf("no container runtime found: install podman or docker")
}

// getImageName returns the container image to use
func getImageName() string {
	// Check for local development override
	if img := os.Getenv("BJARNE_VALIDATOR_IMAGE"); img != "" {
		return img
	}
	// Default to ghcr.io hosted image
	return "ghcr.io/ecopelan/bjarne-validator:latest"
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
	Stage      string // "clang-tidy", "compile", "asan", "ubsan", "tsan", "run"
	Success    bool
	Output     string
	Error      string
	Duration   time.Duration
}

// ValidateCode runs the full validation pipeline on a code string
func (c *ContainerRuntime) ValidateCode(ctx context.Context, code string, filename string) ([]ValidationResult, error) {
	// Create temp directory for the code
	tmpDir, err := os.MkdirTemp("", "bjarne-validate-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write code to temp file
	codePath := filepath.Join(tmpDir, filename)
	if err := os.WriteFile(codePath, []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write code file: %w", err)
	}

	var results []ValidationResult

	// Stage 1: clang-tidy (static analysis)
	result := c.runValidationStage(ctx, tmpDir, filename, "clang-tidy",
		"clang-tidy", "/src/"+filename, "--", "-std=c++17", "-Wall", "-Wextra")
	results = append(results, result)
	if !result.Success {
		return results, nil // Fail fast
	}

	// Stage 2: Compile with strict warnings
	result = c.runValidationStage(ctx, tmpDir, filename, "compile",
		"clang++", "-std=c++17", "-Wall", "-Wextra", "-Werror",
		"-fstack-protector-all", "-D_FORTIFY_SOURCE=2",
		"-o", "/tmp/test", "/src/"+filename)
	results = append(results, result)
	if !result.Success {
		return results, nil
	}

	// Stage 3: ASAN (AddressSanitizer)
	result = c.runValidationStage(ctx, tmpDir, filename, "asan",
		"sh", "-c",
		"clang++ -std=c++17 -fsanitize=address -fno-omit-frame-pointer -g -o /tmp/test /src/"+filename+" && /tmp/test")
	results = append(results, result)
	if !result.Success {
		return results, nil
	}

	// Stage 4: UBSAN (UndefinedBehaviorSanitizer)
	result = c.runValidationStage(ctx, tmpDir, filename, "ubsan",
		"sh", "-c",
		"clang++ -std=c++17 -fsanitize=undefined -fno-omit-frame-pointer -g -o /tmp/test /src/"+filename+" && /tmp/test")
	results = append(results, result)
	if !result.Success {
		return results, nil
	}

	// Stage 5: Check if code uses threads, run TSAN if so
	if codeUsesThreads(code) {
		result = c.runValidationStage(ctx, tmpDir, filename, "tsan",
			"sh", "-c",
			"clang++ -std=c++17 -fsanitize=thread -fno-omit-frame-pointer -g -o /tmp/test /src/"+filename+" && /tmp/test")
		results = append(results, result)
		if !result.Success {
			return results, nil
		}
	}

	// Stage 6: Final run (clean execution)
	result = c.runValidationStage(ctx, tmpDir, filename, "run",
		"sh", "-c",
		"clang++ -std=c++17 -O2 -o /tmp/test /src/"+filename+" && /tmp/test")
	results = append(results, result)

	return results, nil
}

// runValidationStage runs a single validation stage in the container
func (c *ContainerRuntime) runValidationStage(ctx context.Context, tmpDir, filename, stage string, command ...string) ValidationResult {
	start := time.Now()

	// Build container run command
	args := []string{
		"run", "--rm",
		"--network", "none",                        // No network access
		"--read-only",                              // Read-only root filesystem
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m", // Writable /tmp for compilation
		"-v", tmpDir + ":/src:ro",                  // Mount code read-only
		"--timeout", "120",                         // 2 minute timeout
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
			sb.WriteString(fmt.Sprintf("✓ %s (%.2fs)\n", r.Stage, r.Duration.Seconds()))
		} else {
			allPassed = false
			sb.WriteString(fmt.Sprintf("✗ %s (%.2fs)\n", r.Stage, r.Duration.Seconds()))
			if r.Error != "" {
				// Indent error output
				lines := strings.Split(strings.TrimSpace(r.Error), "\n")
				for _, line := range lines {
					sb.WriteString(fmt.Sprintf("  %s\n", line))
				}
			}
		}
	}

	if allPassed {
		sb.WriteString("\n✓ All validation stages passed!\n")
	}

	return sb.String()
}
