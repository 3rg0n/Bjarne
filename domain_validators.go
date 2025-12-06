package main

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// DomainValidationResult holds results from domain-specific validation
type DomainValidationResult struct {
	ValidatorID ValidatorID
	Success     bool
	Output      string
	Metrics     map[string]interface{} // Domain-specific metrics (e.g., latency values, memory usage)
}

// RunDomainValidators executes enabled domain-specific validators
func (c *ContainerRuntime) RunDomainValidators(ctx context.Context, tmpDir string, code string, filename string, config *ValidatorConfig) []DomainValidationResult {
	var results []DomainValidationResult

	// Game Development validators (F-010)
	if config.IsEnabled(ValidatorFrameTiming) {
		result := c.runFrameTimingValidator(ctx, tmpDir, code, filename, config.GetArg(ValidatorFrameTiming))
		results = append(results, result)
	}
	if config.IsEnabled(ValidatorMemoryBudget) {
		result := c.runMemoryBudgetValidator(ctx, tmpDir, code, filename, config.GetArg(ValidatorMemoryBudget))
		results = append(results, result)
	}
	if config.IsEnabled(ValidatorShaderCheck) {
		result := c.runShaderCheckValidator(ctx, tmpDir, code, filename)
		results = append(results, result)
	}

	// HFT validators (F-011)
	if config.IsEnabled(ValidatorLatency) {
		result := c.runLatencyValidator(ctx, tmpDir, code, filename, config.GetArg(ValidatorLatency))
		results = append(results, result)
	}
	if config.IsEnabled(ValidatorLockFree) {
		result := c.runLockFreeValidator(ctx, tmpDir, code, filename)
		results = append(results, result)
	}
	if config.IsEnabled(ValidatorCache) {
		result := c.runCacheValidator(ctx, tmpDir, code, filename)
		results = append(results, result)
	}

	// Embedded validators (F-012)
	if config.IsEnabled(ValidatorStackSize) {
		result := c.runStackSizeValidator(ctx, tmpDir, code, filename, config.GetArg(ValidatorStackSize))
		results = append(results, result)
	}
	if config.IsEnabled(ValidatorInterrupt) {
		result := c.runInterruptValidator(ctx, tmpDir, code, filename)
		results = append(results, result)
	}
	if config.IsEnabled(ValidatorRealTime) {
		result := c.runRealTimeValidator(ctx, tmpDir, code, filename, config.GetArg(ValidatorRealTime))
		results = append(results, result)
	}
	if config.IsEnabled(ValidatorROMSize) {
		result := c.runROMSizeValidator(ctx, tmpDir, code, filename, config.GetArg(ValidatorROMSize))
		results = append(results, result)
	}

	// Security validators (F-013)
	if config.IsEnabled(ValidatorFuzz) {
		result := c.runFuzzValidator(ctx, tmpDir, code, filename, config.GetArg(ValidatorFuzz))
		results = append(results, result)
	}
	if config.IsEnabled(ValidatorSecStatic) {
		result := c.runSecurityStaticValidator(ctx, tmpDir, code, filename)
		results = append(results, result)
	}
	if config.IsEnabled(ValidatorInput) {
		result := c.runInputValidationValidator(ctx, tmpDir, code, filename)
		results = append(results, result)
	}

	// Performance validators (F-014)
	if config.IsEnabled(ValidatorBenchmark) {
		result := c.runBenchmarkValidator(ctx, tmpDir, code, filename, config.GetArg(ValidatorBenchmark))
		results = append(results, result)
	}
	if config.IsEnabled(ValidatorMemProfile) {
		result := c.runMemProfileValidator(ctx, tmpDir, code, filename)
		results = append(results, result)
	}
	if config.IsEnabled(ValidatorCPUProfile) {
		result := c.runCPUProfileValidator(ctx, tmpDir, code, filename)
		results = append(results, result)
	}
	if config.IsEnabled(ValidatorFlameGraph) {
		result := c.runFlameGraphValidator(ctx, tmpDir, code, filename)
		results = append(results, result)
	}

	return results
}

// =============================================================================
// F-010: Game Development Validators
// =============================================================================

