// Package traverse provides call hierarchy traversal for Wildcat.
package traverse

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"

	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/lsp"
	"github.com/jasonmoo/wildcat/internal/output"
)

// Direction indicates traversal direction.
type Direction int

const (
	Up   Direction = iota // Callers (incoming calls)
	Down                  // Callees (outgoing calls)
)

// Scope controls how far traversal extends.
type Scope string

const (
	ScopeAll     Scope = "all"     // Include everything (stdlib, deps)
	ScopeProject Scope = "project" // Project packages only (no stdlib, no deps)
	ScopePackage Scope = "package" // Same package as starting symbol only
)

// Options configures traversal behavior.
type Options struct {
	Direction    Direction
	MaxDepth     int
	ExcludeTests bool
	Scope        Scope  // Traversal boundary (default: project)
	StartFile    string // Starting symbol's file (for package scope)
}

// CallInfo contains information about a call site.
type CallInfo struct {
	Symbol     string
	File       string
	Line       int
	LineEnd    int
	CallRanges []lsp.Range // Where the calls happen
	InTest     bool
}

// Traverser walks the call hierarchy.
type Traverser struct {
	client    *lsp.Client
	extractor *output.SnippetExtractor
}

// NewTraverser creates a new call hierarchy traverser.
func NewTraverser(client *lsp.Client) *Traverser {
	return &Traverser{
		client:    client,
		extractor: output.NewSnippetExtractor(),
	}
}

// GetCallers returns all callers of a call hierarchy item.
func (t *Traverser) GetCallers(ctx context.Context, item lsp.CallHierarchyItem, opts Options) ([]CallInfo, error) {
	return t.traverse(ctx, item, opts, 0, make(map[string]bool))
}

// GetCallees returns all callees of a call hierarchy item.
func (t *Traverser) GetCallees(ctx context.Context, item lsp.CallHierarchyItem, opts Options) ([]CallInfo, error) {
	opts.Direction = Down
	return t.traverse(ctx, item, opts, 0, make(map[string]bool))
}

// traverse recursively walks the call hierarchy.
func (t *Traverser) traverse(ctx context.Context, item lsp.CallHierarchyItem, opts Options, depth int, visited map[string]bool) ([]CallInfo, error) {
	// Check depth limit
	if opts.MaxDepth > 0 && depth >= opts.MaxDepth {
		return nil, nil
	}

	// Prevent cycles
	key := item.URI + ":" + item.Name
	if visited[key] {
		return nil, nil
	}
	visited[key] = true

	var results []CallInfo

	if opts.Direction == Up {
		// Get incoming calls (callers)
		calls, err := t.client.IncomingCalls(ctx, item)
		if err != nil {
			return nil, err
		}

		for _, call := range calls {
			info := t.callInfoFromIncoming(call)

			// Apply filters
			if opts.ExcludeTests && info.InTest {
				continue
			}
			if !t.inScope(call.From.URI, opts) {
				continue
			}

			results = append(results, info)

			// Recurse
			if opts.MaxDepth == 0 || depth+1 < opts.MaxDepth {
				sub, err := t.traverse(ctx, call.From, opts, depth+1, visited)
				if err != nil {
					return nil, err
				}
				results = append(results, sub...)
			}
		}
	} else {
		// Get outgoing calls (callees)
		calls, err := t.client.OutgoingCalls(ctx, item)
		if err != nil {
			return nil, err
		}

		for _, call := range calls {
			info := t.callInfoFromOutgoing(call)

			// Apply filters
			if opts.ExcludeTests && info.InTest {
				continue
			}
			if !t.inScope(call.To.URI, opts) {
				continue
			}

			results = append(results, info)

			// Recurse
			if opts.MaxDepth == 0 || depth+1 < opts.MaxDepth {
				sub, err := t.traverse(ctx, call.To, opts, depth+1, visited)
				if err != nil {
					return nil, err
				}
				results = append(results, sub...)
			}
		}
	}

	return results, nil
}

