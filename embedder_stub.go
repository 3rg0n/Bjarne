//go:build !cgo || !onnx

package main

import (
	"fmt"
)

// Stub implementations when ONNX runtime is not available

// newONNXBackend returns an error when ONNX is not compiled in
func newONNXBackend(_ string) (EmbedderBackend, error) {
	return nil, fmt.Errorf("ONNX runtime not available (build without CGO or onnx tag)")
}

// isONNXAvailable returns false when ONNX is not compiled in
func isONNXAvailable() bool {
	return false
}