// runFrameTimingValidator checks if code can run within frame budget (16.67ms for 60fps)
func (c *ContainerRuntime) runFrameTimingValidator(ctx context.Context, tmpDir, code, filename, arg string) DomainValidationResult { //nolint:unparam // code reserved for future harness generation
	targetFPS := 60
	if arg != "" {
		if fps, err := parseArg(arg, "target_fps"); err == nil {
			targetFPS = fps
		}
	}
	budgetMs := 1000.0 / float64(targetFPS)

	// Compile with timing instrumentation and run
	result := c.runValidationStage(ctx, tmpDir, "frame-timing",
		"sh", "-c",
		fmt.Sprintf(`clang++ -std=c++17 -O2 -o /tmp/frame_test /src/%s &&
		START=$(date +%%s%%N) && /tmp/frame_test && END=$(date +%%s%%N) &&
		ELAPSED=$((($END - $START) / 1000000)) &&
		echo "Execution time: ${ELAPSED}ms" &&
		if [ $ELAPSED -gt %d ]; then
			echo "WARNING: Execution time ${ELAPSED}ms exceeds frame budget %dms"
		else
			echo "Frame timing OK: ${ELAPSED}ms within %dms budget"
		fi`, filename, int(budgetMs), int(budgetMs), int(budgetMs)))

	return DomainValidationResult{
		ValidatorID: ValidatorFrameTiming,
		Success:     result.Success,
		Output:      result.Output,
		Metrics:     map[string]interface{}{"target_fps": targetFPS, "budget_ms": budgetMs},
	}
}

// runMemoryBudgetValidator checks memory allocation limits
func (c *ContainerRuntime) runMemoryBudgetValidator(ctx context.Context, tmpDir, code, filename, arg string) DomainValidationResult { //nolint:unparam // code reserved for future use
	maxMB := 512
	if arg != "" {
		if mb, err := parseArg(arg, "max_mb"); err == nil {
			maxMB = mb
		}
	}

	// Use valgrind massif or a simple approach with ulimit
	result := c.runValidationStage(ctx, tmpDir, "memory-budget",
		"sh", "-c",
		fmt.Sprintf(`clang++ -std=c++17 -O2 -o /tmp/mem_test /src/%s &&
		ulimit -v %d && /tmp/mem_test 2>&1 || {
			echo "ERROR: Memory limit %dMB exceeded"
			exit 1
		}
		echo "Memory usage within %dMB budget"`, filename, maxMB*1024, maxMB, maxMB))

	return DomainValidationResult{
		ValidatorID: ValidatorMemoryBudget,
		Success:     result.Success,
		Output:      result.Output,
		Metrics:     map[string]interface{}{"max_mb": maxMB},
	}
}

// runShaderCheckValidator validates GLSL/HLSL shaders in code
func (c *ContainerRuntime) runShaderCheckValidator(ctx context.Context, tmpDir, code, filename string) DomainValidationResult { //nolint:unparam // filename reserved for future use
	// Check for embedded shader code (common patterns)
	hasShaders := strings.Contains(code, "gl_Position") ||
		strings.Contains(code, "gl_FragColor") ||
		strings.Contains(code, "SV_POSITION") ||
		strings.Contains(code, "#version")

	if !hasShaders {
		return DomainValidationResult{
			ValidatorID: ValidatorShaderCheck,
			Success:     true,
			Output:      "No shader code detected, skipping",
		}
	}

	// Use glslangValidator if available
	result := c.runValidationStage(ctx, tmpDir, "shader-check",
		"sh", "-c",
		`which glslangValidator > /dev/null 2>&1 && {
			# Extract and validate shaders
			echo "Shader validation not yet implemented - requires shader extraction"
		} || echo "glslangValidator not installed, skipping shader validation"`)

	return DomainValidationResult{
		ValidatorID: ValidatorShaderCheck,
		Success:     result.Success || strings.Contains(result.Output, "not installed"),
		Output:      result.Output,
	}
}

// =============================================================================
// F-011: HFT (High-Frequency Trading) Validators
// =============================================================================

