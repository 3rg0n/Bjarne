package main

// ValidatorID identifies a validation gate
type ValidatorID string

// Core validators (always available)
const (
	ValidatorClangTidy  ValidatorID = "clang-tidy"
	ValidatorCppcheck   ValidatorID = "cppcheck"
	ValidatorIWYU       ValidatorID = "iwyu"
	ValidatorComplexity ValidatorID = "complexity"
	ValidatorCompile    ValidatorID = "compile"
	ValidatorASAN       ValidatorID = "asan"
	ValidatorUBSAN      ValidatorID = "ubsan"
	ValidatorMSAN       ValidatorID = "msan"
	ValidatorTSAN       ValidatorID = "tsan"
	ValidatorRun        ValidatorID = "run"
	ValidatorReview     ValidatorID = "review"
)

// Domain-specific validators (F-010 to F-014)
const (
	// F-010: Game Development
	ValidatorFrameTiming  ValidatorID = "frame-timing"  // Check frame time budgets
	ValidatorMemoryBudget ValidatorID = "memory-budget" // Check memory allocation limits
	ValidatorShaderCheck  ValidatorID = "shader-check"  // Validate shader compilation

	// F-011: High-Frequency Trading
	ValidatorLatency  ValidatorID = "latency"   // Measure p50/p95/p99 latency
	ValidatorLockFree ValidatorID = "lock-free" // Verify lock-free algorithms
	ValidatorCache    ValidatorID = "cache"     // Check cache-friendly access patterns

	// F-012: Embedded Systems
	ValidatorStackSize ValidatorID = "stack-size" // Check stack usage
	ValidatorInterrupt ValidatorID = "interrupt"  // Interrupt safety analysis
	ValidatorRealTime  ValidatorID = "real-time"  // Real-time constraint checking
	ValidatorROMSize   ValidatorID = "rom-size"   // Check binary size limits

	// F-013: Security
	ValidatorFuzz      ValidatorID = "fuzz"       // Fuzzing with AFL/libFuzzer
	ValidatorSecStatic ValidatorID = "sec-static" // Security-focused static analysis
	ValidatorInput     ValidatorID = "input"      // Input validation checks

	// F-014: Performance
	ValidatorBenchmark  ValidatorID = "benchmark"  // Google Benchmark comparison
	ValidatorMemProfile ValidatorID = "mem-prof"   // Memory profiling
	ValidatorCPUProfile ValidatorID = "cpu-prof"   // CPU profiling
	ValidatorFlameGraph ValidatorID = "flamegraph" // Flame graph generation
)

// ValidatorCategory groups validators by domain
type ValidatorCategory string

const (
	CategoryCore        ValidatorCategory = "core"
	CategoryGame        ValidatorCategory = "game"
	CategoryHFT         ValidatorCategory = "hft"
	CategoryEmbedded    ValidatorCategory = "embedded"
	CategorySecurity    ValidatorCategory = "security"
	CategoryPerformance ValidatorCategory = "performance"
)

// ValidatorInfo describes a validation gate
type ValidatorInfo struct {
	ID          ValidatorID
	Name        string
	Description string
	Category    ValidatorCategory
	Enabled     bool   // Default enabled state
	RequiresArg bool   // Requires additional configuration
	ArgHelp     string // Help text for argument
}

