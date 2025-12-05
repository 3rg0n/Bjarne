package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// WorkspaceIndex holds the indexed codebase information
type WorkspaceIndex struct {
	Version   string                `json:"version"`
	RootPath  string                `json:"root_path"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
	Files     map[string]*FileIndex `json:"files"`
	Summary   IndexSummary          `json:"summary"`
}

// IndexSummary provides quick stats about the indexed codebase
type IndexSummary struct {
	TotalFiles     int `json:"total_files"`
	TotalFunctions int `json:"total_functions"`
	TotalClasses   int `json:"total_classes"`
	TotalStructs   int `json:"total_structs"`
	TotalLines     int `json:"total_lines"`
}

// FileIndex holds parsed information about a single source file
type FileIndex struct {
	Path      string       `json:"path"`
	Hash      string       `json:"hash"`
	ModTime   time.Time    `json:"mod_time"`
	Lines     int          `json:"lines"`
	Includes  []string     `json:"includes"`
	Functions []FuncInfo   `json:"functions"`
	Classes   []ClassInfo  `json:"classes"`
	Structs   []StructInfo `json:"structs"`
}

// FuncInfo holds information about a function
type FuncInfo struct {
	Name       string `json:"name"`
	Signature  string `json:"signature"`
	Line       int    `json:"line"`
	ReturnType string `json:"return_type,omitempty"`
	IsMethod   bool   `json:"is_method,omitempty"`
	ClassName  string `json:"class_name,omitempty"`
}

// ClassInfo holds information about a class
type ClassInfo struct {
	Name    string   `json:"name"`
	Line    int      `json:"line"`
	Methods []string `json:"methods,omitempty"`
	Members []string `json:"members,omitempty"`
}

// StructInfo holds information about a struct
type StructInfo struct {
	Name    string   `json:"name"`
	Line    int      `json:"line"`
	Members []string `json:"members,omitempty"`
}

const (
	IndexFileName = "bjarne.index.json"
	IndexVersion  = "1.0"
)

// C/C++ file extensions to index
var sourceExtensions = map[string]bool{
	".c":   true,
	".cpp": true,
	".cc":  true,
	".cxx": true,
	".h":   true,
	".hpp": true,
	".hxx": true,
}

// Directories to skip during indexing
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"build":        true,
	"cmake-build":  true,
	"out":          true,
	"bin":          true,
	"obj":          true,
	".vscode":      true,
	".idea":        true,
	"third_party":  true,
	"vendor":       true,
	"external":     true,
}

// Regex patterns for parsing C/C++
var (
	// Match #include statements
	includePattern = regexp.MustCompile(`#include\s*[<"]([^>"]+)[>"]`)

	// Match function declarations/definitions
	// Captures: return_type, function_name, parameters
	funcPattern = regexp.MustCompile(`(?m)^[\t ]*(?:(?:static|inline|virtual|explicit|constexpr|extern)\s+)*` +
		`([\w:*&<>,\s]+?)\s+` + // return type
		`(\w+)\s*` + // function name
		`\(([^)]*)\)\s*` + // parameters
		`(?:const\s*)?(?:noexcept\s*)?(?:override\s*)?(?:final\s*)?` +
		`(?:->[\w:*&<>,\s]+\s*)?` + // trailing return type
		`(?:\{|;)`) // body start or declaration end

	// Match class declarations
	classPattern = regexp.MustCompile(`(?m)^[\t ]*(?:template\s*<[^>]*>\s*)?class\s+(\w+)(?:\s*:\s*[^{]+)?\s*\{`)

	// Match struct declarations
	structPattern = regexp.MustCompile(`(?m)^[\t ]*(?:template\s*<[^>]*>\s*)?struct\s+(\w+)(?:\s*:\s*[^{]+)?\s*\{`)
)

// IndexWorkspace scans and indexes the current directory
func IndexWorkspace(rootPath string, progressFn func(string)) (*WorkspaceIndex, error) {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	index := &WorkspaceIndex{
		Version:   IndexVersion,
		RootPath:  absRoot,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Files:     make(map[string]*FileIndex),
	}

	// Walk the directory tree
	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Skip files we can't access - intentionally return nil to continue walking
			return nil //nolint:nilerr
		}

		// Skip hidden and excluded directories
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

		// Get relative path for cleaner output
		relPath, _ := filepath.Rel(absRoot, path)
		if progressFn != nil {
			progressFn(relPath)
		}

		// Parse the file
		fileIndex, parseErr := parseSourceFile(path)
		if parseErr != nil {
			// Skip files that fail to parse - intentionally continue walking
			return nil //nolint:nilerr
		}

		fileIndex.Path = relPath
		index.Files[relPath] = fileIndex

		// Update summary
		index.Summary.TotalFiles++
		index.Summary.TotalFunctions += len(fileIndex.Functions)
		index.Summary.TotalClasses += len(fileIndex.Classes)
		index.Summary.TotalStructs += len(fileIndex.Structs)
		index.Summary.TotalLines += fileIndex.Lines

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return index, nil
}

