package main

import (
	"testing"
)

func TestCodeUsesThreads(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{
			name:     "no threads",
			code:     "#include <iostream>\nint main() { return 0; }",
			expected: false,
		},
		{
			name:     "std::thread",
			code:     "#include <thread>\nint main() { std::thread t; return 0; }",
			expected: true,
		},
		{
			name:     "std::mutex",
			code:     "#include <mutex>\nstd::mutex m;",
			expected: true,
		},
		{
			name:     "std::atomic",
			code:     "#include <atomic>\nstd::atomic<int> counter;",
			expected: true,
		},
		{
			name:     "pthread",
			code:     "#include <pthread.h>\nvoid* func(void*) { return 0; }",
			expected: true,
		},
		{
			name:     "std::async",
			code:     "#include <future>\nauto f = std::async([](){});",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := codeUsesThreads(tt.code)
			if result != tt.expected {
				t.Errorf("codeUsesThreads(%q) = %v, want %v", tt.code, result, tt.expected)
			}
		})
	}
}

func TestGetImageName(t *testing.T) {
	// Default image name
	name := getImageName()
	if name != "bjarne-validator:local" {
		t.Errorf("getImageName() = %q, want default image name", name)
	}

	// With environment override
	t.Setenv("BJARNE_VALIDATOR_IMAGE", "custom:test")
	name = getImageName()
	if name != "custom:test" {
		t.Errorf("getImageName() with BJARNE_VALIDATOR_IMAGE = %q, want %q", name, "custom:test")
	}
}

func TestFormatResults(t *testing.T) {
	results := []ValidationResult{
		{Stage: "clang-tidy", Success: true, Duration: 100000000}, // 0.1s
		{Stage: "compile", Success: true, Duration: 200000000},    // 0.2s
		{Stage: "asan", Success: false, Error: "memory error", Duration: 300000000},
	}

	output := FormatResults(results)

	// Check that output contains expected strings
	if !contains(output, "PASS clang-tidy") {
		t.Error("FormatResults missing clang-tidy success")
	}
	if !contains(output, "PASS compile") {
		t.Error("FormatResults missing compile success")
	}
	if !contains(output, "FAIL asan") {
		t.Error("FormatResults missing asan failure")
	}
	if !contains(output, "memory error") {
		t.Error("FormatResults missing error message")
	}
	if contains(output, "All validation stages passed") {
		t.Error("FormatResults should not say all passed when there's a failure")
	}
}

func TestFormatResultsAllPassed(t *testing.T) {
	results := []ValidationResult{
		{Stage: "clang-tidy", Success: true, Duration: 100000000},
		{Stage: "compile", Success: true, Duration: 200000000},
	}

	output := FormatResults(results)

	if !contains(output, "All validation stages passed") {
		t.Error("FormatResults should say all passed when all succeeded")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
