package main

import (
	"strings"
	"testing"
)

func TestParseExampleTests(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		wantLen  int
		wantFunc string
	}{
		{
			name: "arrow syntax",
			prompt: `Write a function to check palindromes
Tests:
  isPalindrome("") -> true
  isPalindrome("aba") -> true
  isPalindrome("abc") -> false`,
			wantLen:  3,
			wantFunc: "isPalindrome",
		},
		{
			name: "double arrow syntax",
			prompt: `isPalindrome("racecar") => true
isPalindrome("hello") => false`,
			wantLen:  2,
			wantFunc: "isPalindrome",
		},
		{
			name: "should return syntax",
			prompt: `factorial(5) should return 120
factorial(0) should return 1
factorial(1) returns 1`,
			wantLen:  3,
			wantFunc: "factorial",
		},
		{
			name:    "no tests",
			prompt:  "Write me a hello world program",
			wantLen: 0,
		},
		{
			name: "mixed formats",
			prompt: `sum(1, 2) -> 3
sum(0, 0) => 0
sum(-1, 1) should return 0`,
			wantLen:  3,
			wantFunc: "sum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseExampleTests(tt.prompt)
			if tt.wantLen == 0 {
				if result != nil && len(result.Tests) > 0 {
					t.Errorf("ParseExampleTests() expected no tests, got %d", len(result.Tests))
				}
				return
			}

			if result == nil {
				t.Fatal("ParseExampleTests() returned nil, expected tests")
			}
			if len(result.Tests) != tt.wantLen {
				t.Errorf("ParseExampleTests() got %d tests, want %d", len(result.Tests), tt.wantLen)
			}
			if tt.wantFunc != "" && result.FunctionName != tt.wantFunc {
				t.Errorf("ParseExampleTests() function name = %q, want %q", result.FunctionName, tt.wantFunc)
			}
		})
	}
}

func TestStripMainFunction(t *testing.T) {
	code := `#include <iostream>

bool isPalindrome(const std::string& s) {
    int left = 0, right = s.length() - 1;
    while (left < right) {
        if (s[left++] != s[right--]) return false;
    }
    return true;
}

int main() {
    std::cout << isPalindrome("aba") << std::endl;
    return 0;
}`

	result := stripMainFunction(code)

	if strings.Contains(result, "int main") {
		t.Error("stripMainFunction() did not remove main()")
	}
	if !strings.Contains(result, "isPalindrome") {
		t.Error("stripMainFunction() removed too much code")
	}
}

func TestGenerateTestHarness(t *testing.T) {
	code := `#include <string>

bool isPalindrome(const std::string& s) {
    int left = 0, right = s.length() - 1;
    while (left < right) {
        if (s[left++] != s[right--]) return false;
    }
    return true;
}

int main() {
    return 0;
}`

	examples := &ExampleTests{
		Tests: []TestCase{
			{FunctionCall: `isPalindrome("")`, Expected: "true", Line: 1},
			{FunctionCall: `isPalindrome("aba")`, Expected: "true", Line: 2},
			{FunctionCall: `isPalindrome("abc")`, Expected: "false", Line: 3},
		},
		FunctionName: "isPalindrome",
	}

	harness := GenerateTestHarness(code, examples)

	// Check harness has test framework
	if !strings.Contains(harness, "EXPECT_EQ") {
		t.Error("Harness missing EXPECT_EQ macro")
	}

	// Check harness has test calls
	if !strings.Contains(harness, `isPalindrome("")`) {
		t.Error("Harness missing test case 1")
	}
	if !strings.Contains(harness, `isPalindrome("aba")`) {
		t.Error("Harness missing test case 2")
	}

	// Check original main is stripped
	if strings.Contains(harness, "return 0;\n}") && strings.Count(harness, "int main") > 1 {
		t.Error("Harness has multiple main functions")
	}
}

func TestHasExampleTests(t *testing.T) {
	tests := []struct {
		prompt string
		want   bool
	}{
		{"Write hello world", false},
		{"factorial(5) -> 120", true},
		{"sum(1,2) should return 3", true},
		{"", false},
	}

	for _, tt := range tests {
		got := HasExampleTests(tt.prompt)
		if got != tt.want {
			t.Errorf("HasExampleTests(%q) = %v, want %v", tt.prompt, got, tt.want)
		}
	}
}