// parseSourceFile extracts information from a C/C++ source file
func parseSourceFile(path string) (*FileIndex, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	// Calculate file hash for change detection
	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:8]) // First 8 bytes is enough

	text := string(content)
	lines := strings.Count(text, "\n") + 1

	fileIndex := &FileIndex{
		Hash:    hashStr,
		ModTime: info.ModTime(),
		Lines:   lines,
	}

	// Extract includes
	includeMatches := includePattern.FindAllStringSubmatch(text, -1)
	for _, match := range includeMatches {
		fileIndex.Includes = append(fileIndex.Includes, match[1])
	}

	// Extract functions
	funcMatches := funcPattern.FindAllStringSubmatchIndex(text, -1)
	for _, match := range funcMatches {
		if len(match) >= 8 {
			returnType := strings.TrimSpace(text[match[2]:match[3]])
			funcName := strings.TrimSpace(text[match[4]:match[5]])
			params := strings.TrimSpace(text[match[6]:match[7]])

			// Skip common false positives
			if isKeyword(funcName) || isKeyword(returnType) {
				continue
			}

			line := strings.Count(text[:match[0]], "\n") + 1
			sig := fmt.Sprintf("%s %s(%s)", returnType, funcName, params)

			fileIndex.Functions = append(fileIndex.Functions, FuncInfo{
				Name:       funcName,
				Signature:  sig,
				Line:       line,
				ReturnType: returnType,
			})
		}
	}

	// Extract classes
	classMatches := classPattern.FindAllStringSubmatchIndex(text, -1)
	for _, match := range classMatches {
		if len(match) >= 4 {
			className := text[match[2]:match[3]]
			line := strings.Count(text[:match[0]], "\n") + 1
			fileIndex.Classes = append(fileIndex.Classes, ClassInfo{
				Name: className,
				Line: line,
			})
		}
	}

	// Extract structs
	structMatches := structPattern.FindAllStringSubmatchIndex(text, -1)
	for _, match := range structMatches {
		if len(match) >= 4 {
			structName := text[match[2]:match[3]]
			line := strings.Count(text[:match[0]], "\n") + 1
			fileIndex.Structs = append(fileIndex.Structs, StructInfo{
				Name: structName,
				Line: line,
			})
		}
	}

	return fileIndex, nil
}

// isKeyword checks if a string is a C++ keyword (to avoid false positive function matches)
func isKeyword(s string) bool {
	keywords := map[string]bool{
		"if": true, "else": true, "for": true, "while": true, "do": true,
		"switch": true, "case": true, "default": true, "break": true, "continue": true,
		"return": true, "goto": true, "try": true, "catch": true, "throw": true,
		"new": true, "delete": true, "sizeof": true, "typeid": true, "typeof": true,
		"alignof": true, "decltype": true, "noexcept": true, "static_assert": true,
		"namespace": true, "using": true, "typedef": true, "typename": true,
		"class": true, "struct": true, "union": true, "enum": true,
		"public": true, "private": true, "protected": true, "friend": true,
		"virtual": true, "override": true, "final": true, "explicit": true,
		"static": true, "extern": true, "mutable": true, "register": true,
		"volatile": true, "const": true, "constexpr": true, "inline": true,
		"template": true, "concept": true, "requires": true,
	}
	return keywords[s]
}

// SaveIndex writes the index to a JSON file
func SaveIndex(index *WorkspaceIndex, path string) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	indexPath := filepath.Join(path, IndexFileName)
	if err := os.WriteFile(indexPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	return nil
}

// LoadIndex reads an existing index file
func LoadIndex(path string) (*WorkspaceIndex, error) {
	indexPath := filepath.Join(path, IndexFileName)
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}

	var index WorkspaceIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to parse index: %w", err)
	}

	return &index, nil
}

// GetContextForPrompt generates context string from index for LLM prompts
// maxTokens limits output size (approximate, based on character count / 4)
func (idx *WorkspaceIndex) GetContextForPrompt(maxTokens int) string {
	if idx == nil || len(idx.Files) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Existing Codebase Context\n\n")

	// List classes and structs first (most important for integration)
	var classes, structs, functions []string

	for path, file := range idx.Files {
		for _, c := range file.Classes {
			classes = append(classes, fmt.Sprintf("- class %s (%s:%d)", c.Name, path, c.Line))
		}
		for _, s := range file.Structs {
			structs = append(structs, fmt.Sprintf("- struct %s (%s:%d)", s.Name, path, s.Line))
		}
		for _, f := range file.Functions {
			if !f.IsMethod {
				functions = append(functions, fmt.Sprintf("- %s (%s:%d)", f.Signature, path, f.Line))
			}
		}
	}

	if len(classes) > 0 {
		sb.WriteString("### Classes\n")
		for _, c := range classes {
			sb.WriteString(c + "\n")
		}
		sb.WriteString("\n")
	}

	if len(structs) > 0 {
		sb.WriteString("### Structs\n")
		for _, s := range structs {
			sb.WriteString(s + "\n")
		}
		sb.WriteString("\n")
	}

	if len(functions) > 0 {
		sb.WriteString("### Functions\n")
		// Limit based on maxTokens (approximate: 4 chars per token)
		maxChars := maxTokens * 4
		currentLen := sb.Len()
		remaining := maxChars - currentLen
		limit := remaining / 50 // Average ~50 chars per function line
		if limit < 10 {
			limit = 10
		}
		if limit > 50 {
			limit = 50
		}
		if len(functions) > limit {
			sb.WriteString(fmt.Sprintf("(showing first %d of %d)\n", limit, len(functions)))
			functions = functions[:limit]
		}
		for _, f := range functions {
			sb.WriteString(f + "\n")
		}
	}

	return sb.String()
}
