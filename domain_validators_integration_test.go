package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSecurityStaticValidatorIntegration tests the security static analysis validator
func TestSecurityStaticValidatorIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create a container runtime for testing
	container, err := DetectContainerRuntime()
	if err != nil {
		t.Skipf("No container runtime available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Test code with known security issues
	insecureCode := `
#include <stdio.h>
#include <string.h>

int main() {
    char buffer[10];
    gets(buffer);  // CWE-120: dangerous function
    char dest[5];
    strcpy(dest, buffer);  // CWE-120: buffer overflow
    printf("%s\n", dest);
    return 0;
}
`

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "bjarne-sectest-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Write code to temp file
	codePath := filepath.Join(tmpDir, "insecure.cpp")
	if err := os.WriteFile(codePath, []byte(insecureCode), 0600); err != nil {
		t.Fatalf("Failed to write code: %v", err)
	}

	// Run security static validator
	result := container.runSecurityStaticValidator(ctx, tmpDir, insecureCode, "insecure.cpp")

	// Check that it detected issues
	if result.Success {
		t.Log("Output:", result.Output)
		// Note: Success may be true if container/tools not available
		// but we should see warnings in output
	}

	// Check output contains expected warnings
	if !strings.Contains(result.Output, "gets") && !strings.Contains(result.Output, "strcpy") {
		t.Logf("Expected security warnings about gets/strcpy. Got: %s", result.Output)
	}

	t.Logf("Security validator output:\n%s", result.Output)
}

// TestROMSizeValidatorIntegration tests the ROM size validator
func TestROMSizeValidatorIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	container, err := DetectContainerRuntime()
	if err != nil {
		t.Skipf("No container runtime available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Simple code that should have small binary size
	smallCode := `
#include <iostream>
int main() {
    std::cout << "Hello" << std::endl;
    return 0;
}
`

	tmpDir, err := os.MkdirTemp("", "bjarne-romtest-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	codePath := filepath.Join(tmpDir, "small.cpp")
	if err := os.WriteFile(codePath, []byte(smallCode), 0600); err != nil {
		t.Fatalf("Failed to write code: %v", err)
	}

	// Run ROM size validator with 1MB limit (should pass)
	result := container.runROMSizeValidator(ctx, tmpDir, smallCode, "small.cpp", "max_kb=1024")

	t.Logf("ROM size validator output:\n%s", result.Output)

	if !result.Success && !strings.Contains(result.Output, "Binary size") {
		t.Errorf("Expected ROM size check to run. Output: %s", result.Output)
	}
}

// TestCacheValidatorIntegration tests cache-friendliness analysis
func TestCacheValidatorIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	container, err := DetectContainerRuntime()
	if err != nil {
		t.Skipf("No container runtime available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Code with potential cache issues
	cacheUnfriendlyCode := `
#include <list>
#include <atomic>

struct Counter {
    std::atomic<int> value;  // No alignas - potential false sharing
};

int main() {
    std::list<int> items;  // Linked list - cache unfriendly
    items.push_back(1);
    return 0;
}
`

	tmpDir, err := os.MkdirTemp("", "bjarne-cachetest-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	codePath := filepath.Join(tmpDir, "cache.cpp")
	if err := os.WriteFile(codePath, []byte(cacheUnfriendlyCode), 0600); err != nil {
		t.Fatalf("Failed to write code: %v", err)
	}

	result := container.runCacheValidator(ctx, tmpDir, cacheUnfriendlyCode, "cache.cpp")

	t.Logf("Cache validator output:\n%s", result.Output)

	// Should contain warnings about atomic without alignas and list usage
	if !strings.Contains(result.Output, "atomic") && !strings.Contains(result.Output, "list") {
		t.Logf("Expected cache warnings. Got: %s", result.Output)
	}
}

// TestRunDomainValidatorsIntegration tests the full domain validator pipeline
func TestRunDomainValidatorsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	container, err := DetectContainerRuntime()
	if err != nil {
		t.Skipf("No container runtime available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Simple valid code
	code := `
#include <iostream>
int main() {
    std::cout << "Hello, World!" << std::endl;
    return 0;
}
`

	tmpDir, err := os.MkdirTemp("", "bjarne-domaintest-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	codePath := filepath.Join(tmpDir, "test.cpp")
	if err := os.WriteFile(codePath, []byte(code), 0600); err != nil {
		t.Fatalf("Failed to write code: %v", err)
	}

	// Create config with security validators enabled
	config := DefaultValidatorConfig()
	config.EnableCategory(CategorySecurity)

	// Run domain validators
	results := container.RunDomainValidators(ctx, tmpDir, code, "test.cpp", config)

	t.Logf("Domain validators returned %d results", len(results))

	for _, r := range results {
		t.Logf("Validator %s: success=%v\n%s", r.ValidatorID, r.Success, r.Output)
	}

	// Should have run security validators
	if len(results) == 0 {
		t.Error("Expected some domain validator results")
	}

	// Check that security validators were run
	foundSecStatic := false
	foundInput := false
	for _, r := range results {
		if r.ValidatorID == ValidatorSecStatic {
			foundSecStatic = true
		}
		if r.ValidatorID == ValidatorInput {
			foundInput = true
		}
	}

	if !foundSecStatic {
		t.Error("Expected sec-static validator to run")
	}
	if !foundInput {
		t.Error("Expected input validator to run")
	}
}
