package main

import (
	"regexp"
	"strings"
)

// DiagnosticLevel represents the severity of a diagnostic
type DiagnosticLevel string

const (
	LevelError   DiagnosticLevel = "error"
	LevelWarning DiagnosticLevel = "warning"
	LevelNote    DiagnosticLevel = "note"
)

// Diagnostic represents a single diagnostic message from clang-tidy or sanitizers
type Diagnostic struct {
	File    string
	Line    int
	Column  int
	Level   DiagnosticLevel
	Message string
	Check   string // clang-tidy check name (e.g., "bugprone-unused-return-value")
	Context string // Additional context lines
}

// ParseClangTidyOutput parses clang-tidy output into structured diagnostics
func ParseClangTidyOutput(output string) []Diagnostic {
	var diagnostics []Diagnostic

	// Pattern: /src/code.cpp:10:5: warning: some message [check-name]
	re := regexp.MustCompile(`(?m)^([^:]+):(\d+):(\d+): (error|warning|note): (.+?)(?:\s+\[([^\]]+)\])?$`)

	matches := re.FindAllStringSubmatch(output, -1)
	for _, match := range matches {
		if len(match) >= 6 {
			line := 0
			col := 0
			parseIntSafe(match[2], &line)
			parseIntSafe(match[3], &col)

			d := Diagnostic{
				File:    match[1],
				Line:    line,
				Column:  col,
				Level:   DiagnosticLevel(match[4]),
				Message: match[5],
			}
			if len(match) >= 7 && match[6] != "" {
				d.Check = match[6]
			}
			diagnostics = append(diagnostics, d)
		}
	}

	return diagnostics
}

// ParseCppcheckOutput parses cppcheck output into structured diagnostics
func ParseCppcheckOutput(output string) []Diagnostic {
	var diagnostics []Diagnostic

	// cppcheck patterns:
	// [/src/code.cpp:10]: (error) Message text
	// /src/code.cpp:10:5: error: Message text [errorId]
	re := regexp.MustCompile(`(?m)^(?:\[)?([^:\]]+):(\d+)(?::(\d+))?(?:\])?: \((error|warning|style|performance|portability|information)\) (.+)$`)
	re2 := regexp.MustCompile(`(?m)^([^:]+):(\d+):(\d+): (error|warning|note): (.+?) \[([^\]]+)\]$`)

	// Try standard format first
	matches := re.FindAllStringSubmatch(output, -1)
	for _, match := range matches {
		if len(match) >= 6 {
			line := 0
			col := 0
			parseIntSafe(match[2], &line)
			if len(match) >= 4 && match[3] != "" {
				parseIntSafe(match[3], &col)
			}

			level := LevelWarning
			if match[4] == "error" {
				level = LevelError
			}

			diagnostics = append(diagnostics, Diagnostic{
				File:    match[1],
				Line:    line,
				Column:  col,
				Level:   level,
				Message: match[5],
				Check:   "cppcheck-" + match[4],
			})
		}
	}

	// Try GCC-style format
	if len(diagnostics) == 0 {
		matches = re2.FindAllStringSubmatch(output, -1)
		for _, match := range matches {
			if len(match) >= 7 {
				line := 0
				col := 0
				parseIntSafe(match[2], &line)
				parseIntSafe(match[3], &col)

				level := LevelWarning
				if match[4] == "error" {
					level = LevelError
				} else if match[4] == "note" {
					level = LevelNote
				}

				diagnostics = append(diagnostics, Diagnostic{
					File:    match[1],
					Line:    line,
					Column:  col,
					Level:   level,
					Message: match[5],
					Check:   match[6],
				})
			}
		}
	}

	return diagnostics
}

