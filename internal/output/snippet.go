package output

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SnippetExtractor extracts code snippets from source files.
type SnippetExtractor struct {
	cache map[string][]string // file path -> lines
}

// NewSnippetExtractor creates a new snippet extractor.
func NewSnippetExtractor() *SnippetExtractor {
	return &SnippetExtractor{
		cache: make(map[string][]string),
	}
}

// Extract returns source lines around a position.
// line is 1-indexed (as displayed to users).
// contextLines specifies how many lines before and after to include.
func (e *SnippetExtractor) Extract(filePath string, line, contextLines int) (string, error) {
	lines, err := e.getLines(filePath)
	if err != nil {
		return "", err
	}

	// Convert to 0-indexed
	lineIdx := line - 1
	if lineIdx < 0 || lineIdx >= len(lines) {
		return "", fmt.Errorf("line %d out of range (file has %d lines)", line, len(lines))
	}

	start := max(0, lineIdx-contextLines)
	end := min(len(lines), lineIdx+contextLines+1)

	return strings.Join(lines[start:end], "\n"), nil
}

// ExtractRange returns source lines for a range.
// startLine and endLine are 1-indexed.
func (e *SnippetExtractor) ExtractRange(filePath string, startLine, endLine int) (string, error) {
	lines, err := e.getLines(filePath)
	if err != nil {
		return "", err
	}

	// Convert to 0-indexed
	startIdx := max(0, startLine-1)
	endIdx := min(len(lines), endLine)

	if startIdx >= len(lines) {
		return "", fmt.Errorf("start line %d out of range (file has %d lines)", startLine, len(lines))
	}

	return strings.Join(lines[startIdx:endIdx], "\n"), nil
}

// ExtractLine returns a single line.
// line is 1-indexed.
func (e *SnippetExtractor) ExtractLine(filePath string, line int) (string, error) {
	lines, err := e.getLines(filePath)
	if err != nil {
		return "", err
	}

	lineIdx := line - 1
	if lineIdx < 0 || lineIdx >= len(lines) {
		return "", fmt.Errorf("line %d out of range (file has %d lines)", line, len(lines))
	}

	return lines[lineIdx], nil
}

// ExtractCallExpr extracts a call expression from a line at the given column range.
// line is 1-indexed, startCol and endCol are 0-indexed character positions.
func (e *SnippetExtractor) ExtractCallExpr(filePath string, line, startCol, endCol int) (string, error) {
	lineText, err := e.ExtractLine(filePath, line)
	if err != nil {
		return "", err
	}

	// Clamp to line bounds
	if startCol < 0 {
		startCol = 0
	}
	if endCol > len(lineText) {
		endCol = len(lineText)
	}
	if startCol >= endCol {
		return "", nil
	}

	return lineText[startCol:endCol], nil
}

// getLines returns the lines of a file, using cache.
func (e *SnippetExtractor) getLines(filePath string) ([]string, error) {
	// Check cache
	if lines, ok := e.cache[filePath]; ok {
		return lines, nil
	}

	// Read file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	// Cache the result
	e.cache[filePath] = lines

	return lines, nil
}

// ClearCache clears the file cache.
func (e *SnippetExtractor) ClearCache() {
	e.cache = make(map[string][]string)
}

// IsTestFile returns true if the file appears to be a test file.
func IsTestFile(filePath string) bool {
	base := filepath.Base(filePath)
	return strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, "_test.py") ||
		strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".test.js") ||
		strings.HasSuffix(base, ".spec.ts") ||
		strings.HasSuffix(base, ".spec.js") ||
		strings.HasSuffix(base, "_test.rs") ||
		strings.Contains(base, "test_") ||
		strings.HasPrefix(base, "test_")
}

// AbsolutePath ensures a path is absolute.
func AbsolutePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
