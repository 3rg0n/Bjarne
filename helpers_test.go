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
			name:     "multiple code blocks returns all with FILE markers",
			response: "```cpp\nfirst\n```\ntext\n```cpp\nsecond\n```",
			expected: "// FILE: file0.cpp\nfirst\n\n// FILE: file1.cpp\nsecond",
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

func TestExtractMultipleFiles(t *testing.T) {
	tests := []struct {
		name     string
		response string
		expected []CodeFile
	}{
		{
			name:     "single file without FILE marker",
			response: "```cpp\nint main() { return 0; }\n```",
			expected: []CodeFile{
				{Filename: "code.cpp", Content: "int main() { return 0; }"},
			},
		},
		{
			name:     "single file with FILE marker",
			response: "```cpp\n// FILE: main.cpp\nint main() { return 0; }\n```",
			expected: []CodeFile{
				{Filename: "main.cpp", Content: "int main() { return 0; }"},
			},
		},
		{
			name:     "multiple files with FILE markers",
			response: "```cpp\n// FILE: counter.h\n#pragma once\nclass Counter {};\n```\n\n```cpp\n// FILE: counter.cpp\n#include \"counter.h\"\n```\n\n```cpp\n// FILE: main.cpp\nint main() {}\n```",
			expected: []CodeFile{
				{Filename: "counter.h", Content: "#pragma once\nclass Counter {};"},
				{Filename: "counter.cpp", Content: "#include \"counter.h\""},
				{Filename: "main.cpp", Content: "int main() {}"},
			},
		},
		{
			name:     "multiple files without FILE markers - infers names",
			response: "```cpp\n#pragma once\nclass Foo {};\n```\n\n```cpp\nint main() {}\n```",
			expected: []CodeFile{
				{Filename: "foo.h", Content: "#pragma once\nclass Foo {};"},
				{Filename: "main.cpp", Content: "int main() {}"},
			},
		},
		{
			name:     "header detection from pragma once",
			response: "```cpp\n#pragma once\nstruct Data { int x; };\n```",
			expected: []CodeFile{
				{Filename: "code.cpp", Content: "#pragma once\nstruct Data { int x; };"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMultipleFiles(tt.response)
			if len(result) != len(tt.expected) {
				t.Errorf("extractMultipleFiles() returned %d files, want %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i].Filename != tt.expected[i].Filename {
					t.Errorf("file %d: Filename = %q, want %q", i, result[i].Filename, tt.expected[i].Filename)
				}
				if result[i].Content != tt.expected[i].Content {
					t.Errorf("file %d: Content = %q, want %q", i, result[i].Content, tt.expected[i].Content)
				}
			}
		})
	}
}

func TestIsMultiFileProject(t *testing.T) {
	tests := []struct {
		name     string
		files    []CodeFile
		expected bool
	}{
		{
			name:     "single file",
			files:    []CodeFile{{Filename: "main.cpp"}},
			expected: false,
		},
		{
			name:     "multiple cpp files",
			files:    []CodeFile{{Filename: "a.cpp"}, {Filename: "b.cpp"}},
			expected: true,
		},
		{
			name:     "header and implementation",
			files:    []CodeFile{{Filename: "foo.h"}, {Filename: "foo.cpp"}},
			expected: true,
		},
		{
			name:     "empty",
			files:    []CodeFile{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsMultiFileProject(tt.files)
			if result != tt.expected {
				t.Errorf("IsMultiFileProject() = %v, want %v", result, tt.expected)
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