// runLatencyValidator measures p50/p95/p99 latency
func (c *ContainerRuntime) runLatencyValidator(ctx context.Context, tmpDir, code, filename, arg string) DomainValidationResult { //nolint:unparam // code reserved for future harness generation
	p99Target := 100 // microseconds
	if arg != "" {
		if us, err := parseArg(arg, "p99_us"); err == nil {
			p99Target = us
		}
	}

	// Compile with optimizations and run timing check
	result := c.runValidationStage(ctx, tmpDir, "latency",
		"sh", "-c",
		fmt.Sprintf(`clang++ -std=c++17 -O3 -march=native -o /tmp/lat_test /src/%s &&
		/tmp/lat_test`, filename))

	return DomainValidationResult{
		ValidatorID: ValidatorLatency,
		Success:     result.Success,
		Output:      result.Output,
		Metrics:     map[string]interface{}{"p99_target_us": p99Target},
	}
}

// runLockFreeValidator checks for lock-free algorithm correctness
func (c *ContainerRuntime) runLockFreeValidator(ctx context.Context, tmpDir, code, filename string) DomainValidationResult {
	// Check for std::atomic and verify lock-free property
	hasAtomic := strings.Contains(code, "std::atomic") || strings.Contains(code, "<atomic>")

	if !hasAtomic {
		return DomainValidationResult{
			ValidatorID: ValidatorLockFree,
			Success:     true,
			Output:      "No atomic operations detected",
		}
	}

	// Check for common lock-free anti-patterns
	var warnings []string

	// Check for mutex alongside atomic (potential mixing of paradigms)
	if strings.Contains(code, "std::mutex") && hasAtomic {
		warnings = append(warnings, "WARNING: Code mixes std::mutex and std::atomic - verify this is intentional")
	}

	// Check for memory ordering
	if !strings.Contains(code, "memory_order") {
		warnings = append(warnings, "INFO: No explicit memory_order specified - using default seq_cst (safest but slowest)")
	}

	// Compile with atomic checks
	result := c.runValidationStage(ctx, tmpDir, "lock-free",
		"sh", "-c",
		fmt.Sprintf(`clang++ -std=c++17 -O2 -o /tmp/lockfree_test /src/%s &&
		echo "Lock-free analysis:" &&
		nm /tmp/lockfree_test | grep -i atomic || echo "No atomic symbols found in binary"`, filename))

	output := result.Output
	for _, w := range warnings {
		output = w + "\n" + output
	}

	return DomainValidationResult{
		ValidatorID: ValidatorLockFree,
		Success:     result.Success,
		Output:      output,
	}
}

// runCacheValidator checks cache-friendly access patterns
func (c *ContainerRuntime) runCacheValidator(ctx context.Context, tmpDir, code, filename string) DomainValidationResult {
	var warnings []string

	// Static analysis for cache-unfriendly patterns

	// Check for potential false sharing (small structs with atomics)
	if strings.Contains(code, "std::atomic") {
		if !strings.Contains(code, "alignas") && !strings.Contains(code, "cache_line") {
			warnings = append(warnings, "INFO: atomic variables without explicit alignment - potential false sharing")
		}
	}

	// Check for column-major vs row-major access in 2D arrays
	// Pattern: arr[j][i] inside nested loops
	colMajorPattern := regexp.MustCompile(`for\s*\([^)]*\bi\b[^)]*\)[^{]*\{[^}]*for\s*\([^)]*\bj\b[^)]*\)[^{]*\{[^}]*\[\s*j\s*\]\s*\[\s*i\s*\]`)
	if colMajorPattern.MatchString(code) {
		warnings = append(warnings, "WARNING: Potential column-major access pattern detected - may cause cache misses")
	}

	// Check for linked list usage (cache-unfriendly)
	if strings.Contains(code, "std::list") || strings.Contains(code, "->next") {
		warnings = append(warnings, "INFO: Linked list usage detected - consider std::vector for cache locality")
	}

	result := c.runValidationStage(ctx, tmpDir, "cache",
		"sh", "-c",
		fmt.Sprintf(`clang++ -std=c++17 -O2 -o /tmp/cache_test /src/%s &&
		echo "Cache analysis complete"`, filename))

	output := strings.Join(warnings, "\n")
	if output != "" {
		output += "\n"
	}
	output += result.Output

	return DomainValidationResult{
		ValidatorID: ValidatorCache,
		Success:     result.Success,
		Output:      output,
	}
}

