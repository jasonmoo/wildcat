package output

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
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
// line is 1-indexed. Returns snippet content and line range.
func (e *SnippetExtractor) ExtractSmart(filePath string, line int) (string, int, int, error) {
	// Only use AST for Go files
	if !strings.HasSuffix(filePath, ".go") {
		snippet, err := e.Extract(filePath, line, SmartSnippetFallbackContext)
		start := max(1, line-SmartSnippetFallbackContext)
		end := line + SmartSnippetFallbackContext
		return snippet, start, end, err
	}

	// Try AST-based extraction
	snippet, start, end, err := e.extractASTSnippet(filePath, line)
	if err != nil {
		// Fall back to line-based on any AST error
		snippet, err := e.Extract(filePath, line, SmartSnippetFallbackContext)
		start := max(1, line-SmartSnippetFallbackContext)
		end := line + SmartSnippetFallbackContext
		return snippet, start, end, err
	}

	return snippet, start, end, nil
}

// extractASTSnippet finds an enclosing AST node and extracts it if small enough.
// When the enclosing scope is too large, it falls back to a window that stays
// within the scope boundaries (never crossing into adjacent functions).
// Returns snippet content and line range.
func (e *SnippetExtractor) extractASTSnippet(filePath string, targetLine int) (string, int, int, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return "", 0, 0, err
	}

	// Find the enclosing node
	startLine, endLine, isTopLevel := e.findEnclosingNode(fset, f, targetLine)
	if startLine == 0 {
		// No suitable node found, use fallback (no scope to bound)
		snippet, err := e.Extract(filePath, targetLine, SmartSnippetFallbackContext)
		start := max(1, targetLine-SmartSnippetFallbackContext)
		end := targetLine + SmartSnippetFallbackContext
		return snippet, start, end, err
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
		snippet, err := e.ExtractRange(filePath, startLine, endLine)
		return snippet, startLine, endLine, err
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

	snippet, err := e.ExtractRange(filePath, windowStart, windowEnd)
	return snippet, windowStart, windowEnd, err
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

// MergeOverlappingResults merges results that have overlapping or adjacent snippets.
// Results in the same file within mergeDistance lines are combined into a single
// result with a merged snippet covering all reference lines.
func (e *SnippetExtractor) MergeOverlappingResults(results []Result) []Result {
	if len(results) <= 1 {
		return results
	}

	// Group by file
	byFile := make(map[string][]Result)
	for _, r := range results {
		byFile[r.File] = append(byFile[r.File], r)
	}

	var merged []Result
	for file, fileResults := range byFile {
		if len(fileResults) == 1 {
			merged = append(merged, fileResults[0])
			continue
		}

		// Sort by line
		sortResultsByLine(fileResults)

		// Merge adjacent results
		mergedFile := e.mergeAdjacentResults(file, fileResults)
		merged = append(merged, mergedFile...)
	}

	// Sort final results by file then line for consistent output
	sortResultsByFileLine(merged)
	return merged
}

// mergeAdjacentResults merges results within the same top-level declaration.
// Results in different declarations (functions, types, etc.) stay separate.
func (e *SnippetExtractor) mergeAdjacentResults(file string, results []Result) []Result {
	// For Go files, group by top-level declaration
	if strings.HasSuffix(file, ".go") {
		return e.mergeByDeclaration(file, results)
	}

	// For non-Go files, fall back to line proximity
	return e.mergeByProximity(file, results)
}

// mergeByDeclaration groups results by their enclosing top-level declaration.
func (e *SnippetExtractor) mergeByDeclaration(file string, results []Result) []Result {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
	if err != nil {
		return e.mergeByProximity(file, results)
	}

	// Find top-level declaration ranges
	type declRange struct {
		start, end int
	}
	var decls []declRange

	for _, decl := range f.Decls {
		start := fset.Position(decl.Pos()).Line
		end := fset.Position(decl.End()).Line
		decls = append(decls, declRange{start, end})
	}

	// Group results by which declaration contains them
	groups := make(map[int][]Result) // key is decl index
	for _, r := range results {
		declIdx := -1
		for i, d := range decls {
			if r.Line >= d.start && r.Line <= d.end {
				declIdx = i
				break
			}
		}
		groups[declIdx] = append(groups[declIdx], r)
	}

	// Create merged result for each group
	var merged []Result
	for _, groupResults := range groups {
		if len(groupResults) == 1 {
			merged = append(merged, groupResults[0])
		} else {
			sortResultsByLine(groupResults)
			lines := make([]int, len(groupResults))
			for i, r := range groupResults {
				lines[i] = r.Line
			}
			merged = append(merged, e.finalizeGroup(file, groupResults[0], lines))
		}
	}

	return merged
}

// mergeByProximity merges results that are close enough to have overlapping snippets.
func (e *SnippetExtractor) mergeByProximity(file string, results []Result) []Result {
	mergeDistance := SmartSnippetMaxLines

	var merged []Result
	current := results[0]
	currentLines := []int{current.Line}
	currentMaxLine := current.Line

	for i := 1; i < len(results); i++ {
		r := results[i]

		if r.Line-currentMaxLine <= mergeDistance {
			currentLines = append(currentLines, r.Line)
			if r.Line > currentMaxLine {
				currentMaxLine = r.Line
			}
			if r.InTest {
				current.InTest = true
			}
		} else {
			merged = append(merged, e.finalizeGroup(file, current, currentLines))
			current = r
			currentLines = []int{r.Line}
			currentMaxLine = r.Line
		}
	}

	merged = append(merged, e.finalizeGroup(file, current, currentLines))
	return merged
}

// finalizeGroup creates a merged result from a group of lines.
func (e *SnippetExtractor) finalizeGroup(file string, base Result, lines []int) Result {
	if len(lines) == 1 {
		// Single result, no merge needed
		return base
	}

	// Multiple lines - extract a snippet covering all of them
	minLine := lines[0]
	maxLine := lines[len(lines)-1]

	// Extract smart snippet for the range
	snippet, snippetStart, snippetEnd, err := e.extractMergedSnippet(file, minLine, maxLine)
	if err != nil {
		// Fallback to base snippet with estimated range
		snippet = base.Snippet
		snippetStart = minLine
		snippetEnd = maxLine
	}

	return Result{
		File:         file,
		Lines:        lines, // Line is omitted when Lines is set
		Snippet:      snippet,
		SnippetStart: snippetStart,
		SnippetEnd:   snippetEnd,
		InTest:       base.InTest,
	}
}

// extractMergedSnippet extracts a snippet covering multiple reference lines.
// Returns the snippet content and its line range.
func (e *SnippetExtractor) extractMergedSnippet(file string, minLine, maxLine int) (string, int, int, error) {
	// For Go files, try to find enclosing AST nodes
	if strings.HasSuffix(file, ".go") {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err == nil {
			// Find enclosing scopes for both min and max lines, then union them
			startLine, endLine := e.findEnclosingUnion(fset, f, minLine, maxLine)
			if startLine > 0 {
				// Check if the combined scope is reasonable
				if endLine-startLine+1 <= SmartSnippetMaxLines*3 {
					snippet, err := e.ExtractRange(file, startLine, endLine)
					return snippet, startLine, endLine, err
				}
				// Scope too large, extract just the lines we need plus context
				extractStart := max(startLine, minLine-SmartSnippetFallbackContext)
				extractEnd := min(endLine, maxLine+SmartSnippetFallbackContext)
				snippet, err := e.ExtractRange(file, extractStart, extractEnd)
				return snippet, extractStart, extractEnd, err
			}
		}
	}

	// Fallback: extract range with context
	extractEnd := maxLine + SmartSnippetFallbackContext
	snippet, err := e.ExtractRange(file, minLine, extractEnd)
	return snippet, minLine, extractEnd, err
}

// findEnclosingUnion finds enclosing scopes for both lines and returns their union.
// This handles the case where minLine and maxLine are in different scopes.
func (e *SnippetExtractor) findEnclosingUnion(fset *token.FileSet, f *ast.File, minLine, maxLine int) (int, int) {
	// First try to find a single node containing both
	start, end := e.findEnclosingRange(fset, f, minLine, maxLine)
	if start > 0 {
		return start, end
	}

	// Find enclosing scope for each line separately and union them
	minStart, minEnd, _ := e.findEnclosingNode(fset, f, minLine)
	maxStart, maxEnd, _ := e.findEnclosingNode(fset, f, maxLine)

	if minStart == 0 && maxStart == 0 {
		return 0, 0 // No scopes found
	}
	if minStart == 0 {
		return maxStart, maxEnd
	}
	if maxStart == 0 {
		return minStart, minEnd
	}

	// Union the two ranges
	return min(minStart, maxStart), max(minEnd, maxEnd)
}

// findEnclosingRange finds the smallest AST node containing both minLine and maxLine.
func (e *SnippetExtractor) findEnclosingRange(fset *token.FileSet, f *ast.File, minLine, maxLine int) (int, int) {
	var bestStart, bestEnd int
	var bestSize int = 1<<31 - 1

	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			return true
		}

		startPos := fset.Position(n.Pos())
		endPos := fset.Position(n.End())

		// Check if this node contains both lines
		if startPos.Line > minLine || endPos.Line < maxLine {
			return true
		}

		nodeSize := endPos.Line - startPos.Line + 1
		isCandidate := false

		switch n.(type) {
		case *ast.FuncDecl, *ast.GenDecl:
			isCandidate = true
		case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt, *ast.IfStmt:
			isCandidate = true
		}

		if isCandidate && nodeSize < bestSize {
			bestStart = startPos.Line
			bestEnd = endPos.Line
			bestSize = nodeSize
		}

		return true
	})

	return bestStart, bestEnd
}

// sortResultsByLine sorts results by line number.
func sortResultsByLine(results []Result) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Line < results[j].Line
	})
}

// sortResultsByFileLine sorts results by file then line.
func sortResultsByFileLine(results []Result) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].File != results[j].File {
			return results[i].File < results[j].File
		}
		return results[i].Line < results[j].Line
	})
}
