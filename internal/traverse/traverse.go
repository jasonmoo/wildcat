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

	"github.com/jasonmoo/wildcat/internal/lsp"
	"github.com/jasonmoo/wildcat/internal/output"
)

// Direction indicates traversal direction.
type Direction int

const (
	Up   Direction = iota // Callers (incoming calls)
	Down                  // Callees (outgoing calls)
)

// Options configures traversal behavior.
type Options struct {
	Direction     Direction
	MaxDepth      int
	ExcludeTests  bool
	ExcludeStdlib bool
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
			if opts.ExcludeStdlib && t.isStdlib(call.From.URI) {
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
			if opts.ExcludeStdlib && t.isStdlib(call.To.URI) {
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

// isStdlib checks if a URI is from the standard library.
func (t *Traverser) isStdlib(uri string) bool {
	path := lsp.URIToPath(uri)
	// Standard library is typically in GOROOT
	return strings.Contains(path, "/go/src/") ||
		strings.Contains(path, "/golang.org/") ||
		(!strings.Contains(path, "/") && !strings.HasPrefix(path, "."))
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

// BuildTree builds a call tree structure as paths.
func (t *Traverser) BuildTree(ctx context.Context, item lsp.CallHierarchyItem, opts Options) (*output.TreeResponse, error) {
	functions := make(map[string]output.TreeFunction)
	maxDepth := 0

	// Build all paths
	var paths [][]string
	visited := make(map[string]bool)

	// Target has no call site (it's the endpoint)
	targetName := symbolName(item)

	if opts.Direction == Up {
		// For "up", paths end with target (no call site)
		paths = t.buildPathsUp(ctx, item, opts, 0, 0, visited, functions, &maxDepth)
	} else {
		// For "down", paths start with target (with call site to first callee)
		paths = t.buildPathsDown(ctx, item, opts, 0, 0, visited, functions, &maxDepth)
	}

	direction := "down"
	if opts.Direction == Up {
		direction = "up"
	}

	return &output.TreeResponse{
		Query: output.TreeQuery{
			Command:   "tree",
			Target:    targetName,
			Depth:     opts.MaxDepth,
			Direction: direction,
		},
		Paths:     paths,
		Functions: functions,
		Summary: output.TreeSummary{
			PathCount:       len(paths),
			MaxDepthReached: maxDepth,
			Truncated:       opts.MaxDepth > 0 && maxDepth >= opts.MaxDepth,
		},
	}, nil
}

// buildPathsUp builds paths from callers to the target (paths end with target).
// callSite is the line where this item calls its callee (0 if this is the target/endpoint).
func (t *Traverser) buildPathsUp(ctx context.Context, item lsp.CallHierarchyItem, opts Options, depth int, callSite int, visited map[string]bool, functions map[string]output.TreeFunction, maxDepth *int) [][]string {
	// Build the path element name, including call site if we have one
	baseName := symbolName(item)
	name := baseName
	if callSite > 0 {
		name = fmt.Sprintf("%s:%d", baseName, callSite)
	}

	// Record function info (keyed by base name, not call site)
	file := lsp.URIToPath(item.URI)
	if _, exists := functions[baseName]; !exists {
		line := item.Range.Start.Line + 1
		sig := extractSignature(file, line)
		if sig == "" {
			sig = item.Name
		}
		functions[baseName] = output.TreeFunction{
			Signature: sig,
			Location:  fmt.Sprintf("%s:%d:%d", file, line, item.Range.End.Line+1),
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
		if opts.ExcludeStdlib && t.isStdlib(call.From.URI) {
			continue
		}

		// The caller's call site is where it calls the current item
		callerCallSite := 0
		if len(call.FromRanges) > 0 {
			callerCallSite = call.FromRanges[0].Start.Line + 1
		}

		// Recurse to get paths from this caller
		subPaths := t.buildPathsUp(ctx, call.From, opts, depth+1, callerCallSite, visited, functions, maxDepth)
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
func (t *Traverser) buildPathsDown(ctx context.Context, item lsp.CallHierarchyItem, opts Options, depth int, callSite int, visited map[string]bool, functions map[string]output.TreeFunction, maxDepth *int) [][]string {
	baseName := symbolName(item)
	file := lsp.URIToPath(item.URI)

	// Record function info (keyed by base name, not call site)
	if _, exists := functions[baseName]; !exists {
		line := item.Range.Start.Line + 1
		sig := extractSignature(file, line)
		if sig == "" {
			sig = item.Name
		}
		functions[baseName] = output.TreeFunction{
			Signature: sig,
			Location:  fmt.Sprintf("%s:%d:%d", file, line, item.Range.End.Line+1),
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
		if opts.ExcludeStdlib && t.isStdlib(call.To.URI) {
			continue
		}

		// The call site for this item is where it calls this callee
		myCallSite := 0
		if len(call.FromRanges) > 0 {
			myCallSite = call.FromRanges[0].Start.Line + 1
		}

		// Build name with our call site to this callee
		name := fmt.Sprintf("%s:%d", baseName, myCallSite)

		// Recurse to get paths from this callee (callee doesn't know its call site yet)
		subPaths := t.buildPathsDown(ctx, call.To, opts, depth+1, 0, visited, functions, maxDepth)
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

// extractSignature extracts a clean one-line function signature from a Go source file.
func extractSignature(filePath string, line int) string {
	if !strings.HasSuffix(filePath, ".go") {
		return ""
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return ""
	}

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		pos := fset.Position(fn.Pos())
		if pos.Line >= line-1 && pos.Line <= line+1 {
			return renderFuncSignature(fn)
		}
	}

	return ""
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