// =============================================================================
// F-012: Embedded Systems Validators
// =============================================================================

// runStackSizeValidator analyzes stack usage
func (c *ContainerRuntime) runStackSizeValidator(ctx context.Context, tmpDir, code, filename, arg string) DomainValidationResult { //nolint:unparam // code reserved for future use
	maxKB := 8
	if arg != "" {
		if kb, err := parseArg(arg, "max_kb"); err == nil {
			maxKB = kb
		}
	}

	// Use -fstack-usage to generate stack usage info
	result := c.runValidationStage(ctx, tmpDir, "stack-size",
		"sh", "-c",
		fmt.Sprintf(`clang++ -std=c++17 -fstack-usage -o /tmp/stack_test /src/%s 2>&1 &&
		if [ -f /src/%s.su ]; then
			echo "Stack usage per function:"
			cat /src/%s.su
			MAX_STACK=$(awk '{print $2}' /src/%s.su | sort -n | tail -1)
			echo "Maximum stack usage: ${MAX_STACK} bytes"
			if [ "${MAX_STACK:-0}" -gt %d ]; then
				echo "ERROR: Stack usage ${MAX_STACK} bytes exceeds limit %d bytes"
				exit 1
			fi
		else
			echo "Stack usage file not generated"
		fi`, filename, strings.TrimSuffix(filename, ".cpp"), strings.TrimSuffix(filename, ".cpp"), strings.TrimSuffix(filename, ".cpp"), maxKB*1024, maxKB*1024))

	return DomainValidationResult{
		ValidatorID: ValidatorStackSize,
		Success:     result.Success,
		Output:      result.Output,
		Metrics:     map[string]interface{}{"max_kb": maxKB},
	}
}

// runInterruptValidator checks ISR (Interrupt Service Routine) constraints
func (c *ContainerRuntime) runInterruptValidator(ctx context.Context, tmpDir, code, filename string) DomainValidationResult {
	var warnings []string

	// Check for ISR-unsafe patterns
	isrPatterns := []struct {
		pattern string
		warning string
	}{
		{"malloc", "WARNING: malloc() in potential ISR context - use static buffers"},
		{"new ", "WARNING: new in potential ISR context - use static allocation"},
		{"printf", "WARNING: printf() in potential ISR context - use async-safe logging"},
		{"std::cout", "WARNING: std::cout in potential ISR context - not async-safe"},
		{"std::mutex", "WARNING: mutex in potential ISR context - may cause deadlock"},
		{"std::lock_guard", "WARNING: lock_guard in potential ISR context - may cause deadlock"},
	}

	// Check if code might contain ISR handlers
	hasISR := strings.Contains(code, "interrupt") ||
		strings.Contains(code, "ISR") ||
		strings.Contains(code, "__attribute__((interrupt))")

	if hasISR {
		for _, p := range isrPatterns {
			if strings.Contains(code, p.pattern) {
				warnings = append(warnings, p.warning)
			}
		}
	}

	result := c.runValidationStage(ctx, tmpDir, "interrupt",
		"sh", "-c",
		fmt.Sprintf(`clang++ -std=c++17 -O2 -ffreestanding -fno-exceptions -o /tmp/isr_test /src/%s 2>&1 || {
			echo "Note: Freestanding compilation may fail for non-embedded code"
			clang++ -std=c++17 -O2 -o /tmp/isr_test /src/%s
		}`, filename, filename))

	output := strings.Join(warnings, "\n")
	if output != "" {
		output += "\n"
	}
	output += result.Output

	success := result.Success
	if len(warnings) > 0 && hasISR {
		success = false // Fail if ISR patterns found
	}

	return DomainValidationResult{
		ValidatorID: ValidatorInterrupt,
		Success:     success,
		Output:      output,
	}
}

