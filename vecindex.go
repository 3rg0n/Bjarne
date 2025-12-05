package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	_ "github.com/mattn/go-sqlite3" // SQLite driver with CGO
)

// VectorIndex manages the semantic code index with embeddings
type VectorIndex struct {
	db        *sql.DB
	modelPath string
	embedder  *Embedder
}

// ChunkType identifies what kind of code chunk this is
type ChunkType string

const (
	ChunkFunction ChunkType = "function"
	ChunkClass    ChunkType = "class"
	ChunkStruct   ChunkType = "struct"
	ChunkHeader   ChunkType = "header"  // File-level header summary
	ChunkComment  ChunkType = "comment" // Documentation blocks
)

// CodeChunk represents a chunk of code for embedding
type CodeChunk struct {
	ID        int64
	FileID    int64
	Type      ChunkType
	Name      string // Function/class/struct name
	Content   string // The actual code
	StartLine int
	EndLine   int
	Embedding []float32 // 384-dim for BGE-small
}

// VectorIndexConfig holds configuration for the vector index
type VectorIndexConfig struct {
	DBPath       string // Path to SQLite database
	ModelDir     string // Directory for model files
	EmbeddingDim int    // Embedding dimension (384 for BGE-small)
}

// Model download configuration
const (
	BGESmallModelURL  = "https://huggingface.co/BAAI/bge-small-en-v1.5/resolve/main/onnx/model.onnx"
	BGESmallTokenizer = "https://huggingface.co/BAAI/bge-small-en-v1.5/resolve/main/tokenizer.json"
	EmbeddingDim      = 384 // BGE-small output dimension
	DefaultBatchSize  = 32
)

// DefaultVectorIndexConfig returns default configuration
func DefaultVectorIndexConfig() VectorIndexConfig {
	homeDir, _ := os.UserHomeDir()
	bjarneDir := filepath.Join(homeDir, ".bjarne")

	return VectorIndexConfig{
		DBPath:       filepath.Join(bjarneDir, "index.db"),
		ModelDir:     filepath.Join(bjarneDir, "models"),
		EmbeddingDim: EmbeddingDim,
	}
}

// NewVectorIndex creates or opens a vector index
func NewVectorIndex(cfg VectorIndexConfig) (*VectorIndex, error) {
	// Ensure directories exist
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0750); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}
	if err := os.MkdirAll(cfg.ModelDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create model directory: %w", err)
	}

	// Open SQLite database with sqlite-vec extension
	db, err := sql.Open("sqlite3", cfg.DBPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize schema
	if err := initVectorSchema(db, cfg.EmbeddingDim); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &VectorIndex{
		db:        db,
		modelPath: cfg.ModelDir,
	}, nil
}

// initVectorSchema creates the database schema
func initVectorSchema(db *sql.DB, _ int) error { // embeddingDim reserved for sqlite-vec
	schema := `
	-- Indexed files
	CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT UNIQUE NOT NULL,
		hash TEXT NOT NULL,
		mod_time INTEGER NOT NULL,
		indexed_at INTEGER NOT NULL
	);

	-- Code chunks (functions, classes, structs, etc.)
	CREATE TABLE IF NOT EXISTS chunks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_id INTEGER NOT NULL,
		type TEXT NOT NULL,
		name TEXT NOT NULL,
		content TEXT NOT NULL,
		start_line INTEGER NOT NULL,
		end_line INTEGER NOT NULL,
		FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
	);

	-- Create indexes for faster lookups
	CREATE INDEX IF NOT EXISTS idx_chunks_file ON chunks(file_id);
	CREATE INDEX IF NOT EXISTS idx_chunks_name ON chunks(name);
	CREATE INDEX IF NOT EXISTS idx_files_path ON files(path);

	-- Embeddings table (will use sqlite-vec virtual table when available)
	-- For now, store as blob and do brute-force search
	CREATE TABLE IF NOT EXISTS embeddings (
		chunk_id INTEGER PRIMARY KEY,
		vector BLOB NOT NULL,
		FOREIGN KEY (chunk_id) REFERENCES chunks(id) ON DELETE CASCADE
	);
	`

	_, err := db.Exec(schema)
	return err
}

// Close closes the vector index
func (vi *VectorIndex) Close() error {
	if vi.embedder != nil {
		_ = vi.embedder.Close()
	}
	return vi.db.Close()
}