// callInfoFromIncoming creates CallInfo from an incoming call.
func (t *Traverser) callInfoFromIncoming(call lsp.CallHierarchyIncomingCall) CallInfo {
	file := lsp.URIToPath(call.From.URI)
	return CallInfo{
		Symbol:     call.From.Name,
		File:       file,
		Line:       call.From.Range.Start.Line + 1, // LSP is 0-indexed
		LineEnd:    call.From.Range.End.Line + 1,
		CallRanges: call.FromRanges,
		InTest:     output.IsTestFile(file),
	}
}

// callInfoFromOutgoing creates CallInfo from an outgoing call.
func (t *Traverser) callInfoFromOutgoing(call lsp.CallHierarchyOutgoingCall) CallInfo {
	file := lsp.URIToPath(call.To.URI)
	return CallInfo{
		Symbol:     call.To.Name,
		File:       file,
		Line:       call.To.Range.Start.Line + 1,
		LineEnd:    call.To.Range.End.Line + 1,
		CallRanges: call.FromRanges,
		InTest:     output.IsTestFile(file),
	}
}

// inScope checks if a URI is within the specified scope.
func (t *Traverser) inScope(uri string, opts Options) bool {
	path := lsp.URIToPath(uri)

	switch opts.Scope {
	case ScopeAll:
		return true
	case ScopePackage:
		return golang.IsSamePackage(path, opts.StartFile)
	case ScopeProject:
		fallthrough
	default:
		return golang.IsProjectPath(path)
	}
}

// symbolName returns a qualified symbol name like "pkg.Symbol".
func symbolName(item lsp.CallHierarchyItem) string {
	file := lsp.URIToPath(item.URI)
	pkg := packageFromPath(file)
	if pkg != "" {
		return pkg + "." + item.Name
	}
	return item.Name
}

// packageFromPath extracts a short package name from a file path.
func packageFromPath(path string) string {
	// Extract directory containing the file
	idx := strings.LastIndex(path, "/")
	if idx == -1 {
		return ""
	}
	dir := path[:idx]

	// Get just the last component (package directory name)
	idx = strings.LastIndex(dir, "/")
	if idx == -1 {
		return dir
	}
	return dir[idx+1:]
}

// collectedFunc holds function data during traversal before grouping by package.
type collectedFunc struct {
	name       string // short name like "runTree"
	file       string // full file path
	importPath string // package import path
	signature  string
	startLine  int
	endLine    int
}

// BuildTree builds a call tree structure as paths.
func (t *Traverser) BuildTree(ctx context.Context, item lsp.CallHierarchyItem, opts Options) (*output.TreeResponse, error) {
	collected := make(map[string]*collectedFunc) // keyed by qualified name
	maxDepth := 0

	// Build all paths
	var paths [][]string
	visited := make(map[string]bool)

	// Target has no call site (it's the endpoint)
	targetName := symbolName(item)

	if opts.Direction == Up {
		// For "up", paths end with target (no call site)
		paths = t.buildPathsUp(ctx, item, opts, 0, 0, visited, collected, &maxDepth)
	} else {
		// For "down", paths start with target (with call site to first callee)
		paths = t.buildPathsDown(ctx, item, opts, 0, 0, visited, collected, &maxDepth)
	}

	direction := "down"
	if opts.Direction == Up {
		direction = "up"
	}

	// Group collected functions by package
	packages := groupByPackage(collected)

	return &output.TreeResponse{
		Query: output.TreeQuery{
			Command:   "tree",
			Target:    targetName,
			Depth:     opts.MaxDepth,
			Direction: direction,
		},
		Paths:    paths,
		Packages: packages,
		Summary: output.TreeSummary{
			PathCount:       len(paths),
			MaxDepthReached: maxDepth,
			Truncated:       opts.MaxDepth > 0 && maxDepth >= opts.MaxDepth,
		},
	}, nil
}