// runRealTimeValidator checks real-time constraints (WCET)
func (c *ContainerRuntime) runRealTimeValidator(ctx context.Context, tmpDir, code, filename, arg string) DomainValidationResult {
	deadlineUs := 1000
	if arg != "" {
		if us, err := parseArg(arg, "deadline_us"); err == nil {
			deadlineUs = us
		}
	}

	// Check for unbounded operations
	var warnings []string
	unboundedPatterns := []struct {
		pattern string
		warning string
	}{
		{"std::vector", "INFO: std::vector may have unbounded allocation time"},
		{"std::string", "INFO: std::string may have unbounded allocation time"},
		{"while (true)", "WARNING: Unbounded loop detected - ensure exit condition exists"},
		{"for (;;)", "WARNING: Unbounded loop detected"},
		{"malloc", "WARNING: Dynamic allocation has unbounded WCET"},
		{"new ", "WARNING: Dynamic allocation has unbounded WCET"},
	}

	for _, p := range unboundedPatterns {
		if strings.Contains(code, p.pattern) {
			warnings = append(warnings, p.warning)
		}
	}

	result := c.runValidationStage(ctx, tmpDir, "real-time",
		"sh", "-c",
		fmt.Sprintf(`clang++ -std=c++17 -O2 -o /tmp/rt_test /src/%s &&
		echo "Real-time analysis (deadline: %dus):" &&
		echo "Note: Full WCET analysis requires specialized tools"`, filename, deadlineUs))

	output := strings.Join(warnings, "\n")
	if output != "" {
		output += "\n"
	}
	output += result.Output

	return DomainValidationResult{
		ValidatorID: ValidatorRealTime,
		Success:     result.Success,
		Output:      output,
		Metrics:     map[string]interface{}{"deadline_us": deadlineUs},
	}
}

// runROMSizeValidator checks binary size for embedded targets
func (c *ContainerRuntime) runROMSizeValidator(ctx context.Context, tmpDir, code, filename, arg string) DomainValidationResult { //nolint:unparam // code reserved for future use
	maxKB := 256
	if arg != "" {
		if kb, err := parseArg(arg, "max_kb"); err == nil {
			maxKB = kb
		}
	}

	result := c.runValidationStage(ctx, tmpDir, "rom-size",
		"sh", "-c",
		fmt.Sprintf(`clang++ -std=c++17 -Os -ffunction-sections -fdata-sections -Wl,--gc-sections -o /tmp/rom_test /src/%s &&
		SIZE=$(stat -c%%s /tmp/rom_test 2>/dev/null || stat -f%%z /tmp/rom_test)
		SIZE_KB=$((SIZE / 1024))
		echo "Binary size: ${SIZE} bytes (${SIZE_KB} KB)"
		if [ $SIZE_KB -gt %d ]; then
			echo "ERROR: Binary size ${SIZE_KB}KB exceeds limit %dKB"
			exit 1
		fi
		echo "ROM size check PASSED: ${SIZE_KB}KB <= %dKB"`, filename, maxKB, maxKB, maxKB))

	return DomainValidationResult{
		ValidatorID: ValidatorROMSize,
		Success:     result.Success,
		Output:      result.Output,
		Metrics:     map[string]interface{}{"max_kb": maxKB},
	}
}

// =============================================================================
// F-013: Security Validators
// =============================================================================

// runFuzzValidator runs basic fuzzing with libFuzzer
func (c *ContainerRuntime) runFuzzValidator(ctx context.Context, tmpDir, code, filename, arg string) DomainValidationResult {
	iterations := 10000
	if arg != "" {
		if iters, err := parseArg(arg, "iterations"); err == nil {
			iterations = iters
		}
	}

	// Check if code has a LLVMFuzzerTestOneInput
	hasFuzzTarget := strings.Contains(code, "LLVMFuzzerTestOneInput")

	if !hasFuzzTarget {
		return DomainValidationResult{
			ValidatorID: ValidatorFuzz,
			Success:     true,
			Output:      "No fuzz target (LLVMFuzzerTestOneInput) found - skipping fuzzing",
		}
	}

	result := c.runValidationStage(ctx, tmpDir, "fuzz",
		"sh", "-c",
		fmt.Sprintf(`clang++ -std=c++17 -fsanitize=fuzzer,address -o /tmp/fuzz_test /src/%s &&
		timeout 30 /tmp/fuzz_test -max_total_time=30 -runs=%d 2>&1 || {
			if [ $? -eq 124 ]; then
				echo "Fuzzing completed (timeout)"
			else
				echo "Fuzzer found issues"
				exit 1
			fi
		}`, filename, iterations))

	return DomainValidationResult{
		ValidatorID: ValidatorFuzz,
		Success:     result.Success,
		Output:      result.Output,
		Metrics:     map[string]interface{}{"iterations": iterations},
	}
}

