package main

import (
	"errors"
	"strings"
	"testing"
)

func TestUserError(t *testing.T) {
	t.Run("error without cause", func(t *testing.T) {
		err := &UserError{Message: "test error"}
		if err.Error() != "test error" {
			t.Errorf("Error() = %q, want %q", err.Error(), "test error")
		}
	})

	t.Run("error with cause", func(t *testing.T) {
		cause := errors.New("underlying error")
		err := &UserError{Message: "test error", Cause: cause}
		expected := "test error: underlying error"
		if err.Error() != expected {
			t.Errorf("Error() = %q, want %q", err.Error(), expected)
		}
	})

	t.Run("unwrap returns cause", func(t *testing.T) {
		cause := errors.New("underlying error")
		err := &UserError{Message: "test error", Cause: cause}
		if err.Unwrap() != cause {
			t.Error("Unwrap() did not return the cause")
		}
	})
}

func TestFormatUserError(t *testing.T) {
	t.Run("formats UserError with suggestion", func(t *testing.T) {
		err := &UserError{
			Message:    "test error",
			Suggestion: "try this fix",
		}
		output := FormatUserError(err)
		if !strings.Contains(output, "test error") {
			t.Error("output should contain error message")
		}
		if !strings.Contains(output, "try this fix") {
			t.Error("output should contain suggestion")
		}
	})

	t.Run("formats generic error with auto-suggestion", func(t *testing.T) {
		err := errors.New("no valid credential sources")
		output := FormatUserError(err)
		if !strings.Contains(output, "no valid credential") {
			t.Error("output should contain error message")
		}
		if !strings.Contains(output, "aws configure") {
			t.Error("output should contain AWS credential suggestion")
		}
	})
}

func TestGetSuggestionForError(t *testing.T) {
	tests := []struct {
		name        string
		errStr      string
		shouldMatch string
	}{
		{
			name:        "AWS credentials error",
			errStr:      "no valid credential sources",
			shouldMatch: "aws configure",
		},
		{
			name:        "AWS region error",
			errStr:      "region not specified",
			shouldMatch: "AWS_REGION",
		},
		{
			name:        "access denied",
			errStr:      "Access Denied",
			shouldMatch: "IAM",
		},
		{
			name:        "throttling",
			errStr:      "throttled by service",
			shouldMatch: "rate-limited",
		},
		{
			name:        "podman not found",
			errStr:      "podman: command not found",
			shouldMatch: "podman.io",
		},
		{
			name:        "docker permission",
			errStr:      "docker: permission denied",
			shouldMatch: "docker group",
		},
		{
			name:        "timeout",
			errStr:      "context deadline exceeded (timeout)",
			shouldMatch: "timed out",
		},
		{
			name:        "network error",
			errStr:      "connection refused",
			shouldMatch: "network",
		},
		{
			name:        "unknown error",
			errStr:      "some random error",
			shouldMatch: "", // no suggestion
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestion := getSuggestionForError(tt.errStr)
			if tt.shouldMatch == "" {
				if suggestion != "" {
					t.Errorf("expected no suggestion, got %q", suggestion)
				}
			} else {
				if !strings.Contains(strings.ToLower(suggestion), strings.ToLower(tt.shouldMatch)) {
					t.Errorf("suggestion %q should contain %q", suggestion, tt.shouldMatch)
				}
			}
		})
	}
}

func TestErrorConstructors(t *testing.T) {
	t.Run("ErrNoContainerRuntime", func(t *testing.T) {
		err := ErrNoContainerRuntime()
		if !strings.Contains(err.Message, "container runtime") {
			t.Error("should mention container runtime")
		}
		if !strings.Contains(err.Suggestion, "podman") {
			t.Error("should suggest podman")
		}
	})

	t.Run("ErrAWSConfig", func(t *testing.T) {
		cause := errors.New("config error")
		err := ErrAWSConfig(cause)
		if err.Cause != cause {
			t.Error("should preserve cause")
		}
		if !strings.Contains(err.Suggestion, "aws configure") {
			t.Error("should suggest aws configure")
		}
	})

	t.Run("ErrContainerPull", func(t *testing.T) {
		cause := errors.New("network error")
		err := ErrContainerPull("ghcr.io/test:latest", cause)
		if !strings.Contains(err.Message, "ghcr.io/test:latest") {
			t.Error("should include image name")
		}
	})
}
