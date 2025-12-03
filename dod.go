package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// DefinitionOfDone captures testable acceptance criteria
type DefinitionOfDone struct {
	// Correctness tests - input -> expected output
	Examples []TestCase

	// Behavioral requirements
	HandleEmpty    bool // Should handle empty input
	HandleNegative bool // Should handle negative numbers
	ThreadSafe     bool // Must be thread-safe
	NoAllocation   bool // No dynamic allocation (embedded/real-time)

	// Performance requirements (testable via benchmark)
	MaxTimeMs   int // Max execution time in ms for benchmark
	MaxMemoryMB int // Max memory usage in MB
	BenchmarkN  int // Number of items to benchmark with

	// What bjarne cannot test (informational only)
	CannotTest []string
}

// DoDPrompt is the system prompt for collecting Definition of Done
const DoDPrompt = `You are bjarne. The user has described a complex task. Before generating code, you need a Definition of Done that you can actually test.

Your job is to ask the user for TESTABLE acceptance criteria. Be direct and specific.

WHAT YOU CAN TEST (in the validation container):
- Specific input/output examples: "process(5) returns 120"
- Edge case handling: empty input, negative numbers, overflow
- Thread safety: via ThreadSanitizer
- Memory safety: via AddressSanitizer
- Performance benchmarks: "sort(10000) completes in <100ms"
- Complexity: functions under 100 lines, cyclomatic complexity <15

WHAT YOU CANNOT TEST:
- Production deployment
- Real network load (100K connections)
- External service integration
- Actual hardware constraints

FORMAT YOUR RESPONSE AS:
1. Summarize what you understand they want (1-2 sentences)
2. List what you CAN test with specific questions:
   - "What should X return for input Y?"
   - "Should it handle empty input? Negative numbers?"
   - "Any performance target I can benchmark? (e.g., N items in <X ms)"
3. List what you CANNOT test (be honest)
4. End with: "Once you answer, I'll generate tests first, then the code."

Be concise. No fluff. Channel Bjarne's directness.`

// ParseDefinitionOfDone extracts DoD from user's response
func ParseDefinitionOfDone(response string) *DefinitionOfDone {
	dod := &DefinitionOfDone{}

	// Parse examples (function(args) -> result patterns)
	examplePattern := regexp.MustCompile(`(\w+)\s*\(([^)]*)\)\s*(?:->|=>|returns?|should return)\s*(.+?)(?:\n|$)`)
	matches := examplePattern.FindAllStringSubmatch(response, -1)
	for _, match := range matches {
		if len(match) >= 4 {
			dod.Examples = append(dod.Examples, TestCase{
				FunctionCall: fmt.Sprintf("%s(%s)", match[1], match[2]),
				Expected:     strings.TrimSpace(match[3]),
			})
		}
	}

	// Parse behavioral flags
	responseLower := strings.ToLower(response)

	if strings.Contains(responseLower, "empty") && containsYes(responseLower, "empty") {
		dod.HandleEmpty = true
	}
	if strings.Contains(responseLower, "negative") && containsYes(responseLower, "negative") {
		dod.HandleNegative = true
	}
	// Check for thread safety - handle both "thread-safe" and "thread safe"
	if strings.Contains(responseLower, "thread-safe") || strings.Contains(responseLower, "thread safe") ||
		strings.Contains(responseLower, "threadsafe") {
		// If it mentions thread safety at all, check if it's affirmative
		if !strings.Contains(responseLower, "not thread") && !strings.Contains(responseLower, "no thread") {
			dod.ThreadSafe = true
		}
	} else if strings.Contains(responseLower, "thread") && containsYes(responseLower, "thread") {
		dod.ThreadSafe = true
	}

	// Parse performance requirements
	// Pattern: "N items in <X ms" or "< X ms" or "under X ms"
	perfPattern := regexp.MustCompile(`(\d+)\s*(?:items?|elements?)?\s*(?:in\s*)?[<]?\s*(\d+)\s*ms`)
	if match := perfPattern.FindStringSubmatch(responseLower); len(match) >= 3 {
		dod.BenchmarkN, _ = strconv.Atoi(match[1])
		dod.MaxTimeMs, _ = strconv.Atoi(match[2])
	}

	// Simpler pattern: just "< X ms" or "under X ms"
	simpleTimePattern := regexp.MustCompile(`(?:under|<|less than)\s*(\d+)\s*ms`)
	if match := simpleTimePattern.FindStringSubmatch(responseLower); len(match) >= 2 {
		dod.MaxTimeMs, _ = strconv.Atoi(match[1])
		if dod.BenchmarkN == 0 {
			dod.BenchmarkN = 1000 // Default benchmark size
		}
	}

	return dod
}