// EnsureModel downloads the embedding model if not present
func (vi *VectorIndex) EnsureModel(ctx context.Context, progressFn func(string)) error {
	modelFile := filepath.Join(vi.modelPath, "bge-small-en-v1.5.onnx")
	tokenizerFile := filepath.Join(vi.modelPath, "tokenizer.json")

	// Check if model exists
	modelExists := false
	if _, err := os.Stat(modelFile); err == nil {
		if _, err := os.Stat(tokenizerFile); err == nil {
			if progressFn != nil {
				progressFn("Model already downloaded")
			}
			modelExists = true
		}
	}

	if !modelExists {
		if progressFn != nil {
			progressFn("Downloading BGE-small embedding model (~35MB)...")
		}

		// Download model
		if err := downloadFile(ctx, BGESmallModelURL, modelFile, progressFn); err != nil {
			return fmt.Errorf("failed to download model: %w", err)
		}

		// Download tokenizer
		if progressFn != nil {
			progressFn("Downloading tokenizer...")
		}
		if err := downloadFile(ctx, BGESmallTokenizer, tokenizerFile, progressFn); err != nil {
			return fmt.Errorf("failed to download tokenizer: %w", err)
		}

		if progressFn != nil {
			progressFn("Model ready!")
		}
	}

	// Initialize embedder
	if vi.embedder == nil {
		vi.embedder = NewEmbedder(modelFile, tokenizerFile)
		if IsONNXAvailable() {
			if progressFn != nil {
				progressFn("Initializing ONNX embedder...")
			}
		}
	}

	return nil
}

// downloadFile downloads a file from URL to destination with progress
func downloadFile(ctx context.Context, url, dest string, progressFn func(string)) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Create temp file
	tmpFile := dest + ".tmp"
	f, err := os.Create(tmpFile)
	if err != nil {
		return err
	}

	// Download with progress
	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				_ = f.Close()
				_ = os.Remove(tmpFile)
				return writeErr
			}
			downloaded += int64(n)
			if progressFn != nil && resp.ContentLength > 0 {
				pct := float64(downloaded) / float64(resp.ContentLength) * 100
				progressFn(fmt.Sprintf("  %.0f%% (%d/%d bytes)", pct, downloaded, resp.ContentLength))
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = f.Close()
			_ = os.Remove(tmpFile)
			return readErr
		}
	}

	_ = f.Close()

	// Rename to final destination
	return os.Rename(tmpFile, dest)
}

// IndexWorkspaceWithEmbeddings indexes a workspace and generates embeddings
func (vi *VectorIndex) IndexWorkspaceWithEmbeddings(ctx context.Context, rootPath string, progressFn func(string)) error {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// First pass: scan files and extract chunks
	if progressFn != nil {
		progressFn("Scanning source files...")
	}

	var allChunks []CodeChunk
	var fileCount int

	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr // Skip inaccessible files intentionally
		}

		// Skip directories
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || skipDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if it's a C/C++ source file
		ext := strings.ToLower(filepath.Ext(path))
		if !sourceExtensions[ext] {
			return nil
		}

		relPath, _ := filepath.Rel(absRoot, path)
		fileCount++

		if progressFn != nil && fileCount%10 == 0 {
			progressFn(fmt.Sprintf("  Scanned %d files...", fileCount))
		}

		// Check if file needs re-indexing
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil //nolint:nilerr // Skip unreadable files intentionally
		}

		hash := sha256.Sum256(content)
		hashStr := hex.EncodeToString(hash[:16])

		info, infoErr := d.Info()
		if infoErr != nil {
			return nil //nolint:nilerr // Skip files we can't stat
		}

		// Check if file is already indexed with same hash
		var existingHash string
		queryErr := vi.db.QueryRowContext(ctx, "SELECT hash FROM files WHERE path = ?", relPath).Scan(&existingHash)
		if queryErr == nil && existingHash == hashStr {
			return nil // File unchanged, skip
		}

		// Delete old data for this file
		_, _ = vi.db.ExecContext(ctx, "DELETE FROM files WHERE path = ?", relPath)

		// Insert file record
		result, insertErr := vi.db.ExecContext(ctx,
			"INSERT INTO files (path, hash, mod_time, indexed_at) VALUES (?, ?, ?, ?)",
			relPath, hashStr, info.ModTime().Unix(), time.Now().Unix())
		if insertErr != nil {
			return nil //nolint:nilerr // Skip files that fail to insert
		}

		fileID, _ := result.LastInsertId()

		// Extract chunks from file
		chunks := extractChunks(string(content), fileID, relPath)
		allChunks = append(allChunks, chunks...)

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to scan directory: %w", err)
	}

	if progressFn != nil {
		progressFn(fmt.Sprintf("Found %d chunks in %d files", len(allChunks), fileCount))
	}

	if len(allChunks) == 0 {
		return nil
	}

	// Insert chunks into database
	if progressFn != nil {
		progressFn("Storing chunks...")
	}

	tx, err := vi.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO chunks (file_id, type, name, content, start_line, end_line) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for i := range allChunks {
		result, err := stmt.ExecContext(ctx, allChunks[i].FileID, allChunks[i].Type, allChunks[i].Name, allChunks[i].Content, allChunks[i].StartLine, allChunks[i].EndLine)
		if err != nil {
			continue
		}
		allChunks[i].ID, _ = result.LastInsertId()
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Generate embeddings if model is available
	if vi.embedder != nil {
		if err := vi.generateEmbeddings(ctx, allChunks, progressFn); err != nil {
			if progressFn != nil {
				progressFn(fmt.Sprintf("Warning: embedding generation failed: %v", err))
			}
		}
	} else if progressFn != nil {
		progressFn("Skipping embeddings (model not loaded)")
	}

	return nil
}

