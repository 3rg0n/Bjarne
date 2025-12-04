package main

import (
	"testing"
)

func TestExtractCode(t *testing.T) {
	tests := []struct {
		name     string
		response string
		expected string
	}{
		{
			name:     "cpp code block",
			response: "Here is the code:\n```cpp\nint main() { return 0; }\n```\nDone.",
			expected: "int main() { return 0; }",
		},
		{
			name:     "c code block",
			response: "```c\n#include <stdio.h>\nint main() { return 0; }\n```",
			expected: "#include <stdio.h>\nint main() { return 0; }",
		},
		{
			name:     "generic code block",
			response: "```\nsome code\n```",
			expected: "some code",
		},
		{
			name:     "no code block",
			response: "Just some text without code",
			expected: "",
		},
		{
			name:     "empty code block",
			response: "```cpp\n\n```",
			expected: "",
		},
		{
			name:     "c++ variant",
			response: "```c++\nint x = 42;\n```",
			expected: "int x = 42;",
		},
		{
			name:     "multiple code blocks returns first",
			response: "```cpp\nfirst\n```\ntext\n```cpp\nsecond\n```",
			expected: "first",
		},
		{
			name:     "truncated response (no closing fence)",
			response: "Here's the code:\n```cpp\nint main() {\n    return 0;\n}",
			expected: "int main() {\n    return 0;\n}",
		},
		{
			name:     "windows line endings",
			response: "```cpp\r\nint x = 1;\r\n```",
			expected: "int x = 1;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCode(tt.response)
			if result != tt.expected {
				t.Errorf("extractCode() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestParseDifficulty(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantDifficulty string
		wantText       string
	}{
		{
			name:           "easy tag",
			input:          "[EASY]\nToo easy. Give me a moment...",
			wantDifficulty: "EASY",
			wantText:       "Too easy. Give me a moment...",
		},
		{
			name:           "medium tag",
			input:          "[MEDIUM]\nI'll use a vector for this. Sound good?",
			wantDifficulty: "MEDIUM",
			wantText:       "I'll use a vector for this. Sound good?",
		},
		{
			name:           "complex tag",
			input:          "[COMPLEX]\nThis requires careful thread synchronization...",
			wantDifficulty: "COMPLEX",
			wantText:       "This requires careful thread synchronization...",
		},
		{
			name:           "no tag defaults to medium",
			input:          "Let me think about this...",
			wantDifficulty: "MEDIUM",
			wantText:       "Let me think about this...",
		},
		{
			name:           "tag with space after",
			input:          "[EASY] Child's play.",
			wantDifficulty: "EASY",
			wantText:       "Child's play.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			difficulty, text := parseDifficulty(tt.input)
			if difficulty != tt.wantDifficulty {
				t.Errorf("parseDifficulty() difficulty = %q, want %q", difficulty, tt.wantDifficulty)
			}
			if text != tt.wantText {
				t.Errorf("parseDifficulty() text = %q, want %q", text, tt.wantText)
			}
		})
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	// Test default value
	result := getEnvOrDefault("NONEXISTENT_VAR_12345", "default")
	if result != "default" {
		t.Errorf("getEnvOrDefault() = %q, want %q", result, "default")
	}

	// Test with set value
	t.Setenv("TEST_VAR_BJARNE", "custom_value")
	result = getEnvOrDefault("TEST_VAR_BJARNE", "default")
	if result != "custom_value" {
		t.Errorf("getEnvOrDefault() = %q, want %q", result, "custom_value")
	}
}
