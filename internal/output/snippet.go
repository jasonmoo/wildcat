package output

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

const (
	// SmartSnippetMaxLines is the max lines for showing a complete AST unit
	SmartSnippetMaxLines = 10
	// SmartSnippetMinLines is the min lines for nested units to be shown whole
	SmartSnippetMinLines = 5
	// SmartSnippetFallbackContext is lines before/after for fallback
	SmartSnippetFallbackContext = 3
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

// ExtractSmart extracts an AST-aware snippet around a target line.
// For Go files, it tries to find a meaningful enclosing AST node (function,
// type declaration, loop, etc.) and returns the whole unit if it's small enough.
// Falls back to line-based extraction for non-Go files or large units.
// line is 1-indexed.
func (e *SnippetExtractor) ExtractSmart(filePath string, line int) (string, error) {
	// Only use AST for Go files
	if !strings.HasSuffix(filePath, ".go") {
		return e.Extract(filePath, line, SmartSnippetFallbackContext)
	}

	// Try AST-based extraction
	snippet, err := e.extractASTSnippet(filePath, line)
	if err != nil {
		// Fall back to line-based on any AST error
		return e.Extract(filePath, line, SmartSnippetFallbackContext)
	}

	return snippet, nil
}

// extractASTSnippet finds an enclosing AST node and extracts it if small enough.
// When the enclosing scope is too large, it falls back to a window that stays
// within the scope boundaries (never crossing into adjacent functions).
func (e *SnippetExtractor) extractASTSnippet(filePath string, targetLine int) (string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return "", err
	}

	// Find the enclosing node
	startLine, endLine, isTopLevel := e.findEnclosingNode(fset, f, targetLine)
	if startLine == 0 {
		// No suitable node found, use fallback (no scope to bound)
		return e.Extract(filePath, targetLine, SmartSnippetFallbackContext)
	}

	lineCount := endLine - startLine + 1

	// Decision logic:
	// - Top-level (func/type/const/var): show whole if ≤ maxLines
	// - Nested (for/switch/if/select): show whole if minLines ≤ size ≤ maxLines
	// - Otherwise: fall back with scope-bounded window
	showWhole := false
	if isTopLevel && lineCount <= SmartSnippetMaxLines {
		showWhole = true
	} else if !isTopLevel && lineCount >= SmartSnippetMinLines && lineCount <= SmartSnippetMaxLines {
		showWhole = true
	}

	if showWhole {
		return e.ExtractRange(filePath, startLine, endLine)
	}

	// Fall back to scope-bounded window
	// Compute window centered on target, clamped to scope boundaries
	windowSize := SmartSnippetFallbackContext*2 + 1 // 7 lines
	halfWindow := SmartSnippetFallbackContext       // 3 lines each side

	windowStart := targetLine - halfWindow
	windowEnd := targetLine + halfWindow

	// Clamp to scope boundaries
	if windowStart < startLine {
		windowStart = startLine
		windowEnd = min(startLine+windowSize-1, endLine)
	}
	if windowEnd > endLine {
		windowEnd = endLine
		windowStart = max(endLine-windowSize+1, startLine)
	}

	return e.ExtractRange(filePath, windowStart, windowEnd)
}

// findEnclosingNode finds the best enclosing AST node for a target line.
// Returns (startLine, endLine, isTopLevel). Returns (0, 0, false) if no suitable node.
func (e *SnippetExtractor) findEnclosingNode(fset *token.FileSet, f *ast.File, targetLine int) (int, int, bool) {
	var bestStart, bestEnd int
	var bestIsTopLevel bool
	var bestSize int = 1<<31 - 1 // Start with max int

	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			return true
		}

		startPos := fset.Position(n.Pos())
		endPos := fset.Position(n.End())

		// Check if this node contains the target line
		if startPos.Line > targetLine || endPos.Line < targetLine {
			return true // Continue searching
		}

		nodeSize := endPos.Line - startPos.Line + 1
		isTopLevel := false
		isCandidate := false

		switch n.(type) {
		case *ast.FuncDecl:
			isTopLevel = true
			isCandidate = true
		case *ast.GenDecl:
			// type, const, var declarations
			isTopLevel = true
			isCandidate = true
		case *ast.ForStmt, *ast.RangeStmt:
			isCandidate = true
		case *ast.SwitchStmt, *ast.TypeSwitchStmt:
			isCandidate = true
		case *ast.SelectStmt:
			isCandidate = true
		case *ast.IfStmt:
			isCandidate = true
		}

		// Pick the smallest candidate that contains our line
		if isCandidate && nodeSize < bestSize {
			bestStart = startPos.Line
			bestEnd = endPos.Line
			bestIsTopLevel = isTopLevel
			bestSize = nodeSize
		}

		return true // Keep searching for smaller nodes
	})

	return bestStart, bestEnd, bestIsTopLevel
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
