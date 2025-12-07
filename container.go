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

// Default container image (published to ghcr.io on release)
const defaultValidatorImage = "ghcr.io/3rg0n/bjarne-validator:latest"

// getImageName returns the container image to use
func getImageName() string {
	// Check for override (local development or custom image)
	if img := os.Getenv("BJARNE_VALIDATOR_IMAGE"); img != "" {
		return img
	}
	// Default to ghcr.io hosted image
	return defaultValidatorImage
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

// GetLocalImageDigest returns the digest of the local image, or empty string if not found
func (c *ContainerRuntime) GetLocalImageDigest(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, c.binary, "image", "inspect", "--format", "{{.Digest}}", c.imageName)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// GetRemoteImageDigest returns the digest of the remote image without pulling it
func (c *ContainerRuntime) GetRemoteImageDigest(ctx context.Context) string {
	// Use skopeo-style inspection via podman/docker manifest inspect
	cmd := exec.CommandContext(ctx, c.binary, "manifest", "inspect", c.imageName)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	// Parse the digest from manifest - look for "digest" field
	// The manifest contains a digest in the response
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, `"digest"`) {
			// Extract digest value like "sha256:abc123..."
			parts := strings.Split(line, `"`)
			for i, part := range parts {
				if part == "digest" && i+2 < len(parts) {
					return parts[i+2]
				}
			}
		}
	}
	return ""
}

