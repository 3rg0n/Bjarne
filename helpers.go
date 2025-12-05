package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// handleFirstRunPull handles the first-run container pull experience
func handleFirstRunPull(ctx context.Context, container *ContainerRuntime) error {
	fmt.Println()
	fmt.Println("\033[93m┌─────────────────────────────────────────────────────────────┐\033[0m")
	fmt.Println("\033[93m│                     First-time Setup                        │\033[0m")
	fmt.Println("\033[93m└─────────────────────────────────────────────────────────────┘\033[0m")
	fmt.Println()
	fmt.Println("bjarne requires a validation container to check your C/C++ code")
	fmt.Println("for memory errors, undefined behavior, and data races.")
	fmt.Println()
	fmt.Printf("Container image: \033[96m%s\033[0m\n", container.imageName)
	fmt.Printf("Size: ~500MB (Ubuntu-based with Clang 21 + sanitizers)\n")
	fmt.Println()
	fmt.Print("Pull the validation container now? [Y/n] ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "" && response != "y" && response != "yes" {
		return fmt.Errorf("container pull declined")
	}

	fmt.Println()
	fmt.Println("\033[93mPulling container image...\033[0m")
	fmt.Println("(This may take a few minutes on first run)")
	fmt.Println()

	if err := container.PullImage(ctx); err != nil {
		return fmt.Errorf("failed to pull container: %w", err)
	}

	fmt.Println()
	fmt.Println("\033[92mContainer ready!\033[0m")
	return nil
}

// parseDifficulty extracts the difficulty tag from bjarne's reflection
// Returns the difficulty level (EASY, MEDIUM, COMPLEX) and the text without the tag
func parseDifficulty(text string) (string, string) {
	text = strings.TrimSpace(text)

	// Check for difficulty tags at the start
	for _, level := range []string{"EASY", "MEDIUM", "COMPLEX"} {
		tag := "[" + level + "]"
		if strings.HasPrefix(text, tag) {
			// Remove the tag and any following whitespace/newline
			remainder := strings.TrimPrefix(text, tag)
			remainder = strings.TrimLeft(remainder, " \t\n")
			return level, remainder
		}
	}

	// No tag found - default to MEDIUM (requires confirmation)
	return "MEDIUM", text
}

// CodeFile represents a single source file in a multi-file project
type CodeFile struct {
	Filename string
	Content  string
}

// extractCode extracts code from a markdown code block
// For single file responses, returns the code content
// For multi-file responses, returns all files concatenated (use extractMultipleFiles instead)
func extractCode(response string) string {
	files := extractMultipleFiles(response)
	if len(files) == 0 {
		return ""
	}
	if len(files) == 1 {
		return files[0].Content
	}
	// For backwards compatibility, return all content if multiple files
	var sb strings.Builder
	for i, f := range files {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("// FILE: " + f.Filename + "\n")
		sb.WriteString(f.Content)
	}
	return sb.String()
}