// groupByPackage converts collected functions into package-grouped output.
func groupByPackage(collected map[string]*collectedFunc) []output.TreePackage {
	// Group by import path
	pkgMap := make(map[string][]output.TreeFunction)
	pkgDirs := make(map[string]string)
	var pkgOrder []string

	for _, cf := range collected {
		if _, exists := pkgMap[cf.importPath]; !exists {
			pkgOrder = append(pkgOrder, cf.importPath)
			// Get dir from file path
			dir := cf.file
			if idx := strings.LastIndex(cf.file, "/"); idx >= 0 {
				dir = cf.file[:idx]
			}
			pkgDirs[cf.importPath] = dir
		}

		fileName := cf.file
		if idx := strings.LastIndex(cf.file, "/"); idx >= 0 {
			fileName = cf.file[idx+1:]
		}

		pkgMap[cf.importPath] = append(pkgMap[cf.importPath], output.TreeFunction{
			Name:       cf.name,
			Signature:  cf.signature,
			Definition: fmt.Sprintf("%s:%d:%d", fileName, cf.startLine, cf.endLine),
		})
	}

	// Build output in order
	var packages []output.TreePackage
	for _, pkg := range pkgOrder {
		packages = append(packages, output.TreePackage{
			Package: pkg,
			Dir:     pkgDirs[pkg],
			Symbols: pkgMap[pkg],
		})
	}
	return packages
}

// buildPathsUp builds paths from callers to the target (paths end with target).
// callSite is the line where this item calls its callee (0 if this is the target/endpoint).
func (t *Traverser) buildPathsUp(ctx context.Context, item lsp.CallHierarchyItem, opts Options, depth int, callSite int, visited map[string]bool, collected map[string]*collectedFunc, maxDepth *int) [][]string {
	// Build the path element name, including call site if we have one
	baseName := symbolName(item)
	name := baseName
	if callSite > 0 {
		name = fmt.Sprintf("%s:%d", baseName, callSite)
	}

	// Record function info (keyed by base name, not call site)
	file := lsp.URIToPath(item.URI)
	if _, exists := collected[baseName]; !exists {
		line := item.Range.Start.Line + 1
		info := extractFuncInfo(file, line)
		sig := info.signature
		if sig == "" {
			sig = item.Name
		}
		startLine, endLine := info.startLine, info.endLine
		if startLine == 0 {
			startLine = line
			endLine = item.Range.End.Line + 1
		}
		// Get import path - resolve from file path
		dir := file
		if idx := strings.LastIndex(file, "/"); idx >= 0 {
			dir = file[:idx]
		}
		importPath, _ := golang.ResolvePackagePath(dir, dir)
		if importPath == "" {
			importPath = packageFromPath(file)
		}
		collected[baseName] = &collectedFunc{
			name:       item.Name,
			file:       file,
			importPath: importPath,
			signature:  sig,
			startLine:  startLine,
			endLine:    endLine,
		}
	}

	if depth > *maxDepth {
		*maxDepth = depth
	}

	if opts.MaxDepth > 0 && depth >= opts.MaxDepth {
		return [][]string{{name}}
	}

	key := item.URI + ":" + item.Name
	if visited[key] {
		return [][]string{{name}}
	}
	visited[key] = true
	defer func() { visited[key] = false }() // Allow revisiting on different paths

	calls, err := t.client.IncomingCalls(ctx, item)
	if err != nil || len(calls) == 0 {
		return [][]string{{name}}
	}

	var paths [][]string
	for _, call := range calls {
		callFile := lsp.URIToPath(call.From.URI)

		if opts.ExcludeTests && output.IsTestFile(callFile) {
			continue
		}
		if !t.inScope(call.From.URI, opts) {
			continue
		}

		// The caller's call site is where it calls the current item
		callerCallSite := 0
		if len(call.FromRanges) > 0 {
			callerCallSite = call.FromRanges[0].Start.Line + 1
		}

		// Recurse to get paths from this caller
		subPaths := t.buildPathsUp(ctx, call.From, opts, depth+1, callerCallSite, visited, collected, maxDepth)
		for _, subPath := range subPaths {
			// Append current item to each sub-path
			path := append(subPath, name)
			paths = append(paths, path)
		}
	}

	if len(paths) == 0 {
		return [][]string{{name}}
	}
	return paths
}

