package main

import (
	"errors"
	"fmt"
	"strings"
)

// UserError represents an error that should be displayed to the user with helpful context
type UserError struct {
	Message    string
	Cause      error
	Suggestion string
}

func (e *UserError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *UserError) Unwrap() error {
	return e.Cause
}

// FormatUserError formats an error for user display with colors and suggestions
func FormatUserError(err error) string {
	var sb strings.Builder

	var userErr *UserError
	if errors.As(err, &userErr) {
		sb.WriteString(fmt.Sprintf("\033[91mError:\033[0m %s\n", userErr.Message))
		if userErr.Cause != nil {
			sb.WriteString(fmt.Sprintf("       Cause: %v\n", userErr.Cause))
		}
		if userErr.Suggestion != "" {
			sb.WriteString(fmt.Sprintf("\n\033[93mSuggestion:\033[0m %s\n", userErr.Suggestion))
		}
	} else {
		// Generic error - try to provide helpful context
		errStr := err.Error()
		sb.WriteString(fmt.Sprintf("\033[91mError:\033[0m %s\n", errStr))

		// Add suggestions based on common error patterns
		suggestion := getSuggestionForError(errStr)
		if suggestion != "" {
			sb.WriteString(fmt.Sprintf("\n\033[93mSuggestion:\033[0m %s\n", suggestion))
		}
	}

	return sb.String()
}

// getSuggestionForError returns a helpful suggestion based on error content
func getSuggestionForError(errStr string) string {
	errLower := strings.ToLower(errStr)

	// AWS/Bedrock related errors
	if strings.Contains(errLower, "no valid credential") ||
		strings.Contains(errLower, "unable to sign request") ||
		strings.Contains(errLower, "security token") {
		return "Check your AWS credentials. Run 'aws configure' or set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables."
	}

	if strings.Contains(errLower, "region") {
		return "Set the AWS_REGION environment variable (e.g., 'export AWS_REGION=us-east-1')."
	}

	if strings.Contains(errLower, "access denied") ||
		strings.Contains(errLower, "not authorized") {
		return "Your AWS credentials may not have permission to access Bedrock. Check IAM policies for bedrock:InvokeModel permission."
	}

	if strings.Contains(errLower, "model") && strings.Contains(errLower, "not found") {
		return "The specified model may not be available in your region. Try setting BJARNE_MODEL to a different model ID."
	}

	if strings.Contains(errLower, "throttl") {
		return "You're being rate-limited. Wait a moment and try again, or check your Bedrock service quotas."
	}

	// Container related errors
	if strings.Contains(errLower, "podman") || strings.Contains(errLower, "docker") {
		if strings.Contains(errLower, "not found") || strings.Contains(errLower, "no such") {
			return "Install podman or docker: https://podman.io/getting-started/installation or https://docs.docker.com/get-docker/"
		}
		if strings.Contains(errLower, "permission denied") {
			return "You may need to run with elevated permissions, or add your user to the docker group."
		}
		if strings.Contains(errLower, "daemon") || strings.Contains(errLower, "socket") {
			return "The container daemon may not be running. Start the docker/podman service."
		}
	}

	if strings.Contains(errLower, "timeout") {
		return "The operation timed out. This might be due to slow network or a complex operation. Try again or check your connection."
	}

	if strings.Contains(errLower, "connection refused") ||
		strings.Contains(errLower, "network") {
		return "Check your network connection. You may be offline or behind a firewall."
	}

	return ""
}

// Common error constructors

// ErrNoContainerRuntime creates an error for missing container runtime
func ErrNoContainerRuntime() *UserError {
	return &UserError{
		Message:    "No container runtime found",
		Suggestion: "Install podman (recommended) or docker:\n       - Podman: https://podman.io/getting-started/installation\n       - Docker: https://docs.docker.com/get-docker/",
	}
}

// ErrAWSConfig creates an error for AWS configuration issues
func ErrAWSConfig(cause error) *UserError {
	return &UserError{
		Message: "Failed to initialize AWS configuration",
		Cause:   cause,
		Suggestion: `Check your AWS credentials:
       1. Run 'aws configure' to set up credentials
       2. Or set environment variables:
          export AWS_ACCESS_KEY_ID=your_key
          export AWS_SECRET_ACCESS_KEY=your_secret
          export AWS_REGION=us-east-1`,
	}
}

// ErrBedrockInvoke creates an error for Bedrock API issues
func ErrBedrockInvoke(cause error) *UserError {
	return &UserError{
		Message: "Failed to call Bedrock API",
		Cause:   cause,
		Suggestion: `Possible issues:
       1. Check AWS credentials and region
       2. Verify Bedrock access is enabled in your AWS account
       3. Check IAM permissions for bedrock:InvokeModel
       4. Try a different model with BJARNE_MODEL env var`,
	}
}

// ErrContainerPull creates an error for container pull failures
func ErrContainerPull(image string, cause error) *UserError {
	return &UserError{
		Message: fmt.Sprintf("Failed to pull container image: %s", image),
		Cause:   cause,
		Suggestion: `Possible fixes:
       1. Check your internet connection
       2. Verify you can access ghcr.io (GitHub Container Registry)
       3. Try pulling manually: podman pull ` + image + `
       4. Or build locally: cd docker && podman build -t bjarne-validator .`,
	}
}

// ErrValidation creates an error for validation pipeline failures
func ErrValidation(stage string, cause error) *UserError {
	return &UserError{
		Message: fmt.Sprintf("Validation failed at stage: %s", stage),
		Cause:   cause,
	}
}