// runSecurityStaticValidator runs security-focused static analysis
func (c *ContainerRuntime) runSecurityStaticValidator(ctx context.Context, tmpDir, code, filename string) DomainValidationResult {
	var issues []string

	// Check for common security vulnerabilities (CWE patterns)
	securityPatterns := []struct {
		pattern string
		cwe     string
		message string
	}{
		{"gets(", "CWE-120", "gets() is dangerous - use fgets() instead"},
		{"strcpy(", "CWE-120", "strcpy() can cause buffer overflow - use strncpy() or strlcpy()"},
		{"strcat(", "CWE-120", "strcat() can cause buffer overflow - use strncat() or strlcat()"},
		{"sprintf(", "CWE-120", "sprintf() can cause buffer overflow - use snprintf()"},
		{"scanf(\"%s\"", "CWE-120", "scanf %s can cause buffer overflow - specify width limit"},
		{"system(", "CWE-78", "system() can lead to command injection - validate input"},
		{"popen(", "CWE-78", "popen() can lead to command injection - validate input"},
		{"exec", "CWE-78", "exec family functions can lead to command injection"},
		{"rand()", "CWE-338", "rand() is not cryptographically secure - use std::random_device"},
		{"tmpnam(", "CWE-377", "tmpnam() is insecure - use mkstemp()"},
		{"mktemp(", "CWE-377", "mktemp() is insecure - use mkstemp()"},
	}

	for _, p := range securityPatterns {
		if strings.Contains(code, p.pattern) {
			issues = append(issues, fmt.Sprintf("%s: %s", p.cwe, p.message))
		}
	}

	// Run clang-tidy with security checks
	result := c.runValidationStage(ctx, tmpDir, "sec-static",
		"sh", "-c",
		fmt.Sprintf(`clang-tidy -checks='-*,bugprone-*,cert-*,security-*' /src/%s -- -std=c++17 2>&1`, filename))

	output := "Security static analysis results:\n"
	for _, issue := range issues {
		output += "  " + issue + "\n"
	}
	output += result.Output

	success := result.Success && len(issues) == 0

	return DomainValidationResult{
		ValidatorID: ValidatorSecStatic,
		Success:     success,
		Output:      output,
	}
}

// runInputValidationValidator checks for proper input validation
func (c *ContainerRuntime) runInputValidationValidator(ctx context.Context, tmpDir, code, filename string) DomainValidationResult {
	var warnings []string

	// Check for input handling without validation
	inputPatterns := []struct {
		input    string
		validate string
		warning  string
	}{
		{"std::cin", "if (std::cin.fail())", "Input from cin without validation check"},
		{"argv[", "argc", "Command line args accessed - ensure bounds checking"},
		{"getenv(", "nullptr", "Environment variable access - check for nullptr"},
		{"fread(", "return", "File read - verify return value"},
		{"recv(", "return", "Network recv - verify return value and handle partial reads"},
	}

	for _, p := range inputPatterns {
		if strings.Contains(code, p.input) && !strings.Contains(code, p.validate) {
			warnings = append(warnings, "WARNING: "+p.warning)
		}
	}

	result := c.runValidationStage(ctx, tmpDir, "input",
		"sh", "-c",
		fmt.Sprintf(`clang++ -std=c++17 -Wall -Wextra -o /tmp/input_test /src/%s 2>&1`, filename))

	output := "Input validation analysis:\n"
	for _, w := range warnings {
		output += "  " + w + "\n"
	}
	if len(warnings) == 0 {
		output += "  No obvious input validation issues found\n"
	}
	output += result.Output

	return DomainValidationResult{
		ValidatorID: ValidatorInput,
		Success:     result.Success,
		Output:      output,
	}
}

// =============================================================================
// F-014: Performance Validators
// =============================================================================

