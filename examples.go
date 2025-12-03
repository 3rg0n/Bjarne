package main

import (
	"fmt"
	"regexp"
	"strings"
)

// TestCase represents a single example test case from user prompt
type TestCase struct {
	FunctionCall string // e.g., "isPalindrome("aba")"
	Expected     string // e.g., "true"
	Line         int    // Original line number in prompt
}

// ExampleTests holds parsed test cases from a user prompt
type ExampleTests struct {
	Tests        []TestCase
	FunctionName string // Inferred function name
}

// ParseExampleTests extracts test cases from a user prompt
// Supports formats like:
//   - isPalindrome("") -> true
//   - isPalindrome("aba") => true
//   - isPalindrome("abc") should return false
//   - Input: "hello" Output: "olleh"
func ParseExampleTests(prompt string) *ExampleTests {
	var tests []TestCase
	var funcName string

	lines := strings.Split(prompt, "\n")

	// Pattern 1: function(args) -> expected
	// Match: isPalindrome("aba") -> true
	arrowPattern := regexp.MustCompile(`^\s*(\w+)\s*\(([^)]*)\)\s*(?:->|=>|:)\s*(.+?)\s*$`)

	// Pattern 2: function(args) should return expected
	// Match: isPalindrome("abc") should return false
	shouldPattern := regexp.MustCompile(`^\s*(\w+)\s*\(([^)]*)\)\s+(?:should\s+)?return[s]?\s+(.+?)\s*$`)

	// Pattern 3: Input/Output style
	// Match: Input: "hello" Output: "olleh"
	ioPattern := regexp.MustCompile(`(?i)^\s*Input:\s*(.+?)\s+Output:\s*(.+?)\s*$`)

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try arrow pattern
		if matches := arrowPattern.FindStringSubmatch(line); len(matches) >= 4 {
			name := matches[1]
			args := matches[2]
			expected := strings.TrimSpace(matches[3])

			if funcName == "" {
				funcName = name
			}

			tests = append(tests, TestCase{
				FunctionCall: fmt.Sprintf("%s(%s)", name, args),
				Expected:     expected,
				Line:         i + 1,
			})
			continue
		}

		// Try should return pattern
		if matches := shouldPattern.FindStringSubmatch(line); len(matches) >= 4 {
			name := matches[1]
			args := matches[2]
			expected := strings.TrimSpace(matches[3])

			if funcName == "" {
				funcName = name
			}

			tests = append(tests, TestCase{
				FunctionCall: fmt.Sprintf("%s(%s)", name, args),
				Expected:     expected,
				Line:         i + 1,
			})
			continue
		}

		// Try I/O pattern (needs function name from context)
		if matches := ioPattern.FindStringSubmatch(line); len(matches) >= 3 {
			input := strings.TrimSpace(matches[1])
			output := strings.TrimSpace(matches[2])

			// For I/O patterns, we'll use a placeholder that needs to be filled
			tests = append(tests, TestCase{
				FunctionCall: fmt.Sprintf("FUNCTION(%s)", input),
				Expected:     output,
				Line:         i + 1,
			})
		}
	}

	if len(tests) == 0 {
		return nil
	}

	return &ExampleTests{
		Tests:        tests,
		FunctionName: funcName,
	}
}

// GenerateTestHarness creates a C++ test harness for the example tests
// The harness wraps the user's code and validates it against the examples
func GenerateTestHarness(code string, examples *ExampleTests) string {
	if examples == nil || len(examples.Tests) == 0 {
		return code
	}

	var sb strings.Builder

	// Add iostream for test output
	sb.WriteString("#include <iostream>\n")
	sb.WriteString("#include <sstream>\n")
	sb.WriteString("#include <string>\n\n")

	// Add a simple test framework
	sb.WriteString("// Test framework\n")
	sb.WriteString("static int _test_passed = 0;\n")
	sb.WriteString("static int _test_failed = 0;\n\n")

	sb.WriteString("#define EXPECT_EQ(actual, expected, test_name) do { \\\n")
	sb.WriteString("    if ((actual) == (expected)) { \\\n")
	sb.WriteString("        std::cout << \"PASS: \" << test_name << std::endl; \\\n")
	sb.WriteString("        _test_passed++; \\\n")
	sb.WriteString("    } else { \\\n")
	sb.WriteString("        std::cout << \"FAIL: \" << test_name << std::endl; \\\n")
	sb.WriteString("        std::cout << \"  Expected: \" << (expected) << std::endl; \\\n")
	sb.WriteString("        std::cout << \"  Actual:   \" << (actual) << std::endl; \\\n")
	sb.WriteString("        _test_failed++; \\\n")
	sb.WriteString("    } \\\n")
	sb.WriteString("} while(0)\n\n")

	// Include the user's code, but strip their main() if present
	userCode := stripMainFunction(code)
	sb.WriteString("// User code (main stripped)\n")
	sb.WriteString(userCode)
	sb.WriteString("\n\n")

	// Generate test main
	sb.WriteString("// Generated test main\n")
	sb.WriteString("int main() {\n")
	sb.WriteString("    std::cout << \"Running example tests...\" << std::endl;\n")
	sb.WriteString("    std::cout << std::endl;\n\n")

	for i, test := range examples.Tests {
		testName := fmt.Sprintf("Test %d: %s", i+1, test.FunctionCall)
		sb.WriteString(fmt.Sprintf("    // Test from line %d\n", test.Line))
		sb.WriteString(fmt.Sprintf("    EXPECT_EQ(%s, %s, \"%s\");\n\n",
			test.FunctionCall, test.Expected, escapeString(testName)))
	}

	sb.WriteString("    std::cout << std::endl;\n")
	sb.WriteString("    std::cout << \"Results: \" << _test_passed << \" passed, \" << _test_failed << \" failed\" << std::endl;\n")
	sb.WriteString("    return _test_failed > 0 ? 1 : 0;\n")
	sb.WriteString("}\n")

	return sb.String()
}

// stripMainFunction removes main() from user code to allow test harness to provide its own
func stripMainFunction(code string) string {
	// Simple approach: look for "int main" and remove the function
	// This is a heuristic - a proper parser would be better

	mainPattern := regexp.MustCompile(`(?s)\bint\s+main\s*\([^)]*\)\s*\{`)
	loc := mainPattern.FindStringIndex(code)
	if loc == nil {
		return code
	}

	// Find the matching closing brace
	start := loc[0]
	braceStart := loc[1] - 1 // Position of opening brace
	depth := 1
	end := braceStart + 1

	for end < len(code) && depth > 0 {
		switch code[end] {
		case '{':
			depth++
		case '}':
			depth--
		}
		end++
	}

	// Remove main function
	return code[:start] + code[end:]
}

// escapeString escapes a string for C++ string literal
func escapeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// HasExampleTests checks if a prompt contains example test cases
func HasExampleTests(prompt string) bool {
	examples := ParseExampleTests(prompt)
	return examples != nil && len(examples.Tests) > 0
}