// buildPathsDown builds paths from target to callees (paths start with target).
// callSite is the line where this item is called from (0 if this is the target/root).
func (t *Traverser) buildPathsDown(ctx context.Context, item lsp.CallHierarchyItem, opts Options, depth int, callSite int, visited map[string]bool, collected map[string]*collectedFunc, maxDepth *int) [][]string {
	baseName := symbolName(item)
	file := lsp.URIToPath(item.URI)

	// Record function info (keyed by base name, not call site)
	if _, exists := collected[baseName]; !exists {
		line := item.Range.Start.Line + 1
		info := extractFuncInfo(file, line)
		sig := info.signature
		if sig == "" {
			sig = item.Name
		}
		startLine, endLine := info.startLine, info.endLine
		if startLine == 0 {
			startLine = line
			endLine = item.Range.End.Line + 1
		}
		// Get import path - resolve from file path
		dir := file
		if idx := strings.LastIndex(file, "/"); idx >= 0 {
			dir = file[:idx]
		}
		importPath, _ := golang.ResolvePackagePath(dir, dir)
		if importPath == "" {
			importPath = packageFromPath(file)
		}
		collected[baseName] = &collectedFunc{
			name:       item.Name,
			file:       file,
			importPath: importPath,
			signature:  sig,
			startLine:  startLine,
			endLine:    endLine,
		}
	}

	if depth > *maxDepth {
		*maxDepth = depth
	}

	if opts.MaxDepth > 0 && depth >= opts.MaxDepth {
		// At depth limit, return with call site if we have one
		name := baseName
		if callSite > 0 {
			name = fmt.Sprintf("%s:%d", baseName, callSite)
		}
		return [][]string{{name}}
	}

	key := item.URI + ":" + item.Name
	if visited[key] {
		name := baseName
		if callSite > 0 {
			name = fmt.Sprintf("%s:%d", baseName, callSite)
		}
		return [][]string{{name}}
	}
	visited[key] = true
	defer func() { visited[key] = false }() // Allow revisiting on different paths

	calls, err := t.client.OutgoingCalls(ctx, item)
	if err != nil || len(calls) == 0 {
		// This is a leaf node, no call site to next element
		name := baseName
		if callSite > 0 {
			name = fmt.Sprintf("%s:%d", baseName, callSite)
		}
		return [][]string{{name}}
	}

	var paths [][]string
	for _, call := range calls {
		callFile := lsp.URIToPath(call.To.URI)

		if opts.ExcludeTests && output.IsTestFile(callFile) {
			continue
		}

		// The call site for this item is where it calls this callee
		myCallSite := 0
		if len(call.FromRanges) > 0 {
			myCallSite = call.FromRanges[0].Start.Line + 1
		}

		// If callee is out of scope, record as leaf and don't recurse
		if !t.inScope(call.To.URI, opts) {
			name := baseName
			if myCallSite > 0 {
				name = fmt.Sprintf("%s:%d", baseName, myCallSite)
			}
			paths = append(paths, []string{name})
			continue
		}

		// Build name with our call site to this callee
		name := fmt.Sprintf("%s:%d", baseName, myCallSite)

		// Recurse to get paths from this callee (callee doesn't know its call site yet)
		subPaths := t.buildPathsDown(ctx, call.To, opts, depth+1, 0, visited, collected, maxDepth)
		for _, subPath := range subPaths {
			// Prepend current item (with call site) to each sub-path
			path := append([]string{name}, subPath...)
			paths = append(paths, path)
		}
	}

	if len(paths) == 0 {
		name := baseName
		if callSite > 0 {
			name = fmt.Sprintf("%s:%d", baseName, callSite)
		}
		return [][]string{{name}}
	}
	return paths
}

// funcInfo holds extracted function information.
type funcInfo struct {
	signature string
	startLine int
	endLine   int
}

// extractFuncInfo extracts signature and full definition range from a Go source file.
func extractFuncInfo(filePath string, line int) funcInfo {
	if !strings.HasSuffix(filePath, ".go") {
		return funcInfo{}
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return funcInfo{}
	}

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		pos := fset.Position(fn.Pos())
		if pos.Line >= line-1 && pos.Line <= line+1 {
			endPos := fset.Position(fn.End())
			return funcInfo{
				signature: renderFuncSignature(fn),
				startLine: pos.Line,
				endLine:   endPos.Line,
			}
		}
	}

	return funcInfo{}
}

// renderFuncSignature renders a function declaration as a normalized one-line signature.
func renderFuncSignature(decl *ast.FuncDecl) string {
	cleaned := *decl
	cleaned.Doc = nil
	cleaned.Body = nil

	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), &cleaned); err != nil {
		return ""
	}
	return buf.String()
}
