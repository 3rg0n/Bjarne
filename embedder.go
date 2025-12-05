package main

import (
	"context"
	"fmt"
	"math"
)

// Embedder generates text embeddings
// When ONNX runtime is available (via embedder_onnx.go), it uses real embeddings
// Otherwise, it falls back to pseudo-embeddings for testing
type Embedder struct {
	modelPath     string
	tokenizerPath string
	dimension     int
	maxLength     int
	tokenizer     *BertTokenizer
	backend       EmbedderBackend
}

// EmbedderBackend is the interface for embedding backends
type EmbedderBackend interface {
	EmbedBatch(ctx context.Context, inputIDs, attentionMask []int64, batchSize, seqLen, dim int) ([][]float32, error)
	Close() error
}

// NewEmbedder creates a new embedder with the given model
func NewEmbedder(modelPath, tokenizerPath string) *Embedder {
	return &Embedder{
		modelPath:     modelPath,
		tokenizerPath: tokenizerPath,
		dimension:     EmbeddingDim,
		maxLength:     512, // BGE-small default
	}
}

// Close releases resources
func (e *Embedder) Close() error {
	if e.backend != nil {
		return e.backend.Close()
	}
	return nil
}

// Embed generates an embedding for a single text
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding generated")
	}
	return embeddings[0], nil
}

// EmbedBatch generates embeddings for multiple texts
func (e *Embedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	// Check context
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Try to initialize backend if not done
	if e.backend == nil {
		e.initBackend()
	}

	// If we have a tokenizer, use it with the backend
	if e.tokenizer != nil && e.backend != nil {
		return e.embedWithBackend(ctx, texts)
	}

	// Fallback to pseudo-embeddings
	return e.pseudoEmbedBatch(texts), nil
}

// initBackend initializes the embedding backend
func (e *Embedder) initBackend() {
	// Try to load tokenizer first
	if e.tokenizerPath != "" {
		tokenizer, err := NewBertTokenizer(e.tokenizerPath, e.maxLength)
		if err == nil {
			e.tokenizer = tokenizer
		}
	}

	// Try to initialize ONNX backend (implemented in embedder_onnx.go if available)
	backend, err := newONNXBackend(e.modelPath)
	if err == nil {
		e.backend = backend
	}
}

// embedWithBackend uses the tokenizer and backend for embeddings
func (e *Embedder) embedWithBackend(ctx context.Context, texts []string) ([][]float32, error) {
	batchSize := len(texts)
	seqLen := e.maxLength

	// Tokenize all texts
	inputIDs := make([]int64, batchSize*seqLen)
	attentionMask := make([]int64, batchSize*seqLen)

	for i, text := range texts {
		ids, mask := e.tokenizer.Encode(text)
		copy(inputIDs[i*seqLen:(i+1)*seqLen], ids)
		copy(attentionMask[i*seqLen:(i+1)*seqLen], mask)
	}

	return e.backend.EmbedBatch(ctx, inputIDs, attentionMask, batchSize, seqLen, e.dimension)
}

// pseudoEmbedBatch generates deterministic pseudo-embeddings for fallback
func (e *Embedder) pseudoEmbedBatch(texts []string) [][]float32 {
	result := make([][]float32, len(texts))
	for i, text := range texts {
		result[i] = generatePseudoEmbedding(text, e.dimension)
	}
	return result
}

// normalizeL2 normalizes a vector to unit length
func normalizeL2(v []float32) []float32 {
	var norm float32
	for _, x := range v {
		norm += x * x
	}
	norm = float32(math.Sqrt(float64(norm)))

	if norm > 0 {
		for i := range v {
			v[i] /= norm
		}
	}
	return v
}

// generatePseudoEmbedding creates a deterministic fake embedding for testing
// This will be used when ONNX runtime is not available
func generatePseudoEmbedding(text string, dim int) []float32 {
	embedding := make([]float32, dim)

	// Simple hash-based pseudo-embedding for testing
	// NOT suitable for actual semantic search - just for infrastructure testing
	hash := uint64(0)
	for i, c := range text {
		hash = hash*31 + uint64(c) + uint64(i&0x7FFFFFFF) //nolint:gosec // overflow is intentional for hash
	}

	for i := 0; i < dim; i++ {
		// Generate pseudo-random values between -1 and 1
		hash = hash*1103515245 + 12345
		embedding[i] = float32(hash%1000)/500.0 - 1.0
	}

	// Normalize
	return normalizeL2(embedding)
}

// IsONNXAvailable checks if ONNX runtime is available on this system
func IsONNXAvailable() bool {
	return isONNXAvailable()
}
