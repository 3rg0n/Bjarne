//go:build cgo && onnx

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// ONNXBackend wraps the ONNX runtime for embedding generation
type ONNXBackend struct {
	session *ort.DynamicAdvancedSession
	mu      sync.Mutex
}

var onnxInitOnce sync.Once
var onnxInitErr error

// newONNXBackend creates an ONNX backend for embeddings
func newONNXBackend(modelPath string) (EmbedderBackend, error) {
	// Check if model exists
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("model file not found: %s", modelPath)
	}

	// Initialize ONNX runtime (once per process)
	onnxInitOnce.Do(func() {
		// Find and set the library path
		if !findONNXLibrary() {
			onnxInitErr = fmt.Errorf("ONNX runtime library not found")
			return
		}
		onnxInitErr = ort.InitializeEnvironment()
	})

	if onnxInitErr != nil {
		return nil, onnxInitErr
	}

	// Create session options
	options, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("failed to create session options: %w", err)
	}
	defer func() { _ = options.Destroy() }()

	// Set thread count
	numThreads := runtime.NumCPU()
	if numThreads > 4 {
		numThreads = 4
	}
	if err := options.SetIntraOpNumThreads(numThreads); err != nil {
		return nil, fmt.Errorf("failed to set thread count: %w", err)
	}

	// BGE-small inputs and outputs
	inputNames := []string{"input_ids", "attention_mask", "token_type_ids"}
	outputNames := []string{"sentence_embedding"}

	// Create session
	session, err := ort.NewDynamicAdvancedSession(
		modelPath,
		inputNames,
		outputNames,
		options,
	)
	if err != nil {
		// Try without token_type_ids
		inputNames = []string{"input_ids", "attention_mask"}
		session, err = ort.NewDynamicAdvancedSession(
			modelPath,
			inputNames,
			outputNames,
			options,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create ONNX session: %w", err)
		}
	}

	return &ONNXBackend{session: session}, nil
}

// EmbedBatch runs inference on tokenized inputs
func (b *ONNXBackend) EmbedBatch(ctx context.Context, inputIDs, attentionMask []int64, batchSize, seqLen, dim int) ([][]float32, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	shape := ort.Shape{int64(batchSize), int64(seqLen)}

	// Create input tensors
	inputIDsTensor, err := ort.NewTensor(shape, inputIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to create input_ids tensor: %w", err)
	}
	defer func() { _ = inputIDsTensor.Destroy() }()

	attentionTensor, err := ort.NewTensor(shape, attentionMask)
	if err != nil {
		return nil, fmt.Errorf("failed to create attention_mask tensor: %w", err)
	}
	defer func() { _ = attentionTensor.Destroy() }()

	// Token type IDs (all zeros)
	tokenTypeIDs := make([]int64, batchSize*seqLen)
	tokenTypeTensor, err := ort.NewTensor(shape, tokenTypeIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to create token_type_ids tensor: %w", err)
	}
	defer func() { _ = tokenTypeTensor.Destroy() }()

	// Create output tensor
	outputShape := ort.Shape{int64(batchSize), int64(dim)}
	outputData := make([]float32, batchSize*dim)
	outputTensor, err := ort.NewTensor(outputShape, outputData)
	if err != nil {
		return nil, fmt.Errorf("failed to create output tensor: %w", err)
	}
	defer func() { _ = outputTensor.Destroy() }()

	// Run inference
	err = b.session.Run(
		[]ort.Value{inputIDsTensor, attentionTensor, tokenTypeTensor},
		[]ort.Value{outputTensor},
	)
	if err != nil {
		// Try without token_type_ids
		err = b.session.Run(
			[]ort.Value{inputIDsTensor, attentionTensor},
			[]ort.Value{outputTensor},
		)
		if err != nil {
			return nil, fmt.Errorf("ONNX inference failed: %w", err)
		}
	}

	// Extract and normalize embeddings
	result := make([][]float32, batchSize)
	for i := 0; i < batchSize; i++ {
		embedding := make([]float32, dim)
		copy(embedding, outputData[i*dim:(i+1)*dim])
		result[i] = normalizeL2(embedding)
	}

	return result, nil
}

// Close releases ONNX resources
func (b *ONNXBackend) Close() error {
	if b.session != nil {
		_ = b.session.Destroy()
		b.session = nil
	}
	return nil
}

// isONNXAvailable checks if ONNX runtime is available
func isONNXAvailable() bool {
	return findONNXLibrary()
}

// findONNXLibrary searches for and configures the ONNX runtime library
func findONNXLibrary() bool {
	libName := getONNXLibraryName()
	searchPaths := getONNXSearchPaths()

	for _, dir := range searchPaths {
		libPath := filepath.Join(dir, libName)
		if _, err := os.Stat(libPath); err == nil {
			ort.SetSharedLibraryPath(libPath)
			return true
		}
	}
	return false
}

// getONNXLibraryName returns the platform-specific library name
func getONNXLibraryName() string {
	switch runtime.GOOS {
	case "windows":
		return "onnxruntime.dll"
	case "darwin":
		return "libonnxruntime.dylib"
	default:
		return "libonnxruntime.so"
	}
}

// getONNXSearchPaths returns paths to search for ONNX runtime library
func getONNXSearchPaths() []string {
	var paths []string

	// User's bjarne directory
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(home, ".bjarne", "lib"),
			filepath.Join(home, ".bjarne", "onnxruntime"),
		)
	}

	// Current directory
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, cwd, filepath.Join(cwd, "lib"))
	}

	// System paths
	switch runtime.GOOS {
	case "windows":
		paths = append(paths,
			`C:\Program Files\onnxruntime\lib`,
			`C:\onnxruntime\lib`,
		)
		if pathEnv := os.Getenv("PATH"); pathEnv != "" {
			paths = append(paths, filepath.SplitList(pathEnv)...)
		}
	case "darwin":
		paths = append(paths,
			"/opt/homebrew/lib",
			"/opt/homebrew/opt/onnxruntime/lib",
			"/usr/local/lib",
			"/usr/local/opt/onnxruntime/lib",
		)
	default:
		paths = append(paths,
			"/usr/lib",
			"/usr/lib/x86_64-linux-gnu",
			"/usr/local/lib",
			"/opt/onnxruntime/lib",
		)
		if ldPath := os.Getenv("LD_LIBRARY_PATH"); ldPath != "" {
			paths = append(paths, filepath.SplitList(ldPath)...)
		}
	}

	return paths
}
