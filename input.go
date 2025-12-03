package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	// PasteCollapseThreshold is the minimum number of lines before collapsing pasted text
	PasteCollapseThreshold = 5
	// PasteLineTimeout is max time between lines to consider it part of the same paste
	PasteLineTimeout = 150 * time.Millisecond
)

// InputReader handles reading input with paste detection
type InputReader struct {
	scanner  *bufio.Scanner
	theme    *Theme
	pasteNum int
	lineChan chan string
	errChan  chan error
}

// NewInputReader creates a new input reader
func NewInputReader(theme *Theme) *InputReader {
	ir := &InputReader{
		scanner:  bufio.NewScanner(os.Stdin),
		theme:    theme,
		pasteNum: 0,
		lineChan: make(chan string),
		errChan:  make(chan error),
	}

	// Start background reader
	go ir.backgroundReader()

	return ir
}

// backgroundReader continuously reads lines and sends them to the channel
func (ir *InputReader) backgroundReader() {
	for ir.scanner.Scan() {
		ir.lineChan <- ir.scanner.Text()
	}
	if err := ir.scanner.Err(); err != nil {
		ir.errChan <- err
	}
	close(ir.lineChan)
	close(ir.errChan)
}

// ReadInput reads user input, detecting and collapsing multi-line pastes
// Returns the full input text and a display string (collapsed if pasted)
func (ir *InputReader) ReadInput() (fullText string, displayText string, err error) {
	var lines []string

	// Wait for first line (blocking)
	select {
	case line, ok := <-ir.lineChan:
		if !ok {
			return "", "", fmt.Errorf("input closed")
		}
		lines = append(lines, line)
	case err := <-ir.errChan:
		return "", "", err
	}

	// Try to read more lines with timeout (paste detection)
	// If lines come in rapidly, it's a paste
collecting:
	for {
		select {
		case line, ok := <-ir.lineChan:
			if !ok {
				break collecting
			}
			lines = append(lines, line)
		case <-time.After(PasteLineTimeout):
			// No more lines within timeout - done
			break collecting
		}
	}

	fullText = strings.Join(lines, "\n")

	// Format display based on line count
	if len(lines) > PasteCollapseThreshold {
		ir.pasteNum++
		displayText = ir.formatCollapsedPaste(lines)
	} else if len(lines) > 1 {
		// Show indicator for small pastes
		displayText = fmt.Sprintf("%s %s+%d lines%s",
			lines[0],
			ir.theme.Dim(""),
			len(lines)-1,
			ir.theme.Reset())
	} else {
		displayText = fullText
	}

	return fullText, displayText, nil
}

// formatCollapsedPaste formats a collapsed paste display
func (ir *InputReader) formatCollapsedPaste(lines []string) string {
	firstLine := lines[0]
	if len(firstLine) > 50 {
		firstLine = firstLine[:47] + "..."
	}

	additionalLines := len(lines) - 1
	return fmt.Sprintf("%s[Pasted text #%d]%s %s %s+%d lines%s",
		ir.theme.Accent(""),
		ir.pasteNum,
		ir.theme.Reset(),
		firstLine,
		ir.theme.Dim(""),
		additionalLines,
		ir.theme.Reset(),
	)
}

// Close cleans up resources
func (ir *InputReader) Close() {
	// Scanner will be cleaned up when stdin closes
}