// extractChunks extracts code chunks from file content
func extractChunks(content string, fileID int64, filePath string) []CodeChunk {
	var chunks []CodeChunk
	lines := strings.Split(content, "\n")

	// For now, use simple function/class extraction
	// This can be enhanced with proper AST parsing later

	// Extract using existing regex patterns from index.go
	funcMatches := funcPattern.FindAllStringSubmatchIndex(content, -1)
	for _, match := range funcMatches {
		if len(match) >= 8 {
			funcName := strings.TrimSpace(content[match[4]:match[5]])
			if isKeyword(funcName) {
				continue
			}

			startLine := strings.Count(content[:match[0]], "\n") + 1
			// Find end of function (simple heuristic: next function or EOF)
			endLine := startLine + 20 // Default chunk size
			if endLine > len(lines) {
				endLine = len(lines)
			}

			// Extract function content with context
			chunkContent := strings.Join(lines[startLine-1:endLine], "\n")

			chunks = append(chunks, CodeChunk{
				FileID:    fileID,
				Type:      ChunkFunction,
				Name:      funcName,
				Content:   chunkContent,
				StartLine: startLine,
				EndLine:   endLine,
			})
		}
	}

	// Extract classes
	classMatches := classPattern.FindAllStringSubmatchIndex(content, -1)
	for _, match := range classMatches {
		if len(match) >= 4 {
			className := content[match[2]:match[3]]
			startLine := strings.Count(content[:match[0]], "\n") + 1
			endLine := startLine + 50 // Class chunks are larger

			if endLine > len(lines) {
				endLine = len(lines)
			}

			chunkContent := strings.Join(lines[startLine-1:endLine], "\n")

			chunks = append(chunks, CodeChunk{
				FileID:    fileID,
				Type:      ChunkClass,
				Name:      className,
				Content:   chunkContent,
				StartLine: startLine,
				EndLine:   endLine,
			})
		}
	}

	// Extract structs
	structMatches := structPattern.FindAllStringSubmatchIndex(content, -1)
	for _, match := range structMatches {
		if len(match) >= 4 {
			structName := content[match[2]:match[3]]
			startLine := strings.Count(content[:match[0]], "\n") + 1
			endLine := startLine + 30

			if endLine > len(lines) {
				endLine = len(lines)
			}

			chunkContent := strings.Join(lines[startLine-1:endLine], "\n")

			chunks = append(chunks, CodeChunk{
				FileID:    fileID,
				Type:      ChunkStruct,
				Name:      structName,
				Content:   chunkContent,
				StartLine: startLine,
				EndLine:   endLine,
			})
		}
	}

	// Add file-level header chunk for context
	headerEnd := 50
	if headerEnd > len(lines) {
		headerEnd = len(lines)
	}
	if headerEnd > 0 {
		headerContent := strings.Join(lines[:headerEnd], "\n")
		chunks = append(chunks, CodeChunk{
			FileID:    fileID,
			Type:      ChunkHeader,
			Name:      filepath.Base(filePath),
			Content:   headerContent,
			StartLine: 1,
			EndLine:   headerEnd,
		})
	}

	return chunks
}

