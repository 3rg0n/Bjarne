# bjarne Vector Indexing Implementation Context

## Session Summary (2025-12-05)

This document captures the state of the vector indexing implementation for bjarne, allowing continuation in a new Claude session.

## What Was Implemented

### New Files Created:

1. **`tokenizer.go`** (~200 lines)
   - Pure Go WordPiece tokenizer for BERT-style models
   - Parses HuggingFace `tokenizer.json` format
   - Implements `BertTokenizer` with `Encode()` method

2. **`embedder.go`** (~165 lines)
   - Main `Embedder` struct with lazy initialization
   - `EmbedderBackend` interface for pluggable backends
   - Falls back to pseudo-embeddings when ONNX unavailable
   - `generatePseudoEmbedding()` for hash-based fallback

3. **`embedder_stub.go`** (~20 lines)
   - Build tag: `//go:build !cgo || !onnx`
   - Stub implementation returning error for ONNX backend
   - Used in default builds without CGO

4. **`embedder_onnx.go`** (~220 lines)
   - Build tag: `//go:build cgo && onnx`
   - Full ONNX runtime implementation with `yalue/onnxruntime_go`
   - `ONNXBackend` struct with batch inference
   - Library path detection for Windows/macOS/Linux

5. **`vecindex.go`** (~680 lines)
   - `VectorIndex` struct with SQLite storage
   - SQLite schema: `files`, `chunks`, `embeddings` tables
   - `EnsureModel()` - downloads BGE-small from HuggingFace
   - `IndexWorkspaceWithEmbeddings()` - scans and embeds code
   - `extractChunks()` - smart chunking (functions, classes, structs)
   - `SearchSimilar()` - cosine similarity search
   - Helper functions: `float32sToBytes`, `bytesToFloat32s`, `cosineSimilarity`

### Modified Files:

1. **`tui.go`**:
   - Added `vectorIndex *VectorIndex` field to Model struct (line 117)
   - Updated `/init` command (lines 1072-1164):
     - Creates structural index first
     - Downloads BGE-small model if needed
     - Generates embeddings for all code chunks
     - Shows stats (files, chunks, embeddings)
   - Updated `buildSystemPrompt()` (lines 780-832):
     - Uses semantic search with vector index
     - Falls back to structural workspace index
     - Includes top 5 relevant chunks in prompt
   - Updated `/clear` command to close vector index

2. **`go.mod`**:
   - Added `github.com/yalue/onnxruntime_go v1.24.0`
   - Already had `github.com/mattn/go-sqlite3 v1.14.32`

## Current Build Status

- **Default build (no CGO)**: Works with pseudo-embeddings
  ```
  go build -o bjarne.exe .
  ```

- **ONNX build (requires CGO + MinGW)**: Not yet tested
  ```
  CGO_ENABLED=1 go build -tags "cgo,onnx" -o bjarne.exe .
  ```

## Blocking Issue

CGO requires a C compiler. On Windows:
- MSVC (Visual Studio) doesn't work well with Go's CGO
- Need MinGW-w64 installed

**To install MinGW:**
```powershell
# Option 1: winget
winget install -e --id MingW-W64.MingW-W64

# Option 2: Download from GitHub
# https://github.com/niXman/mingw-builds-binaries/releases
# Extract to C:\mingw64 and add C:\mingw64\bin to PATH
```

**ONNX runtime is already installed:**
- Location: `C:\Windows\System32\onnxruntime.dll`

## Next Steps

1. Install MinGW-w64
2. Verify with `gcc --version`
3. Build with ONNX support:
   ```
   CGO_ENABLED=1 go build -tags "cgo,onnx" -o bjarne.exe .
   ```
4. Test `/init` command to verify real embeddings work

## Key Constants (vecindex.go)

```go
const (
    BGESmallModelURL  = "https://huggingface.co/BAAI/bge-small-en-v1.5/resolve/main/onnx/model.onnx"
    BGESmallTokenizer = "https://huggingface.co/BAAI/bge-small-en-v1.5/resolve/main/tokenizer.json"
    EmbeddingDim      = 384 // BGE-small output dimension
    DefaultBatchSize  = 32
)
```

## Data Storage

- SQLite database: `~/.bjarne/index.db`
- Model files: `~/.bjarne/models/bge-small-en-v1.5.onnx`, `tokenizer.json`

## Architecture

```
User runs /init
    │
    ├── IndexWorkspace() → structural index (JSON)
    │
    └── NewVectorIndex()
        ├── EnsureModel() → downloads from HuggingFace
        ├── IndexWorkspaceWithEmbeddings()
        │   ├── Walk directory
        │   ├── extractChunks() → functions, classes, structs
        │   └── generateEmbeddings() → batched ONNX inference
        └── Store in SQLite

User sends prompt
    │
    └── buildSystemPrompt()
        ├── SearchSimilar(query, 5) → top 5 relevant chunks
        └── Include in <relevant_code_context> XML tag
```

## Tests

All tests pass:
```
go test -v -short ./...
```

Linter passes:
```
golangci-lint run ./...
```

## Files to Review

For the next session, review these files to understand the implementation:
- `embedder.go` - main interface
- `embedder_stub.go` - fallback when no ONNX
- `embedder_onnx.go` - real ONNX implementation
- `tokenizer.go` - WordPiece tokenizer
- `vecindex.go` - vector index with SQLite
- `tui.go` lines 780-832 (buildSystemPrompt) and 1072-1164 (/init command)
