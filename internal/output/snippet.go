package output

import (
	"bytes"
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

// fileCache holds raw bytes and line offsets for a file.
type fileCache struct {
	content     []byte // raw file bytes
	lineOffsets []int  // byte offset where each line starts (0-indexed by line number)
}

// SnippetExtractor extracts code snippets from source files.
type SnippetExtractor struct {
	cache map[string]*fileCache // file path -> cached content
}

// NewSnippetExtractor creates a new snippet extractor.
func NewSnippetExtractor() *SnippetExtractor {
	return &SnippetExtractor{
		cache: make(map[string]*fileCache),
	}
}

// Extract returns source lines around a position.
// line is 1-indexed (as displayed to users).
// contextLines specifies how many lines before and after to include.
func (e *SnippetExtractor) Extract(filePath string, line, contextLines int) (string, error) {
	fc, err := e.getFileCache(filePath)
	if err != nil {
		return "", err
	}

	if line < 1 || line > fc.lineCount() {
		return "", fmt.Errorf("line %d out of range (file has %d lines)", line, fc.lineCount())
	}

	startLine := max(1, line-contextLines)
	endLine := min(fc.lineCount(), line+contextLines)

	return fc.extractRange(startLine, endLine), nil
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
		// Return error - caller should emit diagnostic and handle gracefully
		return "<ast-parse-failed>", line, line, err
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
	fc, err := e.getFileCache(filePath)
	if err != nil {
		return "", err
	}

	if startLine > fc.lineCount() {
		return "", fmt.Errorf("start line %d out of range (file has %d lines)", startLine, fc.lineCount())
	}

	return fc.extractRange(startLine, endLine), nil
}

// IsUnique checks if a source string appears exactly once in the file.
// This helps determine if the snippet is safe to use with string-matching edit tools.
func (e *SnippetExtractor) IsUnique(filePath, source string) (bool, error) {
	fc, err := e.getFileCache(filePath)
	if err != nil {
		return false, fmt.Errorf("checking uniqueness: %w", err)
	}
	return bytes.Count(fc.content, []byte(source)) == 1, nil
}

// getFileCache returns the cached file content and line offsets.
func (e *SnippetExtractor) getFileCache(filePath string) (*fileCache, error) {
	// Check cache
	if fc, ok := e.cache[filePath]; ok {
		return fc, nil
	}

	// Read file as raw bytes
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	// Build line offset index
	// lineOffsets[i] is the byte offset where line i+1 starts (1-indexed lines)
	lineOffsets := []int{0} // line 1 starts at offset 0
	for i, b := range content {
		if b == '\n' {
			lineOffsets = append(lineOffsets, i+1)
		}
	}

	fc := &fileCache{
		content:     content,
		lineOffsets: lineOffsets,
	}
	e.cache[filePath] = fc

	return fc, nil
}

// lineCount returns the number of lines in the cached file.
func (fc *fileCache) lineCount() int {
	return len(fc.lineOffsets)
}

// extractRange returns bytes for lines startLine to endLine (1-indexed, inclusive).
// Includes trailing newline for byte-for-byte parity with source.
func (fc *fileCache) extractRange(startLine, endLine int) string {
	if startLine < 1 {
		startLine = 1
	}
	if endLine > fc.lineCount() {
		endLine = fc.lineCount()
	}
	if startLine > endLine || startLine > fc.lineCount() {
		return ""
	}

	startOffset := fc.lineOffsets[startLine-1]

	var endOffset int
	if endLine >= fc.lineCount() {
		// Last line - go to end of file
		endOffset = len(fc.content)
	} else {
		// End at start of next line (includes newline of endLine)
		endOffset = fc.lineOffsets[endLine]
	}

	return string(fc.content[startOffset:endOffset])
}

// MergeLocations merges locations within the same AST declaration scope.
// Locations in different top-level declarations stay separate.
// baseDir is the package directory (needed to construct full paths for AST parsing).
// Returns merged locations and any errors encountered during uniqueness checks.
func (e *SnippetExtractor) MergeLocations(baseDir string, locations []Location) ([]Location, []error) {
	if len(locations) <= 1 {
		return locations, nil
	}

	// Group by filename
	byFile := make(map[string][]Location)
	for _, loc := range locations {
		fileName, _ := parseLocation(loc.Location)
		byFile[fileName] = append(byFile[fileName], loc)
	}

	var merged []Location
	var errs []error
	for fileName, fileLocs := range byFile {
		if len(fileLocs) == 1 {
			merged = append(merged, fileLocs[0])
			continue
		}

		// Sort by line
		sortLocationsByLine(fileLocs)

		// Merge within declaration scopes
		fullPath := filepath.Join(baseDir, fileName)
		mergedFile, fileErrs := e.mergeLocationsByDeclaration(fullPath, fileName, fileLocs)
		merged = append(merged, mergedFile...)
		errs = append(errs, fileErrs...)
	}

	// Sort final results by location for consistent output
	sortLocationsByLocation(merged)
	return merged, errs
}

// mergeLocationsByDeclaration groups locations by their enclosing top-level declaration.
func (e *SnippetExtractor) mergeLocationsByDeclaration(fullPath, fileName string, locations []Location) ([]Location, []error) {
	// For Go files, use AST to find declaration scopes
	if strings.HasSuffix(fullPath, ".go") {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, fullPath, nil, parser.ParseComments)
		if err == nil {
			return e.mergeLocationsByAST(fset, f, fullPath, fileName, locations)
		}
	}

	// For non-Go files, fall back to proximity
	return e.mergeLocationsByProximity(fullPath, fileName, locations)
}