// generateEmbeddings generates embeddings for chunks in batches
func (vi *VectorIndex) generateEmbeddings(ctx context.Context, chunks []CodeChunk, progressFn func(string)) error {
	if vi.embedder == nil {
		return fmt.Errorf("embedder not initialized")
	}

	if progressFn != nil {
		progressFn(fmt.Sprintf("Generating embeddings for %d chunks...", len(chunks)))
	}

	// Process in batches
	batchSize := DefaultBatchSize
	_ = runtime.NumCPU() // Reserved for future parallel processing

	tx, err := vi.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		"INSERT OR REPLACE INTO embeddings (chunk_id, vector) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for i := 0; i < len(chunks); i += batchSize {
		end := i + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}

		batch := chunks[i:end]
		texts := make([]string, len(batch))
		for j, chunk := range batch {
			texts[j] = chunk.Content
		}

		embeddings, err := vi.embedder.EmbedBatch(ctx, texts)
		if err != nil {
			return fmt.Errorf("embedding batch failed: %w", err)
		}

		for j, emb := range embeddings {
			chunk := batch[j]
			// Store embedding as blob
			blob := float32sToBytes(emb)
			_, err := stmt.ExecContext(ctx, chunk.ID, blob)
			if err != nil {
				return err
			}
		}

		if progressFn != nil {
			progressFn(fmt.Sprintf("  Embedded %d/%d chunks", end, len(chunks)))
		}
	}

	return tx.Commit()
}

// SearchSimilar finds chunks similar to the query
func (vi *VectorIndex) SearchSimilar(ctx context.Context, query string, topK int) ([]CodeChunk, error) {
	if vi.embedder == nil {
		return nil, fmt.Errorf("embedder not initialized")
	}

	// Embed query
	queryEmb, err := vi.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	// Brute force search (replace with sqlite-vec when available)
	rows, err := vi.db.QueryContext(ctx, `
		SELECT c.id, c.file_id, c.type, c.name, c.content, c.start_line, c.end_line, e.vector
		FROM chunks c
		JOIN embeddings e ON c.id = e.chunk_id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	type scoredChunk struct {
		chunk CodeChunk
		score float32
	}
	var scored []scoredChunk

	for rows.Next() {
		var chunk CodeChunk
		var vectorBlob []byte
		err := rows.Scan(&chunk.ID, &chunk.FileID, &chunk.Type, &chunk.Name, &chunk.Content,
			&chunk.StartLine, &chunk.EndLine, &vectorBlob)
		if err != nil {
			continue
		}

		chunkEmb := bytesToFloat32s(vectorBlob)
		score := cosineSimilarity(queryEmb, chunkEmb)
		scored = append(scored, scoredChunk{chunk, score})
	}

	// Sort by score descending
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	// Return top K
	result := make([]CodeChunk, 0, topK)
	for i := 0; i < len(scored) && i < topK; i++ {
		result = append(result, scored[i].chunk)
	}

	return result, nil
}

// GetStats returns statistics about the index
func (vi *VectorIndex) GetStats(ctx context.Context) (files, chunks, embeddings int, err error) {
	err = vi.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM files").Scan(&files)
	if err != nil {
		return
	}
	err = vi.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks").Scan(&chunks)
	if err != nil {
		return
	}
	err = vi.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM embeddings").Scan(&embeddings)
	return
}

// Helper functions for vector operations

func float32sToBytes(floats []float32) []byte {
	buf := make([]byte, len(floats)*4)
	for i, f := range floats {
		bits := *(*uint32)(unsafe.Pointer(&f))
		buf[i*4] = byte(bits)
		buf[i*4+1] = byte(bits >> 8)
		buf[i*4+2] = byte(bits >> 16)
		buf[i*4+3] = byte(bits >> 24)
	}
	return buf
}

func bytesToFloat32s(buf []byte) []float32 {
	floats := make([]float32, len(buf)/4)
	for i := range floats {
		bits := uint32(buf[i*4]) | uint32(buf[i*4+1])<<8 | uint32(buf[i*4+2])<<16 | uint32(buf[i*4+3])<<24
		floats[i] = *(*float32)(unsafe.Pointer(&bits))
	}
	return floats
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (sqrt32(normA) * sqrt32(normB))
}

func sqrt32(x float32) float32 {
	return float32(math.Sqrt(float64(x)))
}
