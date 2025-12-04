package main

import (
	"strings"
	"testing"
)

func TestParseClangTidyOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected int // number of diagnostics
		checkMsg string
	}{
		{
			name:     "single warning",
			output:   "/src/code.cpp:10:5: warning: unused variable 'x' [clang-diagnostic-unused-variable]",
			expected: 1,
			checkMsg: "unused variable",
		},
		{
			name:     "error without check name",
			output:   "/src/code.cpp:5:1: error: expected ';' after expression",
			expected: 1,
			checkMsg: "expected ';'",
		},
		{
			name: "multiple diagnostics",
			output: `/src/code.cpp:10:5: warning: unused variable 'x' [clang-diagnostic-unused-variable]
/src/code.cpp:15:10: warning: use of old-style cast [cppcoreguidelines-pro-type-cstyle-cast]
/src/code.cpp:20:1: error: unknown type name 'foo'`,
			expected: 3,
		},
		{
			name:     "empty output",
			output:   "",
			expected: 0,
		},
		{
			name:     "no diagnostics",
			output:   "3 warnings generated.\n",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := ParseClangTidyOutput(tt.output)
			if len(diags) != tt.expected {
				t.Errorf("ParseClangTidyOutput() returned %d diagnostics, want %d", len(diags), tt.expected)
			}
			if tt.checkMsg != "" && len(diags) > 0 {
				if !strings.Contains(diags[0].Message, tt.checkMsg) {
					t.Errorf("diagnostic message %q does not contain %q", diags[0].Message, tt.checkMsg)
				}
			}
		})
	}
}

func TestParseClangTidyOutputDetails(t *testing.T) {
	output := "/src/code.cpp:10:5: warning: unused variable 'x' [clang-diagnostic-unused-variable]"
	diags := ParseClangTidyOutput(output)

	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}

	d := diags[0]
	if d.File != "/src/code.cpp" {
		t.Errorf("File = %q, want /src/code.cpp", d.File)
	}
	if d.Line != 10 {
		t.Errorf("Line = %d, want 10", d.Line)
	}
	if d.Column != 5 {
		t.Errorf("Column = %d, want 5", d.Column)
	}
	if d.Level != LevelWarning {
		t.Errorf("Level = %q, want warning", d.Level)
	}
	if d.Check != "clang-diagnostic-unused-variable" {
		t.Errorf("Check = %q, want clang-diagnostic-unused-variable", d.Check)
	}
}

func TestParseSanitizerOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		sanType  string
		expected int
		checkMsg string
	}{
		{
			name: "asan heap-buffer-overflow",
			output: `=================================================================
==12345==ERROR: AddressSanitizer: heap-buffer-overflow on address 0x602000000014
    #0 0x4c3b2a in main /src/code.cpp:10
    #1 0x7f1234567890 in __libc_start_main`,
			sanType:  "asan",
			expected: 1,
			checkMsg: "heap-buffer-overflow",
		},
		{
			name:     "ubsan signed overflow",
			output:   `/src/code.cpp:15:10: runtime error: signed integer overflow: 2147483647 + 1 cannot be represented in type 'int'`,
			sanType:  "ubsan",
			expected: 1,
			checkMsg: "signed integer overflow",
		},
		{
			name: "tsan data race",
			output: `==================
WARNING: ThreadSanitizer: data race (pid=12345)
  Write of size 4 at 0x7b3c00000000 by thread T1:
    #0 increment /src/code.cpp:20`,
			sanType:  "tsan",
			expected: 1,
			checkMsg: "data race",
		},
		{
			name:     "empty output",
			output:   "",
			sanType:  "asan",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := ParseSanitizerOutput(tt.output, tt.sanType)
			if len(diags) != tt.expected {
				t.Errorf("ParseSanitizerOutput() returned %d diagnostics, want %d", len(diags), tt.expected)
			}
			if tt.checkMsg != "" && len(diags) > 0 {
				if !strings.Contains(diags[0].Message, tt.checkMsg) {
					t.Errorf("diagnostic message %q does not contain %q", diags[0].Message, tt.checkMsg)
				}
			}
		})
	}
}

func TestFormatDiagnostics(t *testing.T) {
	diags := []Diagnostic{
		{
			File:    "/src/code.cpp",
			Line:    10,
			Column:  5,
			Level:   LevelWarning,
			Message: "unused variable 'x'",
			Check:   "clang-diagnostic-unused-variable",
		},
	}

	output := FormatDiagnostics(diags)

	if !strings.Contains(output, "warning") {
		t.Error("FormatDiagnostics missing 'warning'")
	}
	if !strings.Contains(output, "unused variable") {
		t.Error("FormatDiagnostics missing message")
	}
	if !strings.Contains(output, "/src/code.cpp:10:5") {
		t.Error("FormatDiagnostics missing location")
	}
	if !strings.Contains(output, "clang-diagnostic-unused-variable") {
		t.Error("FormatDiagnostics missing check name")
	}
}

func TestFormatDiagnosticsEmpty(t *testing.T) {
	output := FormatDiagnostics(nil)
	if output != "" {
		t.Errorf("FormatDiagnostics(nil) = %q, want empty", output)
	}
}

func TestFormatDiagnosticsForLLM(t *testing.T) {
	diags := []Diagnostic{
		{
			File:    "/src/code.cpp",
			Line:    15,
			Column:  3,
			Level:   LevelWarning,
			Message: "use nullptr instead of NULL",
			Check:   "modernize-use-nullptr",
		},
		{
			File:    "/src/code.cpp",
			Line:    23,
			Column:  1,
			Level:   LevelError,
			Message: "undeclared identifier 'foo'",
			Check:   "",
		},
	}

	output := FormatDiagnosticsForLLM(diags)

	// Should strip /src/ prefix
	if strings.Contains(output, "/src/") {
		t.Error("FormatDiagnosticsForLLM should strip /src/ prefix")
	}

	// Should contain file:line format
	if !strings.Contains(output, "code.cpp:15") {
		t.Error("FormatDiagnosticsForLLM missing 'code.cpp:15'")
	}

	// Should contain check name
	if !strings.Contains(output, "modernize-use-nullptr:") {
		t.Error("FormatDiagnosticsForLLM missing check name")
	}

	// Should contain message
	if !strings.Contains(output, "use nullptr instead of NULL") {
		t.Error("FormatDiagnosticsForLLM missing message")
	}

	// Should NOT contain ANSI color codes
	if strings.Contains(output, "\033[") {
		t.Error("FormatDiagnosticsForLLM should not contain ANSI color codes")
	}

	// For error without check name, should use level as prefix
	if !strings.Contains(output, "error:") {
		t.Error("FormatDiagnosticsForLLM should fall back to level when no check name")
	}
}

func TestFormatDiagnosticsForLLMEmpty(t *testing.T) {
	output := FormatDiagnosticsForLLM(nil)
	if output != "" {
		t.Errorf("FormatDiagnosticsForLLM(nil) = %q, want empty", output)
	}
}

func TestIntToStr(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{9999, "9999"},
	}

	for _, tt := range tests {
		result := intToStr(tt.input)
		if result != tt.expected {
			t.Errorf("intToStr(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
