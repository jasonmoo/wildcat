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
	"sort"
	"strings"

	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/lsp"
	"github.com/jasonmoo/wildcat/internal/output"
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
	UpDepth      int    // How many levels of callers (0 = skip)
	DownDepth    int    // How many levels of callees (0 = skip)
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

// GetCallers returns all callers of a call hierarchy item (flat list).
// Uses UpDepth from opts to control depth.
func (t *Traverser) GetCallers(ctx context.Context, item lsp.CallHierarchyItem, opts Options) ([]CallInfo, error) {
	return t.traverseUp(ctx, item, opts, 0, make(map[string]bool))
}

// GetCallees returns all callees of a call hierarchy item (flat list).
// Uses DownDepth from opts to control depth.
func (t *Traverser) GetCallees(ctx context.Context, item lsp.CallHierarchyItem, opts Options) ([]CallInfo, error) {
	return t.traverseDown(ctx, item, opts, 0, make(map[string]bool))
}

// traverseUp recursively walks callers.
func (t *Traverser) traverseUp(ctx context.Context, item lsp.CallHierarchyItem, opts Options, depth int, visited map[string]bool) ([]CallInfo, error) {
	// Check depth limit
	if opts.UpDepth > 0 && depth >= opts.UpDepth {
		return nil, nil
	}

	// Prevent cycles
	key := item.URI + ":" + item.Name
	if visited[key] {
		return nil, nil
	}
	visited[key] = true

	// Get incoming calls (callers)
	calls, err := t.client.IncomingCalls(ctx, item)
	if err != nil {
		return nil, err
	}

	var results []CallInfo
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
		if opts.UpDepth == 0 || depth+1 < opts.UpDepth {
			sub, err := t.traverseUp(ctx, call.From, opts, depth+1, visited)
			if err != nil {
				return nil, err
			}
			results = append(results, sub...)
		}
	}

	return results, nil
}