// extractMultipleFiles extracts multiple code files from an LLM response
// Returns a slice of CodeFile, each with filename and content
// If no // FILE: markers are found, returns single file with default name
func extractMultipleFiles(response string) []CodeFile {
	// Normalize line endings (Windows \r\n to \n)
	response = strings.ReplaceAll(response, "\r\n", "\n")

	var files []CodeFile

	// Match all code blocks: ```cpp ... ``` or ```c ... ``` or ```c++ ... ```
	re := regexp.MustCompile("(?s)```(?:cpp|c\\+\\+|c)?[ \t]*\n(.*?)\n?```")
	matches := re.FindAllStringSubmatch(response, -1)

	if len(matches) == 0 {
		// Fallback: try truncated response (no closing ```)
		reOpen := regexp.MustCompile("(?s)```(?:cpp|c\\+\\+|c)[ \t]*\n(.+)")
		matches = reOpen.FindAllStringSubmatch(response, -1)
		if len(matches) == 0 {
			return nil
		}
	}

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		content := strings.TrimSpace(match[1])
		if content == "" {
			continue
		}

		// Check for // FILE: marker at the start
		filename := detectFilename(content)
		if filename != "" {
			// Remove the FILE: line from content
			lines := strings.SplitN(content, "\n", 2)
			if len(lines) > 1 {
				content = strings.TrimSpace(lines[1])
			} else {
				content = ""
			}
		}

		if content != "" {
			files = append(files, CodeFile{
				Filename: filename,
				Content:  content,
			})
		}
	}

	// If no filenames detected, assign defaults
	if len(files) == 1 && files[0].Filename == "" {
		files[0].Filename = "code.cpp"
	} else if len(files) > 1 {
		hasMain := false
		for i := range files {
			if files[i].Filename == "" {
				// Try to detect from content
				files[i].Filename = inferFilename(files[i].Content, i)
			}
			if files[i].Filename == "main.cpp" || strings.Contains(files[i].Content, "int main(") {
				hasMain = true
			}
		}
		// If no main.cpp, assign it to the first file with main()
		if !hasMain {
			for i := range files {
				if strings.Contains(files[i].Content, "int main(") {
					files[i].Filename = "main.cpp"
					break
				}
			}
		}
	}

	return files
}

// detectFilename looks for // FILE: marker at the start of content
func detectFilename(content string) string {
	lines := strings.SplitN(content, "\n", 2)
	if len(lines) == 0 {
		return ""
	}
	firstLine := strings.TrimSpace(lines[0])

	// Match // FILE: filename.ext or /* FILE: filename.ext */
	patterns := []string{
		`^//\s*FILE:\s*(\S+)`,
		`^/\*\s*FILE:\s*(\S+)\s*\*/`,
	}
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(firstLine)
		if len(matches) >= 2 {
			return matches[1]
		}
	}

	return ""
}

// inferFilename tries to guess a filename from content
func inferFilename(content string, index int) string {
	// Check for #pragma once or header guards -> it's a header
	if strings.Contains(content, "#pragma once") || regexp.MustCompile(`#ifndef\s+\w+_H`).MatchString(content) {
		// Try to find class/struct name
		classRe := regexp.MustCompile(`(?:class|struct)\s+(\w+)`)
		if match := classRe.FindStringSubmatch(content); len(match) >= 2 {
			return strings.ToLower(match[1]) + ".h"
		}
		return fmt.Sprintf("header%d.h", index)
	}

	// Check for main function
	if strings.Contains(content, "int main(") {
		return "main.cpp"
	}

	// Check if it looks like implementation (includes local header)
	if regexp.MustCompile(`#include\s*"[^"]+\.h"`).MatchString(content) {
		// Find the first local include
		includeRe := regexp.MustCompile(`#include\s*"([^"]+)\.h"`)
		if match := includeRe.FindStringSubmatch(content); len(match) >= 2 {
			return match[1] + ".cpp"
		}
	}

	return fmt.Sprintf("file%d.cpp", index)
}

// IsMultiFileProject checks if the extracted files represent a multi-file project
func IsMultiFileProject(files []CodeFile) bool {
	if len(files) <= 1 {
		return false
	}
	// Check if any file is a header
	for _, f := range files {
		if strings.HasSuffix(f.Filename, ".h") || strings.HasSuffix(f.Filename, ".hpp") {
			return true
		}
	}
	return len(files) > 1
}

// saveToFile writes code to a file
func saveToFile(filename, code string) error {
	return os.WriteFile(filename, []byte(code), 0600)
}