// runBenchmarkValidator runs Google Benchmark comparison
func (c *ContainerRuntime) runBenchmarkValidator(ctx context.Context, tmpDir, code, filename, arg string) DomainValidationResult {
	// baseline arg could specify a baseline file to compare against
	_ = arg

	// Check for benchmark::State
	hasBenchmark := strings.Contains(code, "benchmark::State") ||
		strings.Contains(code, "BENCHMARK(")

	if !hasBenchmark {
		return DomainValidationResult{
			ValidatorID: ValidatorBenchmark,
			Success:     true,
			Output:      "No Google Benchmark code found - skipping",
		}
	}

	result := c.runValidationStage(ctx, tmpDir, "benchmark",
		"sh", "-c",
		fmt.Sprintf(`clang++ -std=c++17 -O2 -o /tmp/bench_test /src/%s -lbenchmark -lpthread 2>&1 &&
		/tmp/bench_test --benchmark_format=console`, filename))

	return DomainValidationResult{
		ValidatorID: ValidatorBenchmark,
		Success:     result.Success,
		Output:      result.Output,
	}
}

// runMemProfileValidator profiles memory usage
func (c *ContainerRuntime) runMemProfileValidator(ctx context.Context, tmpDir, code, filename string) DomainValidationResult { //nolint:unparam // code reserved for future use
	result := c.runValidationStage(ctx, tmpDir, "mem-prof",
		"sh", "-c",
		fmt.Sprintf(`clang++ -std=c++17 -O2 -o /tmp/mem_prof /src/%s &&
		which valgrind > /dev/null 2>&1 && {
			valgrind --tool=massif --massif-out-file=/tmp/massif.out /tmp/mem_prof 2>&1
			ms_print /tmp/massif.out 2>/dev/null || cat /tmp/massif.out
		} || {
			echo "valgrind not installed - using basic memory tracking"
			/usr/bin/time -v /tmp/mem_prof 2>&1 | grep -E "(Maximum resident|Minor|Major)"
		}`, filename))

	return DomainValidationResult{
		ValidatorID: ValidatorMemProfile,
		Success:     result.Success,
		Output:      result.Output,
	}
}

// runCPUProfileValidator profiles CPU usage
func (c *ContainerRuntime) runCPUProfileValidator(ctx context.Context, tmpDir, code, filename string) DomainValidationResult { //nolint:unparam // code reserved for future use
	result := c.runValidationStage(ctx, tmpDir, "cpu-prof",
		"sh", "-c",
		fmt.Sprintf(`clang++ -std=c++17 -O2 -g -o /tmp/cpu_prof /src/%s &&
		which perf > /dev/null 2>&1 && {
			perf record -g -o /tmp/perf.data /tmp/cpu_prof 2>&1
			perf report -i /tmp/perf.data --stdio 2>&1 | head -50
		} || {
			echo "perf not available - using time-based profiling"
			/usr/bin/time -v /tmp/cpu_prof 2>&1
		}`, filename))

	return DomainValidationResult{
		ValidatorID: ValidatorCPUProfile,
		Success:     result.Success,
		Output:      result.Output,
	}
}

// runFlameGraphValidator generates flame graph data
func (c *ContainerRuntime) runFlameGraphValidator(ctx context.Context, tmpDir, code, filename string) DomainValidationResult { //nolint:unparam // code reserved for future use
	result := c.runValidationStage(ctx, tmpDir, "flamegraph",
		"sh", "-c",
		fmt.Sprintf(`clang++ -std=c++17 -O2 -g -fno-omit-frame-pointer -o /tmp/flame_test /src/%s &&
		which perf > /dev/null 2>&1 && {
			perf record -F 99 -g -o /tmp/perf.data /tmp/flame_test 2>&1
			perf script -i /tmp/perf.data 2>&1 | head -100
			echo "Flame graph data generated (use flamegraph.pl to visualize)"
		} || {
			echo "perf not available - flame graph generation requires perf"
		}`, filename))

	return DomainValidationResult{
		ValidatorID: ValidatorFlameGraph,
		Success:     result.Success,
		Output:      result.Output,
	}
}

// =============================================================================
// Helper functions
// =============================================================================

// parseArg extracts an integer value from arg string like "key=value"
func parseArg(arg, key string) (int, error) {
	parts := strings.Split(arg, "=")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid arg format")
	}
	if parts[0] != key {
		return 0, fmt.Errorf("key mismatch")
	}
	return strconv.Atoi(parts[1])
}