// mergeLocationsByAST merges locations within the same top-level AST declaration.
func (e *SnippetExtractor) mergeLocationsByAST(fset *token.FileSet, f *ast.File, fullPath, fileName string, locations []Location) ([]Location, []error) {
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

	// Group locations by which declaration contains them
	groups := make(map[int][]Location) // key is decl index (-1 for outside any decl)
	for _, loc := range locations {
		_, line := parseLocation(loc.Location)
		declIdx := -1
		for i, d := range decls {
			if line >= d.start && line <= d.end {
				declIdx = i
				break
			}
		}
		groups[declIdx] = append(groups[declIdx], loc)
	}

	// Create merged location for each group
	var merged []Location
	var errs []error
	for _, groupLocs := range groups {
		sortLocationsByLine(groupLocs)
		locs, err := e.finalizeLocationGroup(fullPath, fileName, groupLocs)
		merged = append(merged, locs...)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return merged, errs
}

// mergeLocationsByProximity merges locations that are close enough to have overlapping snippets.
func (e *SnippetExtractor) mergeLocationsByProximity(fullPath, fileName string, locations []Location) ([]Location, []error) {
	mergeDistance := SmartSnippetMaxLines

	var merged []Location
	var errs []error
	currentGroup := []Location{locations[0]}
	_, currentMaxLine := parseLocation(locations[0].Location)

	for i := 1; i < len(locations); i++ {
		loc := locations[i]
		_, line := parseLocation(loc.Location)

		if line-currentMaxLine <= mergeDistance {
			currentGroup = append(currentGroup, loc)
			if line > currentMaxLine {
				currentMaxLine = line
			}
		} else {
			locs, err := e.finalizeLocationGroup(fullPath, fileName, currentGroup)
			merged = append(merged, locs...)
			if err != nil {
				errs = append(errs, err)
			}
			currentGroup = []Location{loc}
			currentMaxLine = line
		}
	}

	locs, err := e.finalizeLocationGroup(fullPath, fileName, currentGroup)
	merged = append(merged, locs...)
	if err != nil {
		errs = append(errs, err)
	}
	return merged, errs
}

// finalizeLocationGroup creates a merged Location from a group of locations.
// Returns a slice with one merged location on success, or all original locations on error.
// Also returns any error encountered while checking uniqueness.
func (e *SnippetExtractor) finalizeLocationGroup(fullPath, fileName string, locations []Location) ([]Location, error) {
	if len(locations) == 1 {
		return locations, nil
	}

	// Collect all line numbers
	lines := make([]int, len(locations))
	for i, loc := range locations {
		_, lines[i] = parseLocation(loc.Location)
	}
	minLine := lines[0]
	maxLine := lines[len(lines)-1]

	// Extract merged snippet
	snippet, snippetStart, snippetEnd, err := e.extractMergedSnippet(fullPath, minLine, maxLine)
	if err != nil {
		// Return all locations unmerged so AI gets complete data
		return locations, nil
	}

	// Build comma-separated line list
	lineStrs := make([]string, len(lines))
	for i, l := range lines {
		lineStrs[i] = fmt.Sprintf("%d", l)
	}

	// Collect symbols (use first non-empty)
	symbol := ""
	for _, loc := range locations {
		if loc.Symbol != "" {
			symbol = loc.Symbol
			break
		}
	}

	unique, uniqueErr := e.IsUnique(fullPath, snippet)

	return []Location{{
		Location: fmt.Sprintf("%s:%s", fileName, strings.Join(lineStrs, ",")),
		Symbol:   symbol,
		Snippet: Snippet{
			Location: fmt.Sprintf("%s:%d:%d", fileName, snippetStart, snippetEnd),
			Source:   snippet,
			Unique:   unique,
		},
		RefCount: len(locations),
	}}, uniqueErr
}

// parseLocation extracts filename and line from "file.go:123" or "file.go:123,124,125".
// Returns the filename and the first line number.
func parseLocation(loc string) (string, int) {
	idx := strings.LastIndex(loc, ":")
	if idx == -1 {
		return loc, 0
	}
	fileName := loc[:idx]
	linesPart := loc[idx+1:]

	// Handle comma-separated lines, take first
	if commaIdx := strings.Index(linesPart, ","); commaIdx != -1 {
		linesPart = linesPart[:commaIdx]
	}

	line := 0
	fmt.Sscanf(linesPart, "%d", &line)
	return fileName, line
}

// sortLocationsByLine sorts locations by their first line number.
func sortLocationsByLine(locations []Location) {
	sort.Slice(locations, func(i, j int) bool {
		_, lineI := parseLocation(locations[i].Location)
		_, lineJ := parseLocation(locations[j].Location)
		return lineI < lineJ
	})
}

// sortLocationsByLocation sorts locations by filename then line.
func sortLocationsByLocation(locations []Location) {
	sort.Slice(locations, func(i, j int) bool {
		fileI, lineI := parseLocation(locations[i].Location)
		fileJ, lineJ := parseLocation(locations[j].Location)
		if fileI != fileJ {
			return fileI < fileJ
		}
		return lineI < lineJ
	})
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