// stripMarkdown removes common markdown formatting from text for terminal display
func stripMarkdown(text string) string {
	// Remove code blocks entirely (```...```)
	re := regexp.MustCompile("(?s)```[a-z]*\\s*\n.*?```")
	text = re.ReplaceAllString(text, "")

	// Remove headers (# ## ### etc) - keep the text
	re = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	text = re.ReplaceAllString(text, "")

	// Remove horizontal rules (--- or ***)
	re = regexp.MustCompile(`(?m)^[-*]{3,}\s*$`)
	text = re.ReplaceAllString(text, "")

	// Remove table formatting - convert to simple lines
	// First, remove table separator lines (|---|---|)
	re = regexp.MustCompile(`(?m)^\|[-:|\s]+\|\s*$`)
	text = re.ReplaceAllString(text, "")
	// Then clean up table rows (| cell | cell |) -> cell, cell
	re = regexp.MustCompile(`(?m)^\|\s*`)
	text = re.ReplaceAllString(text, "")
	re = regexp.MustCompile(`(?m)\s*\|$`)
	text = re.ReplaceAllString(text, "")
	re = regexp.MustCompile(`\s*\|\s*`)
	text = re.ReplaceAllString(text, " | ")

	// Remove bold (**text** or __text__)
	re = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	text = re.ReplaceAllString(text, "$1")
	re = regexp.MustCompile(`__([^_]+)__`)
	text = re.ReplaceAllString(text, "$1")

	// Remove italic (*text* or _text_) - be careful not to match bullet points
	re = regexp.MustCompile(`(?:^|[^*])\*([^*\n]+)\*(?:[^*]|$)`)
	text = re.ReplaceAllString(text, "$1")

	// Remove inline code (`text`)
	re = regexp.MustCompile("`([^`]+)`")
	text = re.ReplaceAllString(text, "$1")

	// Clean up multiple blank lines
	re = regexp.MustCompile(`\n{3,}`)
	text = re.ReplaceAllString(text, "\n\n")

	return strings.TrimSpace(text)
}

// wrapText wraps text to a specified width, preserving paragraph breaks
func wrapText(text string, width int) []string {
	var result []string
	paragraphs := strings.Split(text, "\n")

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			result = append(result, "")
			continue
		}

		// Wrap this paragraph
		words := strings.Fields(para)
		if len(words) == 0 {
			continue
		}

		var line string
		for _, word := range words {
			if line == "" {
				line = word
			} else if len(line)+1+len(word) <= width {
				line += " " + word
			} else {
				result = append(result, line)
				line = word
			}
		}
		if line != "" {
			result = append(result, line)
		}
	}

	return result
}

// containsQuestion checks if text contains a question that needs user response
// Used to determine if we should wait for user input even for EASY tasks
func containsQuestion(text string) bool {
	text = strings.ToLower(text)

	// Check for question marks in non-rhetorical context
	if strings.Contains(text, "?") {
		// Common question patterns that indicate waiting for user
		questionPatterns := []string{
			"what's the context",
			"what is the context",
			"what are you",
			"what do you",
			"help me understand",
			"can you clarify",
			"could you clarify",
			"can you explain",
			"could you explain",
			"what would you",
			"correct me if",
			"any corrections",
			"sound good",
			"does that work",
			"is that correct",
			"is that right",
			"let me know",
			"what's your",
			"what is your",
		}
		for _, pattern := range questionPatterns {
			if strings.Contains(text, pattern) {
				return true
			}
		}

		// Check if ends with a question (last sentence has ?)
		lines := strings.Split(strings.TrimSpace(text), "\n")
		if len(lines) > 0 {
			lastLine := strings.TrimSpace(lines[len(lines)-1])
			if strings.HasSuffix(lastLine, "?") {
				return true
			}
		}
	}

	return false
}

// shortModelName extracts a readable model name from the full ID
func shortModelName(modelID string) string {
	// global.anthropic.claude-haiku-4-5-20251001-v1:0 -> claude-haiku-4-5
	parts := strings.Split(modelID, ".")
	if len(parts) >= 3 {
		modelPart := parts[2] // claude-haiku-4-5-20251001-v1:0
		// Remove version suffix like -20251001-v1:0
		if idx := strings.Index(modelPart, "-202"); idx > 0 {
			return modelPart[:idx]
		}
		return modelPart
	}
	return modelID
}