// containsYes checks if the response indicates "yes" for a feature
func containsYes(response, feature string) bool {
	// Look for patterns like "handle empty: yes" or "empty input - yes" or just context suggesting yes
	idx := strings.Index(response, feature)
	if idx < 0 {
		return false
	}

	// Check nearby text for affirmative - expand the window
	start := idx
	if start > 50 {
		start = idx - 50
	}
	end := idx + len(feature) + 50
	if end > len(response) {
		end = len(response)
	}
	context := response[start:end]

	affirmatives := []string{"yes", "true", "should", "must", "need", "needs", "want", "handle", "be ", "is "}
	negatives := []string{" no ", "false", "don't", "doesn't", " not ", "skip", "without"}

	hasAffirmative := false
	hasNegative := false
	for _, a := range affirmatives {
		if strings.Contains(context, a) {
			hasAffirmative = true
			break
		}
	}
	for _, n := range negatives {
		if strings.Contains(context, n) {
			hasNegative = true
			break
		}
	}

	return hasAffirmative && !hasNegative
}

// HasTestableRequirements checks if DoD has anything we can actually test
func (d *DefinitionOfDone) HasTestableRequirements() bool {
	return len(d.Examples) > 0 ||
		d.HandleEmpty ||
		d.HandleNegative ||
		d.ThreadSafe ||
		d.MaxTimeMs > 0
}

// ToExampleTests converts DoD into ExampleTests for validation
func (d *DefinitionOfDone) ToExampleTests() *ExampleTests {
	if len(d.Examples) == 0 {
		return nil
	}

	funcName := ""
	if len(d.Examples) > 0 {
		// Extract function name from first example
		parts := strings.Split(d.Examples[0].FunctionCall, "(")
		if len(parts) > 0 {
			funcName = strings.TrimSpace(parts[0])
		}
	}

	return &ExampleTests{
		Tests:        d.Examples,
		FunctionName: funcName,
	}
}

// GenerateBenchmarkHarness creates a benchmark test for performance requirements
func (d *DefinitionOfDone) GenerateBenchmarkHarness(code, funcName string) string {
	if d.MaxTimeMs == 0 {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("#include <iostream>\n")
	sb.WriteString("#include <chrono>\n")
	sb.WriteString("#include <vector>\n\n")

	// Include user code (strip main)
	userCode := stripMainFunction(code)
	sb.WriteString("// User code\n")
	sb.WriteString(userCode)
	sb.WriteString("\n\n")

	// Generate benchmark main
	sb.WriteString("int main() {\n")
	sb.WriteString("    using namespace std::chrono;\n\n")

	sb.WriteString(fmt.Sprintf("    const int N = %d;\n", d.BenchmarkN))
	sb.WriteString(fmt.Sprintf("    const int MAX_MS = %d;\n\n", d.MaxTimeMs))

	sb.WriteString("    // Warmup\n")
	sb.WriteString("    for (int i = 0; i < 10; i++) {\n")
	sb.WriteString(fmt.Sprintf("        %s; // warmup call\n", funcName))
	sb.WriteString("    }\n\n")

	sb.WriteString("    // Benchmark\n")
	sb.WriteString("    auto start = high_resolution_clock::now();\n")
	sb.WriteString("    for (int i = 0; i < N; i++) {\n")
	sb.WriteString(fmt.Sprintf("        %s;\n", funcName))
	sb.WriteString("    }\n")
	sb.WriteString("    auto end = high_resolution_clock::now();\n\n")

	sb.WriteString("    auto duration = duration_cast<milliseconds>(end - start).count();\n")
	sb.WriteString("    double avg_ms = static_cast<double>(duration) / N;\n\n")

	sb.WriteString("    std::cout << \"Benchmark: \" << N << \" iterations in \" << duration << \"ms\" << std::endl;\n")
	sb.WriteString("    std::cout << \"Average: \" << avg_ms << \"ms per call\" << std::endl;\n\n")

	sb.WriteString("    if (duration > MAX_MS) {\n")
	sb.WriteString("        std::cout << \"FAIL: Exceeded \" << MAX_MS << \"ms threshold\" << std::endl;\n")
	sb.WriteString("        return 1;\n")
	sb.WriteString("    }\n\n")

	sb.WriteString("    std::cout << \"PASS: Within \" << MAX_MS << \"ms threshold\" << std::endl;\n")
	sb.WriteString("    return 0;\n")
	sb.WriteString("}\n")

	return sb.String()
}

// FormatDoDSummary creates a human-readable summary of the DoD
func (d *DefinitionOfDone) FormatDoDSummary() string {
	var parts []string

	if len(d.Examples) > 0 {
		parts = append(parts, fmt.Sprintf("%d example test(s)", len(d.Examples)))
	}
	if d.HandleEmpty {
		parts = append(parts, "handle empty input")
	}
	if d.HandleNegative {
		parts = append(parts, "handle negative numbers")
	}
	if d.ThreadSafe {
		parts = append(parts, "thread-safe")
	}
	if d.MaxTimeMs > 0 {
		parts = append(parts, fmt.Sprintf("<%dms for %d items", d.MaxTimeMs, d.BenchmarkN))
	}

	if len(parts) == 0 {
		return "No testable requirements specified"
	}

	return strings.Join(parts, ", ")
}