// traverseDown recursively walks callees.
func (t *Traverser) traverseDown(ctx context.Context, item lsp.CallHierarchyItem, opts Options, depth int, visited map[string]bool) ([]CallInfo, error) {
	// Check depth limit
	if opts.DownDepth > 0 && depth >= opts.DownDepth {
		return nil, nil
	}

	// Prevent cycles
	key := item.URI + ":" + item.Name
	if visited[key] {
		return nil, nil
	}
	visited[key] = true

	// Get outgoing calls (callees)
	calls, err := t.client.OutgoingCalls(ctx, item)
	if err != nil {
		return nil, err
	}

	var results []CallInfo
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
		if opts.DownDepth == 0 || depth+1 < opts.DownDepth {
			sub, err := t.traverseDown(ctx, call.To, opts, depth+1, visited)
			if err != nil {
				return nil, err
			}
			results = append(results, sub...)
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

// symbolName returns a qualified symbol name like "pkg.Symbol" or "pkg.Type.Method".
func symbolName(item lsp.CallHierarchyItem) string {
	file := lsp.URIToPath(item.URI)
	pkg := packageFromPath(file)

	// Check if this is a method by parsing the file
	line := item.Range.Start.Line + 1
	info := extractFuncInfo(file, line)

	// Build the short name - use Type.Method for methods
	shortName := item.Name
	if info.receiverType != "" {
		shortName = info.receiverType + "." + item.Name
	}

	if pkg != "" {
		return pkg + "." + shortName
	}
	return shortName
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
	name       string // short name like "runTree" or "Type.Method"
	pkgName    string // short package name like "cmd", "lsp"
	file       string // full file path
	importPath string // package import path
	signature  string
	startLine  int
	endLine    int
}

// BuildTree builds a bidirectional call tree centered on the target symbol.
func (t *Traverser) BuildTree(ctx context.Context, item lsp.CallHierarchyItem, opts Options) (*output.TreeResponse, error) {
	collected := make(map[string]*collectedFunc) // keyed by qualified name

	// Track stats for each direction
	var maxUpDepth, maxDownDepth int
	var totalCallers, totalCallees int

	// Build root node
	symbol := symbolName(item)
	file := lsp.URIToPath(item.URI)
	t.collectFunc(item, file, collected)

	tree := &output.TreeNode{Symbol: symbol}

	// Build callers tree (up)
	if opts.UpDepth > 0 {
		visited := make(map[string]bool)
		tree.Callers = t.buildCallersTree(ctx, item, opts, 0, visited, collected, &maxUpDepth, &totalCallers)
	}

	// Build callees tree (down)
	if opts.DownDepth > 0 {
		visited := make(map[string]bool)
		tree.Calls = t.buildCalleesTree(ctx, item, opts, 0, visited, collected, &maxDownDepth, &totalCallees)
	}

	// Group collected functions by package
	packages := groupByPackage(collected)

	return &output.TreeResponse{
		Query: output.TreeQuery{
			Command: "tree",
			Target:  symbol,
			Up:      opts.UpDepth,
			Down:    opts.DownDepth,
		},
		Tree:     tree,
		Packages: packages,
		Summary: output.TreeSummary{
			Callers:       totalCallers,
			Callees:       totalCallees,
			MaxUpDepth:    maxUpDepth,
			MaxDownDepth:  maxDownDepth,
			UpTruncated:   opts.UpDepth > 0 && maxUpDepth >= opts.UpDepth,
			DownTruncated: opts.DownDepth > 0 && maxDownDepth >= opts.DownDepth,
		},
	}, nil
}

// groupByPackage converts collected functions into package-grouped output.
func groupByPackage(collected map[string]*collectedFunc) []output.TreePackage {
	// Group by import path
	pkgMap := make(map[string][]output.TreeFunction)
	pkgDirs := make(map[string]string)

	for _, cf := range collected {
		if _, exists := pkgMap[cf.importPath]; !exists {
			// Get dir from file path
			dir := cf.file
			if idx := strings.LastIndex(cf.file, "/"); idx >= 0 {
				dir = cf.file[:idx]
			}
			pkgDirs[cf.importPath] = dir
		}

		// Build qualified symbol name: pkg.Name or pkg.Type.Method
		symbol := cf.pkgName + "." + cf.name

		pkgMap[cf.importPath] = append(pkgMap[cf.importPath], output.TreeFunction{
			Symbol:     symbol,
			Signature:  cf.signature,
			Definition: fmt.Sprintf("%s:%d:%d", cf.file, cf.startLine, cf.endLine),
		})
	}

	// Sort packages alphabetically for deterministic output
	pkgOrder := make([]string, 0, len(pkgMap))
	for pkg := range pkgMap {
		pkgOrder = append(pkgOrder, pkg)
	}
	sort.Strings(pkgOrder)

	// Build output in sorted order
	var packages []output.TreePackage
	for _, pkg := range pkgOrder {
		// Sort symbols within each package
		symbols := pkgMap[pkg]
		sort.Slice(symbols, func(i, j int) bool {
			return symbols[i].Symbol < symbols[j].Symbol
		})

		packages = append(packages, output.TreePackage{
			Package: pkg,
			Dir:     pkgDirs[pkg],
			Symbols: symbols,
		})
	}
	return packages
}

// buildCallersTree builds a list of caller nodes for the given item.
// Returns the direct callers, each with their own Callers populated recursively.
func (t *Traverser) buildCallersTree(ctx context.Context, item lsp.CallHierarchyItem, opts Options, depth int, visited map[string]bool, collected map[string]*collectedFunc, maxDepth *int, totalCalls *int) []*output.TreeNode {
	if depth > *maxDepth {
		*maxDepth = depth
	}

	// Check depth limit
	if depth >= opts.UpDepth {
		return nil
	}

	// Check for cycles
	key := item.URI + ":" + item.Name
	if visited[key] {
		return nil
	}
	visited[key] = true
	defer func() { visited[key] = false }()

	// Get callers
	calls, err := t.client.IncomingCalls(ctx, item)
	if err != nil || len(calls) == 0 {
		return nil
	}

	var callers []*output.TreeNode
	for _, call := range calls {
		callFile := lsp.URIToPath(call.From.URI)

		if opts.ExcludeTests && output.IsTestFile(callFile) {
			continue
		}
		if !t.inScope(call.From.URI, opts) {
			continue
		}

		*totalCalls++

		// Record function info
		t.collectFunc(call.From, callFile, collected)

		// Get call site location (where caller calls current item)
		callLocation := ""
		if len(call.FromRanges) > 0 {
			callLine := call.FromRanges[0].Start.Line + 1
			callLocation = fmt.Sprintf("%s:%d", callFile, callLine)
		}

		callerNode := &output.TreeNode{
			Symbol:   symbolName(call.From),
			Location: callLocation,
		}

		// Recurse to get this caller's callers
		callerNode.Callers = t.buildCallersTree(ctx, call.From, opts, depth+1, visited, collected, maxDepth, totalCalls)

		callers = append(callers, callerNode)
	}

	return callers
}

// buildCalleesTree builds a list of callee nodes for the given item.
// Returns the direct callees, each with their own Calls populated recursively.
func (t *Traverser) buildCalleesTree(ctx context.Context, item lsp.CallHierarchyItem, opts Options, depth int, visited map[string]bool, collected map[string]*collectedFunc, maxDepth *int, totalCalls *int) []*output.TreeNode {
	file := lsp.URIToPath(item.URI)

	if depth > *maxDepth {
		*maxDepth = depth
	}

	// Check depth limit
	if depth >= opts.DownDepth {
		return nil
	}

	// Check for cycles
	key := item.URI + ":" + item.Name
	if visited[key] {
		return nil
	}
	visited[key] = true
	defer func() { visited[key] = false }()

	// Get callees
	calls, err := t.client.OutgoingCalls(ctx, item)
	if err != nil || len(calls) == 0 {
		return nil
	}

	var callees []*output.TreeNode
	for _, call := range calls {
		calleeFile := lsp.URIToPath(call.To.URI)

		if opts.ExcludeTests && output.IsTestFile(calleeFile) {
			continue
		}
		if !t.inScope(call.To.URI, opts) {
			continue
		}

		*totalCalls++

		// Record function info
		t.collectFunc(call.To, calleeFile, collected)

		// Get call site location (where current item calls callee - in current item's file)
		callLocation := ""
		if len(call.FromRanges) > 0 {
			callLine := call.FromRanges[0].Start.Line + 1
			callLocation = fmt.Sprintf("%s:%d", file, callLine)
		}

		calleeNode := &output.TreeNode{
			Symbol:   symbolName(call.To),
			Location: callLocation,
		}

		// Recurse to get this callee's callees
		calleeNode.Calls = t.buildCalleesTree(ctx, call.To, opts, depth+1, visited, collected, maxDepth, totalCalls)

		callees = append(callees, calleeNode)
	}

	return callees
}

// collectFunc records function info for the packages output.
func (t *Traverser) collectFunc(item lsp.CallHierarchyItem, file string, collected map[string]*collectedFunc) {
	line := item.Range.Start.Line + 1
	info := extractFuncInfo(file, line)

	// Build the short name - use Type.Method for methods, just Name for functions
	shortName := item.Name
	if info.receiverType != "" {
		shortName = info.receiverType + "." + item.Name
	}

	// Build qualified name for deduplication
	pkgName := packageFromPath(file)
	qualifiedName := pkgName + "." + shortName

	if _, exists := collected[qualifiedName]; exists {
		return
	}

	sig := info.signature
	if sig == "" {
		sig = item.Name
	}
	startLine, endLine := info.startLine, info.endLine
	if startLine == 0 {
		startLine = line
		endLine = item.Range.End.Line + 1
	}

	// Get import path from file path
	dir := file
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		dir = file[:idx]
	}
	importPath, _ := golang.ResolvePackagePath(dir, dir)
	if importPath == "" {
		importPath = pkgName
	}

	collected[qualifiedName] = &collectedFunc{
		name:       shortName, // Now includes Type.Method for methods
		pkgName:    pkgName,
		file:       file,
		importPath: importPath,
		signature:  sig,
		startLine:  startLine,
		endLine:    endLine,
	}
}

// funcInfo holds extracted function information.
type funcInfo struct {
	signature    string
	startLine    int
	endLine      int
	receiverType string // e.g., "Server" for method (s *Server) Conn()
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
			info := funcInfo{
				signature: renderFuncSignature(fn),
				startLine: pos.Line,
				endLine:   endPos.Line,
			}
			// Extract receiver type for methods
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				info.receiverType = extractReceiverType(fn.Recv.List[0].Type)
			}
			return info
		}
	}

	return funcInfo{}
}

// extractReceiverType extracts the base type name from a receiver expression.
// Handles both value receivers (T) and pointer receivers (*T).
func extractReceiverType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
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
