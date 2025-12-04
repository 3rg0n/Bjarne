package main

import (
	"strings"
	"testing"
)

func TestParseDefinitionOfDone(t *testing.T) {
	tests := []struct {
		name           string
		response       string
		wantExamples   int
		wantMaxTime    int
		wantThreadSafe bool
	}{
		{
			name: "examples with arrow",
			response: `Here's what I need:
factorial(5) -> 120
factorial(0) -> 1
factorial(1) returns 1`,
			wantExamples: 3,
		},
		{
			name:        "performance requirement",
			response:    "It should handle 1000 items in under 50ms",
			wantMaxTime: 50,
		},
		{
			name:           "thread safety",
			response:       "Yes, it needs to be thread-safe for concurrent access",
			wantThreadSafe: true,
		},
		{
			name: "combined requirements",
			response: `process(10) -> 100
Should be thread-safe: yes
Performance: 10000 items in < 100 ms`,
			wantExamples:   1,
			wantMaxTime:    100,
			wantThreadSafe: true,
		},
		{
			name:        "simple time requirement",
			response:    "Should complete in under 20ms",
			wantMaxTime: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dod := ParseDefinitionOfDone(tt.response)

			if tt.wantExamples > 0 && len(dod.Examples) != tt.wantExamples {
				t.Errorf("Examples: got %d, want %d", len(dod.Examples), tt.wantExamples)
			}
			if tt.wantMaxTime > 0 && dod.MaxTimeMs != tt.wantMaxTime {
				t.Errorf("MaxTimeMs: got %d, want %d", dod.MaxTimeMs, tt.wantMaxTime)
			}
			if tt.wantThreadSafe && !dod.ThreadSafe {
				t.Error("Expected ThreadSafe to be true")
			}
		})
	}
}

func TestDoDHasTestableRequirements(t *testing.T) {
	tests := []struct {
		name string
		dod  DefinitionOfDone
		want bool
	}{
		{
			name: "empty",
			dod:  DefinitionOfDone{},
			want: false,
		},
		{
			name: "has examples",
			dod:  DefinitionOfDone{Examples: []TestCase{{FunctionCall: "f()", Expected: "1"}}},
			want: true,
		},
		{
			name: "has performance",
			dod:  DefinitionOfDone{MaxTimeMs: 100},
			want: true,
		},
		{
			name: "thread safe flag",
			dod:  DefinitionOfDone{ThreadSafe: true},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dod.HasTestableRequirements()
			if got != tt.want {
				t.Errorf("HasTestableRequirements() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDoDFormatSummary(t *testing.T) {
	dod := &DefinitionOfDone{
		Examples: []TestCase{
			{FunctionCall: "f(1)", Expected: "2"},
			{FunctionCall: "f(2)", Expected: "4"},
		},
		ThreadSafe: true,
		MaxTimeMs:  100,
		BenchmarkN: 1000,
	}

	summary := dod.FormatDoDSummary()

	if !strings.Contains(summary, "2 example test(s)") {
		t.Error("Summary should mention example tests")
	}
	if !strings.Contains(summary, "thread-safe") {
		t.Error("Summary should mention thread safety")
	}
	if !strings.Contains(summary, "<100ms") {
		t.Error("Summary should mention time constraint")
	}
}

func TestDoDToExampleTests(t *testing.T) {
	dod := &DefinitionOfDone{
		Examples: []TestCase{
			{FunctionCall: "isPrime(7)", Expected: "true"},
			{FunctionCall: "isPrime(4)", Expected: "false"},
		},
	}

	examples := dod.ToExampleTests()

	if examples == nil {
		t.Fatal("ToExampleTests returned nil")
	}
	if len(examples.Tests) != 2 {
		t.Errorf("Got %d tests, want 2", len(examples.Tests))
	}
	if examples.FunctionName != "isPrime" {
		t.Errorf("FunctionName = %q, want %q", examples.FunctionName, "isPrime")
	}
}

func TestGenerateBenchmarkHarness(t *testing.T) {
	dod := &DefinitionOfDone{
		MaxTimeMs:  100,
		BenchmarkN: 1000,
	}

	code := `int compute() { return 42; }`
	harness := dod.GenerateBenchmarkHarness(code, "compute()")

	if harness == "" {
		t.Fatal("GenerateBenchmarkHarness returned empty string")
	}
	if !strings.Contains(harness, "const int N = 1000") {
		t.Error("Harness should contain benchmark count")
	}
	if !strings.Contains(harness, "const int MAX_MS = 100") {
		t.Error("Harness should contain max time")
	}
	if !strings.Contains(harness, "compute()") {
		t.Error("Harness should call the function")
	}
}

func TestParseProperties(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		wantProps []string
		wantCount int
	}{
		{
			name:      "roundtrip property",
			response:  "yes, it should have roundtrip property - encode then decode gives original",
			wantProps: []string{"roundtrip"},
			wantCount: 1,
		},
		{
			name:      "idempotent property",
			response:  "sorting should be idempotent - sorting twice is same as once",
			wantProps: []string{"idempotent"},
			wantCount: 1,
		},
		{
			name:      "commutative property",
			response:  "the operation is commutative, order doesn't matter",
			wantProps: []string{"commutative"},
			wantCount: 1,
		},
		{
			name:      "invariant property",
			response:  "there's an invariant - size is always positive",
			wantProps: []string{"invariant"},
			wantCount: 1,
		},
		{
			name:      "multiple properties",
			response:  "it should be idempotent and have a roundtrip with its inverse",
			wantProps: []string{"roundtrip", "idempotent"},
			wantCount: 2,
		},
		{
			name:      "reverse twice pattern",
			response:  "reverse twice should give the original back",
			wantProps: []string{"roundtrip"},
			wantCount: 1,
		},
		{
			name:      "encode decode pattern",
			response:  "encode and decode should preserve the data",
			wantProps: []string{"roundtrip"},
			wantCount: 1,
		},
		{
			name:      "no properties",
			response:  "just a simple function with no special properties",
			wantProps: []string{},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dod := ParseDefinitionOfDone(tt.response)

			if len(dod.Properties) != tt.wantCount {
				t.Errorf("Properties count: got %d, want %d", len(dod.Properties), tt.wantCount)
			}

			for _, wantProp := range tt.wantProps {
				found := false
				for _, prop := range dod.Properties {
					if prop.Name == wantProp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected property %q not found", wantProp)
				}
			}
		})
	}
}

func TestDoDHasTestableRequirementsWithProperties(t *testing.T) {
	dod := &DefinitionOfDone{
		Properties: []PropertyTest{
			{Name: "roundtrip", Description: "Roundtrip property"},
		},
	}

	if !dod.HasTestableRequirements() {
		t.Error("DoD with properties should have testable requirements")
	}
}

func TestDoDFormatSummaryWithProperties(t *testing.T) {
	dod := &DefinitionOfDone{
		Properties: []PropertyTest{
			{Name: "roundtrip", Description: "Roundtrip property"},
			{Name: "idempotent", Description: "Idempotent property"},
		},
	}

	summary := dod.FormatDoDSummary()

	if !strings.Contains(summary, "properties:") {
		t.Error("Summary should mention properties")
	}
	if !strings.Contains(summary, "roundtrip") {
		t.Error("Summary should mention roundtrip property")
	}
	if !strings.Contains(summary, "idempotent") {
		t.Error("Summary should mention idempotent property")
	}
}