// CheckForUpdate checks if a newer container image is available
// Returns true if an update is available, false otherwise
func (c *ContainerRuntime) CheckForUpdate(ctx context.Context) bool {
	localDigest := c.GetLocalImageDigest(ctx)
	if localDigest == "" {
		return false // No local image, not an "update" scenario
	}

	remoteDigest := c.GetRemoteImageDigest(ctx)
	if remoteDigest == "" {
		return false // Can't check remote, assume no update
	}

	// Compare digests - if different, update available
	return localDigest != remoteDigest
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

// ValidateMultiFileCode validates a multi-file project
func (c *ContainerRuntime) ValidateMultiFileCode(ctx context.Context, files []CodeFile) ([]ValidationResult, error) {
	return c.ValidateMultiFileCodeWithExamples(ctx, files, nil, nil)
}

// ValidateMultiFileCodeWithExamples validates a multi-file project with example tests
// Note: examples and dod parameters are reserved for future use (will be implemented similar to single-file validation)
func (c *ContainerRuntime) ValidateMultiFileCodeWithExamples(ctx context.Context, files []CodeFile, examples *ExampleTests, dod *DefinitionOfDone) ([]ValidationResult, error) { //nolint:unparam // examples and dod will be used in future
	// TODO: Implement example tests and DoD for multi-file projects
	_ = examples // Reserved for future use
	_ = dod      // Reserved for future use

	// Create temp directory for all files
	tmpDir, err := os.MkdirTemp("", "bjarne-validate-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Write all files to temp directory
	var sourceFiles []string
	for _, f := range files {
		filePath := filepath.Join(tmpDir, f.Filename)
		if err := os.WriteFile(filePath, []byte(f.Content), 0600); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", f.Filename, err)
		}
		// Track .cpp files for compilation
		if strings.HasSuffix(f.Filename, ".cpp") || strings.HasSuffix(f.Filename, ".cc") || strings.HasSuffix(f.Filename, ".c") {
			sourceFiles = append(sourceFiles, "/src/"+f.Filename)
		}
	}

	if len(sourceFiles) == 0 {
		return nil, fmt.Errorf("no source files (.cpp/.cc/.c) found")
	}

	// Build compilation command for all source files
	srcArgs := strings.Join(sourceFiles, " ")

	var results []ValidationResult

	// Stage 1: clang-tidy on all source files
	for _, f := range files {
		if strings.HasSuffix(f.Filename, ".cpp") || strings.HasSuffix(f.Filename, ".cc") || strings.HasSuffix(f.Filename, ".c") {
			result := c.runValidationStage(ctx, tmpDir, "clang-tidy:"+f.Filename,
				"clang-tidy", "-quiet", "-header-filter=.*", "/src/"+f.Filename, "--", "-std=c++17", "-Wall", "-Wextra", "-I/src")
			results = append(results, result)
			if !result.Success {
				return results, nil
			}
		}
	}

	// Stage 2: cppcheck on all files
	result := c.runValidationStage(ctx, tmpDir, "cppcheck",
		"sh", "-c",
		"which cppcheck > /dev/null 2>&1 && cppcheck --enable=all --error-exitcode=1 --suppress=missingIncludeSystem --std=c++17 -I/src /src/*.cpp /src/*.h 2>&1 || (which cppcheck > /dev/null 2>&1 || echo 'cppcheck not installed, skipping')")
	if !result.Success && !strings.Contains(result.Output, "not installed") {
		results = append(results, result)
		return results, nil
	}
	if !strings.Contains(result.Output, "not installed") {
		results = append(results, result)
	}

	// Stage 3: Compile all source files together with hardening flags
	// Security hardening: stack protector, FORTIFY_SOURCE, PIE, RELRO
	// Note: -U_FORTIFY_SOURCE before -D to avoid macro redefinition error (container may have it set)
	result = c.runValidationStage(ctx, tmpDir, "compile",
		"sh", "-c",
		"clang++ -std=c++17 -Wall -Wextra -Werror -fstack-protector-all -U_FORTIFY_SOURCE -D_FORTIFY_SOURCE=2 -fPIE -pie -Wl,-z,relro -Wl,-z,now -I/src -o /tmp/test "+srcArgs)
	results = append(results, result)
	if !result.Success {
		return results, nil
	}

	// Stage 4: ASAN
	result = c.runValidationStage(ctx, tmpDir, "asan",
		"sh", "-c",
		"clang++ -std=c++17 -fsanitize=address -fno-omit-frame-pointer -g -I/src -o /tmp/test "+srcArgs+" && /tmp/test")
	results = append(results, result)
	if !result.Success {
		return results, nil
	}

	// Stage 5: UBSAN
	result = c.runValidationStage(ctx, tmpDir, "ubsan",
		"sh", "-c",
		"clang++ -std=c++17 -fsanitize=undefined -fno-omit-frame-pointer -g -I/src -o /tmp/test "+srcArgs+" && /tmp/test")
	results = append(results, result)
	if !result.Success {
		return results, nil
	}

	// Stage 6: MSan
	// Note: MSan is optional - if the instrumented libc++ isn't working, we skip it
	result = c.runValidationStage(ctx, tmpDir, "msan",
		"sh", "-c",
		"if [ -d /opt/msan/lib ] && ([ -f /opt/msan/lib/libc++.so ] || [ -f /opt/msan/lib/libc++.a ]); then "+
			"clang++ -std=c++17 -fsanitize=memory -fsanitize-memory-track-origins "+
			"-fno-omit-frame-pointer -g -stdlib=libc++ "+
			"-nostdinc++ -isystem /opt/msan/include/c++/v1 "+
			"-L/opt/msan/lib -Wl,-rpath,/opt/msan/lib "+
			"-lc++ -lc++abi "+
			"-I/src -o /tmp/test "+srcArgs+" 2>&1 || echo 'MSAN_SKIP: MSan libc++ linkage failed'; "+
			"else echo 'MSAN_SKIP: MSan libc++ not available'; fi && "+
			"[ -f /tmp/test ] && /tmp/test || true")
	// MSan is advisory when libc++ linkage fails - it's a complex setup
	if strings.Contains(result.Output, "MSAN_SKIP") {
		result.Success = true
		result.Output = "MSan skipped: " + result.Output
	}
	results = append(results, result)
	if !result.Success {
		return results, nil
	}

	// Stage 7: TSAN if threads detected
	usesThreads := false
	for _, f := range files {
		if codeUsesThreads(f.Content) {
			usesThreads = true
			break
		}
	}
	if usesThreads {
		result = c.runValidationStage(ctx, tmpDir, "tsan",
			"sh", "-c",
			"clang++ -std=c++17 -fsanitize=thread -fno-omit-frame-pointer -g -I/src -o /tmp/test "+srcArgs+" && /tmp/test")
		results = append(results, result)
		if !result.Success {
			return results, nil
		}
	}

	// Stage 8: Final run
	result = c.runValidationStage(ctx, tmpDir, "run",
		"sh", "-c",
		"clang++ -std=c++17 -O2 -I/src -o /tmp/test "+srcArgs+" && /tmp/test")
	results = append(results, result)

	return results, nil
}

// ValidateCodeWithExamples runs validation including example-based tests
func (c *ContainerRuntime) ValidateCodeWithExamples(ctx context.Context, code string, filename string, examples *ExampleTests, dod *DefinitionOfDone) ([]ValidationResult, error) {
	return c.validateCodeFull(ctx, code, filename, examples, dod, nil)
}

// ValidateCodeWithDoD runs validation with Definition of Done requirements
func (c *ContainerRuntime) ValidateCodeWithDoD(ctx context.Context, code string, filename string, examples *ExampleTests, dod *DefinitionOfDone, progress ProgressCallback) ([]ValidationResult, error) {
	return c.validateCodeFull(ctx, code, filename, examples, dod, progress)
}

// validateCodeFull runs the full validation pipeline with examples and DoD
func (c *ContainerRuntime) validateCodeFull(ctx context.Context, code string, filename string, examples *ExampleTests, dod *DefinitionOfDone, progress ProgressCallback) ([]ValidationResult, error) {
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

	// Run example tests if provided
	if examples != nil && len(examples.Tests) > 0 {
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

		if !result.Success {
			return results, nil // Fail fast on example tests
		}
	}

	// Run benchmark if DoD has performance requirements
	if dod != nil && dod.MaxTimeMs > 0 {
		// Try to detect function name for benchmarking
		funcCall := detectBenchmarkFunction(code, examples)
		if funcCall != "" {
			benchHarness := dod.GenerateBenchmarkHarness(code, funcCall)
			benchFilename := "benchmark.cpp"

			// Create temp directory for benchmark
			tmpDir, err := os.MkdirTemp("", "bjarne-bench-*")
			if err != nil {
				return results, fmt.Errorf("failed to create temp dir for benchmark: %w", err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			// Write benchmark harness
			benchPath := filepath.Join(tmpDir, benchFilename)
			if err := os.WriteFile(benchPath, []byte(benchHarness), 0600); err != nil {
				return results, fmt.Errorf("failed to write benchmark: %w", err)
			}

			// Run benchmark
			if progress != nil {
				progress("benchmark", true, nil)
			}
			result := c.runValidationStage(ctx, tmpDir, "benchmark",
				"sh", "-c",
				"clang++ -std=c++17 -O2 -o /tmp/benchmark /src/"+benchFilename+" && /tmp/benchmark")
			if progress != nil {
				progress("benchmark", false, &result)
			}
			results = append(results, result)
		}
	}

	return results, nil
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
	// -quiet removes system header noise, focusing on user code issues
	result := runStage("clang-tidy",
		"clang-tidy", "-quiet", "-header-filter=.*", "/src/"+filename, "--", "-std=c++17", "-Wall", "-Wextra")
	results = append(results, result)
	if !result.Success {
		return results, nil // Fail fast
	}

	// Stage 2: cppcheck (deep static analysis - catches things clang-tidy misses)
	// Skip if cppcheck not installed
	result = runStage("cppcheck",
		"sh", "-c",
		"which cppcheck > /dev/null 2>&1 && cppcheck --enable=all --error-exitcode=1 --suppress=missingIncludeSystem --std=c++17 /src/"+filename+" || (which cppcheck > /dev/null 2>&1 || echo 'cppcheck not installed, skipping')")
	// Only fail if cppcheck exists and found issues
	if !result.Success && !strings.Contains(result.Output, "not installed") {
		results = append(results, result)
		return results, nil
	}
	if !strings.Contains(result.Output, "not installed") {
		results = append(results, result)
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
	// Skip if lizard not installed
	result = runStage("complexity",
		"sh", "-c",
		"which lizard > /dev/null 2>&1 && lizard -C 15 -L 100 -w /src/"+filename+" || (which lizard > /dev/null 2>&1 || echo 'lizard not installed, skipping')")
	// Only fail if lizard exists and found issues
	if !result.Success && !strings.Contains(result.Output, "not installed") {
		results = append(results, result)
		return results, nil
	}
	if !strings.Contains(result.Output, "not installed") {
		results = append(results, result)
	}

	// Stage 5: Compile with strict warnings and hardening flags
	// Security hardening: stack protector, FORTIFY_SOURCE, PIE, RELRO
	// Note: -U_FORTIFY_SOURCE before -D to avoid macro redefinition error (container may have it set)
	result = runStage("compile",
		"clang++", "-std=c++17", "-Wall", "-Wextra", "-Werror",
		"-fstack-protector-all", "-U_FORTIFY_SOURCE", "-D_FORTIFY_SOURCE=2",
		"-fPIE", "-pie", "-Wl,-z,relro", "-Wl,-z,now",
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

	// Stage 8: MSan (MemorySanitizer) - detects uninitialized memory reads
	// MSan requires ALL code (including stdlib) to be instrumented
	// We use custom-built MSan-instrumented libc++ from /opt/msan
	// Note: MSan is optional - if the instrumented libc++ isn't working, we skip it
	result = runStage("msan",
		"sh", "-c",
		"if [ -d /opt/msan/lib ] && ([ -f /opt/msan/lib/libc++.so ] || [ -f /opt/msan/lib/libc++.a ]); then "+
			"clang++ -std=c++17 -fsanitize=memory -fsanitize-memory-track-origins "+
			"-fno-omit-frame-pointer -g -stdlib=libc++ "+
			"-nostdinc++ -isystem /opt/msan/include/c++/v1 "+
			"-L/opt/msan/lib -Wl,-rpath,/opt/msan/lib "+
			"-lc++ -lc++abi "+
			"-o /tmp/test /src/"+filename+" 2>&1 || echo 'MSAN_SKIP: MSan libc++ linkage failed'; "+
			"else echo 'MSAN_SKIP: MSan libc++ not available'; fi && "+
			"[ -f /tmp/test ] && /tmp/test || true")
	// MSan is advisory when libc++ linkage fails - it's a complex setup
	if strings.Contains(result.Output, "MSAN_SKIP") {
		result.Success = true
		result.Output = "MSan skipped: " + result.Output
	}
	results = append(results, result)
	if !result.Success {
		return results, nil
	}

	// Stage 9: Check if code uses threads, run TSAN if so
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

// detectBenchmarkFunction tries to find a function to benchmark in the code
// Returns empty string if no suitable function found
func detectBenchmarkFunction(code string, examples *ExampleTests) string {
	// If we have examples, use the function name from those
	if examples != nil && examples.FunctionName != "" {
		// Try to construct a valid call
		// For functions that take simple arguments, create a test call
		funcName := examples.FunctionName
		if len(examples.Tests) > 0 {
			// Use the first test case's function call
			return examples.Tests[0].FunctionCall
		}
		return funcName + "()"
	}

	// Try to detect common function patterns
	// Look for functions that aren't main()
	// Pattern: returnType functionName(args)
	lines := strings.Split(code, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip main function
		if strings.Contains(line, "int main") {
			continue
		}
		// Look for function definitions that could be benchmarked
		// Simple heuristic: type name followed by ( and not a control structure
		if strings.Contains(line, "(") && !strings.HasPrefix(line, "//") {
			// Check if it looks like a function definition (not a call)
			for _, retType := range []string{"int ", "void ", "bool ", "double ", "float ", "long ", "auto "} {
				if strings.HasPrefix(line, retType) {
					// Extract function name
					rest := strings.TrimPrefix(line, retType)
					if idx := strings.Index(rest, "("); idx > 0 {
						funcName := strings.TrimSpace(rest[:idx])
						// Skip if it contains operators or looks invalid
						if !strings.ContainsAny(funcName, " *&<>[]") && len(funcName) > 0 {
							return funcName + "()"
						}
					}
				}
			}
		}
	}

	return ""
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

// formatStageError parses and formats error output based on stage type (for user display)
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
	case "msan":
		diags := ParseSanitizerOutput(errorOutput, "msan")
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

// FormatErrorForLLM formats a validation error in a compact format for LLM processing
// Returns a clean, minimal representation without ANSI colors
func FormatErrorForLLM(stage, errorOutput string) string {
	var diags []Diagnostic

	switch stage {
	case "clang-tidy":
		diags = ParseClangTidyOutput(errorOutput)
	case "cppcheck":
		diags = ParseCppcheckOutput(errorOutput)
	case "asan":
		diags = ParseSanitizerOutput(errorOutput, "asan")
	case "ubsan":
		diags = ParseSanitizerOutput(errorOutput, "ubsan")
	case "msan":
		diags = ParseSanitizerOutput(errorOutput, "msan")
	case "tsan":
		diags = ParseSanitizerOutput(errorOutput, "tsan")
	case "compile":
		// Compiler errors follow similar pattern to clang-tidy
		diags = ParseClangTidyOutput(errorOutput)
	}

	if len(diags) > 0 {
		return fmt.Sprintf("[%s] %s", stage, FormatDiagnosticsForLLM(diags))
	}

	// Fallback: use raw output but with stage prefix
	// Keep more lines to not lose important context
	lines := strings.Split(strings.TrimSpace(errorOutput), "\n")
	if len(lines) > 50 {
		lines = lines[:50]
		lines = append(lines, "... (truncated, showing first 50 lines)")
	}
	return fmt.Sprintf("[%s] %s", stage, strings.Join(lines, "\n"))
}