// AllValidators returns all known validators with their info
func AllValidators() []ValidatorInfo {
	return []ValidatorInfo{
		// Core validators (enabled by default)
		{ValidatorClangTidy, "clang-tidy", "Static analysis", CategoryCore, true, false, ""},
		{ValidatorCppcheck, "cppcheck", "Deep static analysis", CategoryCore, true, false, ""},
		{ValidatorIWYU, "include-what-you-use", "Header hygiene (advisory)", CategoryCore, true, false, ""},
		{ValidatorComplexity, "complexity", "Cyclomatic complexity check (CCN≤15)", CategoryCore, true, false, ""},
		{ValidatorCompile, "compile", "Compile with -Wall -Wextra -Werror", CategoryCore, true, false, ""},
		{ValidatorASAN, "AddressSanitizer", "Memory errors (heap/stack overflow, use-after-free)", CategoryCore, true, false, ""},
		{ValidatorUBSAN, "UBSanitizer", "Undefined behavior", CategoryCore, true, false, ""},
		{ValidatorMSAN, "MemorySanitizer", "Uninitialized memory reads", CategoryCore, true, false, ""},
		{ValidatorTSAN, "ThreadSanitizer", "Data races (auto-enabled for threaded code)", CategoryCore, true, false, ""},
		{ValidatorRun, "run", "Execute and verify output", CategoryCore, true, false, ""},
		{ValidatorReview, "review", "LLM code review (confidence scoring)", CategoryCore, true, false, ""},

		// Game Development (F-010)
		{ValidatorFrameTiming, "Frame Timing", "Check 16.67ms (60fps) / 33.33ms (30fps) budget", CategoryGame, false, true, "target_fps=60"},
		{ValidatorMemoryBudget, "Memory Budget", "Check allocation limits", CategoryGame, false, true, "max_mb=512"},
		{ValidatorShaderCheck, "Shader Check", "Validate GLSL/HLSL compilation", CategoryGame, false, false, ""},

		// HFT (F-011)
		{ValidatorLatency, "Latency", "Measure p50/p95/p99 latency", CategoryHFT, false, true, "p99_us=100"},
		{ValidatorLockFree, "Lock-Free", "Verify lock-free properties", CategoryHFT, false, false, ""},
		{ValidatorCache, "Cache Analysis", "Check cache-friendly patterns", CategoryHFT, false, false, ""},

		// Embedded (F-012)
		{ValidatorStackSize, "Stack Size", "Analyze stack usage", CategoryEmbedded, false, true, "max_kb=8"},
		{ValidatorInterrupt, "Interrupt Safety", "Check ISR constraints", CategoryEmbedded, false, false, ""},
		{ValidatorRealTime, "Real-Time", "WCET analysis", CategoryEmbedded, false, true, "deadline_us=1000"},
		{ValidatorROMSize, "ROM Size", "Check binary size", CategoryEmbedded, false, true, "max_kb=256"},

		// Security (F-013)
		{ValidatorFuzz, "Fuzzing", "AFL++/libFuzzer testing", CategorySecurity, false, true, "iterations=10000"},
		{ValidatorSecStatic, "Security Analysis", "CWE/CERT checks", CategorySecurity, false, false, ""},
		{ValidatorInput, "Input Validation", "Check input handling", CategorySecurity, false, false, ""},

		// Performance (F-014)
		{ValidatorBenchmark, "Benchmark", "Google Benchmark comparison", CategoryPerformance, false, true, "baseline="},
		{ValidatorMemProfile, "Memory Profile", "Heap profiling", CategoryPerformance, false, false, ""},
		{ValidatorCPUProfile, "CPU Profile", "CPU sampling", CategoryPerformance, false, false, ""},
		{ValidatorFlameGraph, "Flame Graph", "Generate flame graph", CategoryPerformance, false, false, ""},
	}
}

// ValidatorConfig holds the configuration for enabled validators
type ValidatorConfig struct {
	Enabled map[ValidatorID]bool
	Args    map[ValidatorID]string // Additional arguments per validator
}

// DefaultValidatorConfig returns the default validator configuration
// Core validators enabled, domain-specific disabled
func DefaultValidatorConfig() *ValidatorConfig {
	cfg := &ValidatorConfig{
		Enabled: make(map[ValidatorID]bool),
		Args:    make(map[ValidatorID]string),
	}

	for _, v := range AllValidators() {
		cfg.Enabled[v.ID] = v.Enabled
		if v.RequiresArg && v.ArgHelp != "" {
			cfg.Args[v.ID] = v.ArgHelp // Store default arg
		}
	}

	return cfg
}

// GetValidatorsByCategory returns validators grouped by category
func GetValidatorsByCategory() map[ValidatorCategory][]ValidatorInfo {
	result := make(map[ValidatorCategory][]ValidatorInfo)
	for _, v := range AllValidators() {
		result[v.Category] = append(result[v.Category], v)
	}
	return result
}

// IsEnabled checks if a validator is enabled
func (vc *ValidatorConfig) IsEnabled(id ValidatorID) bool {
	enabled, ok := vc.Enabled[id]
	return ok && enabled
}

// Toggle enables/disables a validator
func (vc *ValidatorConfig) Toggle(id ValidatorID) bool {
	vc.Enabled[id] = !vc.Enabled[id]
	return vc.Enabled[id]
}

// SetArg sets an argument for a validator
func (vc *ValidatorConfig) SetArg(id ValidatorID, arg string) {
	vc.Args[id] = arg
}

// GetArg gets the argument for a validator
func (vc *ValidatorConfig) GetArg(id ValidatorID) string {
	return vc.Args[id]
}

// EnableCategory enables all validators in a category
func (vc *ValidatorConfig) EnableCategory(cat ValidatorCategory) {
	for _, v := range AllValidators() {
		if v.Category == cat {
			vc.Enabled[v.ID] = true
		}
	}
}

// DisableCategory disables all validators in a category
func (vc *ValidatorConfig) DisableCategory(cat ValidatorCategory) {
	for _, v := range AllValidators() {
		if v.Category == cat {
			vc.Enabled[v.ID] = false
		}
	}
}
