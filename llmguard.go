package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// LLMGuardClient handles communication with llm-guard API for prompt scanning
// See: https://github.com/protectai/llm-guard
type LLMGuardClient struct {
	baseURL    string
	httpClient *http.Client
	enabled    bool
}

// LLMGuardScanRequest is the request format for /scan/prompt and /scan/output
type LLMGuardScanRequest struct {
	Prompt string `json:"prompt,omitempty"` // For input scanning
	Output string `json:"output,omitempty"` // For output scanning
}

// LLMGuardScanResponse is the response from scanning endpoints
type LLMGuardScanResponse struct {
	IsValid         bool              `json:"is_valid"`
	SanitizedPrompt string            `json:"sanitized_prompt,omitempty"`
	SanitizedOutput string            `json:"sanitized_output,omitempty"`
	Results         map[string]Result `json:"results"`
}

// Result represents a single scanner result
type Result struct {
	Score   float64 `json:"score"`
	IsValid bool    `json:"is_valid"`
	Risk    string  `json:"risk,omitempty"`
}

// NewLLMGuardClient creates a new llm-guard client
// Reads LLMGUARD_URL from environment (default: disabled)
func NewLLMGuardClient() *LLMGuardClient {
	url := os.Getenv("LLMGUARD_URL")
	if url == "" {
		return &LLMGuardClient{enabled: false}
	}

	return &LLMGuardClient{
		baseURL: url,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		enabled: true,
	}
}

// IsEnabled returns whether llm-guard is configured
func (c *LLMGuardClient) IsEnabled() bool {
	return c.enabled
}

// ScanPrompt scans user input for prompt injection, secrets, and toxicity
// Returns sanitized prompt and any security issues found
func (c *LLMGuardClient) ScanPrompt(prompt string) (*LLMGuardScanResponse, error) {
	if !c.enabled {
		return &LLMGuardScanResponse{IsValid: true, SanitizedPrompt: prompt}, nil
	}

	req := LLMGuardScanRequest{Prompt: prompt}
	return c.doScan("/scan/prompt", req)
}

// ScanOutput scans LLM-generated code for embedded secrets and security issues
func (c *LLMGuardClient) ScanOutput(output string) (*LLMGuardScanResponse, error) {
	if !c.enabled {
		return &LLMGuardScanResponse{IsValid: true, SanitizedOutput: output}, nil
	}

	req := LLMGuardScanRequest{Output: output}
	return c.doScan("/scan/output", req)
}

// doScan performs the actual HTTP request to llm-guard API
func (c *LLMGuardClient) doScan(endpoint string, req LLMGuardScanRequest) (*LLMGuardScanResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Optional auth token
	if token := os.Getenv("LLMGUARD_TOKEN"); token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llm-guard request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm-guard returned status %d", resp.StatusCode)
	}

	var result LLMGuardScanResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// FormatSecurityIssues formats scan results as human-readable warnings
func (c *LLMGuardClient) FormatSecurityIssues(resp *LLMGuardScanResponse) string {
	if resp.IsValid {
		return ""
	}

	var issues []string
	for scanner, result := range resp.Results {
		if !result.IsValid {
			issue := fmt.Sprintf("- %s: score=%.2f", scanner, result.Score)
			if result.Risk != "" {
				issue += fmt.Sprintf(" (%s)", result.Risk)
			}
			issues = append(issues, issue)
		}
	}

	if len(issues) == 0 {
		return ""
	}

	return "Security scan detected issues:\n" + joinLines(issues)
}

// joinLines joins strings with newlines
func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}