// ParseSanitizerOutput parses ASAN/UBSAN/TSAN output into structured diagnostics
func ParseSanitizerOutput(output string, sanitizerType string) []Diagnostic {
	var diagnostics []Diagnostic

	// ASAN pattern: ==PID==ERROR: AddressSanitizer: heap-buffer-overflow
	// UBSAN pattern: code.cpp:10:5: runtime error: ...
	// TSAN pattern: WARNING: ThreadSanitizer: data race

	lines := strings.Split(output, "\n")
	var currentDiag *Diagnostic

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// ASAN/TSAN summary line
		if strings.Contains(line, "ERROR: AddressSanitizer:") ||
			strings.Contains(line, "ERROR: LeakSanitizer:") ||
			strings.Contains(line, "WARNING: ThreadSanitizer:") {

			if currentDiag != nil {
				diagnostics = append(diagnostics, *currentDiag)
			}
			currentDiag = &Diagnostic{
				Level:   LevelError,
				Message: extractSanitizerMessage(line),
				Check:   sanitizerType,
			}
			continue
		}

		// UBSAN runtime error pattern
		if strings.Contains(line, "runtime error:") {
			if currentDiag != nil {
				diagnostics = append(diagnostics, *currentDiag)
			}
			d := parseUBSANLine(line)
			d.Check = "ubsan"
			currentDiag = &d
			continue
		}

		// Stack trace location: #0 0x... in func /path/file.cpp:10
		if strings.HasPrefix(line, "#") && currentDiag != nil {
			if loc := extractStackLocation(line); loc != "" {
				if currentDiag.Context == "" {
					currentDiag.Context = loc
				} else {
					currentDiag.Context += "\n" + loc
				}
			}
		}

		// Limit context to avoid huge outputs
		if currentDiag != nil && i > 0 && len(currentDiag.Context) < 500 {
			if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
				// Code snippet or additional context
				if currentDiag.Context != "" {
					currentDiag.Context += "\n"
				}
				currentDiag.Context += line
			}
		}
	}

	if currentDiag != nil {
		diagnostics = append(diagnostics, *currentDiag)
	}

	return diagnostics
}

// FormatDiagnostics formats diagnostics for user display
func FormatDiagnostics(diagnostics []Diagnostic) string {
	if len(diagnostics) == 0 {
		return ""
	}

	var sb strings.Builder

	for _, d := range diagnostics {
		// Format: [error/warning] message (check-name)
		//         at file:line:col
		//         context...

		levelColor := "\033[91m" // red for error
		if d.Level == LevelWarning {
			levelColor = "\033[93m" // yellow for warning
		} else if d.Level == LevelNote {
			levelColor = "\033[94m" // blue for note
		}

		sb.WriteString(levelColor)
		sb.WriteString(string(d.Level))
		sb.WriteString("\033[0m: ")
		sb.WriteString(d.Message)

		if d.Check != "" {
			sb.WriteString(" \033[90m[")
			sb.WriteString(d.Check)
			sb.WriteString("]\033[0m")
		}
		sb.WriteString("\n")

		if d.File != "" {
			sb.WriteString("  at ")
			sb.WriteString(d.File)
			if d.Line > 0 {
				sb.WriteString(":")
				sb.WriteString(intToStr(d.Line))
				if d.Column > 0 {
					sb.WriteString(":")
					sb.WriteString(intToStr(d.Column))
				}
			}
			sb.WriteString("\n")
		}

		if d.Context != "" {
			// Indent context lines
			contextLines := strings.Split(d.Context, "\n")
			for _, cl := range contextLines {
				if cl != "" {
					sb.WriteString("  ")
					sb.WriteString(cl)
					sb.WriteString("\n")
				}
			}
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

// Helper functions

func parseIntSafe(s string, out *int) {
	val := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			val = val*10 + int(c-'0')
		} else {
			return
		}
	}
	*out = val
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func extractSanitizerMessage(line string) string {
	// Extract the error type after "ERROR: AddressSanitizer: " or similar
	patterns := []string{
		"ERROR: AddressSanitizer: ",
		"ERROR: LeakSanitizer: ",
		"WARNING: ThreadSanitizer: ",
	}

	for _, prefix := range patterns {
		if idx := strings.Index(line, prefix); idx >= 0 {
			msg := line[idx+len(prefix):]
			// Trim trailing location info
			if endIdx := strings.Index(msg, " on address"); endIdx > 0 {
				msg = msg[:endIdx]
			}
			return strings.TrimSpace(msg)
		}
	}

	return line
}

func parseUBSANLine(line string) Diagnostic {
	d := Diagnostic{Level: LevelError}

	// Pattern: /path/file.cpp:10:5: runtime error: message
	re := regexp.MustCompile(`^([^:]+):(\d+):(\d+): runtime error: (.+)$`)
	if match := re.FindStringSubmatch(line); len(match) >= 5 {
		d.File = match[1]
		parseIntSafe(match[2], &d.Line)
		parseIntSafe(match[3], &d.Column)
		d.Message = match[4]
	} else {
		// Fallback: just extract message after "runtime error:"
		if idx := strings.Index(line, "runtime error:"); idx >= 0 {
			d.Message = strings.TrimSpace(line[idx+14:])
		} else {
			d.Message = line
		}
	}

	return d
}

func extractStackLocation(line string) string {
	// Pattern: #0 0x... in func_name /path/file.cpp:10
	re := regexp.MustCompile(`#\d+\s+\S+\s+in\s+(\S+)\s+([^:]+):(\d+)`)
	if match := re.FindStringSubmatch(line); len(match) >= 4 {
		return match[1] + " at " + match[2] + ":" + match[3]
	}
	return ""
}
