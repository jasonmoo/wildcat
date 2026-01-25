package tree_cmd

import (
	"context"
	"fmt"
	"go/ast"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/spf13/cobra"
)

type TreeCommand struct {
	symbol      string
	upDepth     int
	downDepth   int
	scope       string
	scopeFilter *commands.ScopeFilter
}

var _ commands.Command[*TreeCommand] = (*TreeCommand)(nil)

func WithSymbol(s string) func(*TreeCommand) error {
	return func(c *TreeCommand) error {
		c.symbol = s
		return nil
	}
}

func WithUpDepth(d int) func(*TreeCommand) error {
	return func(c *TreeCommand) error {
		c.upDepth = d
		return nil
	}
}

func WithDownDepth(d int) func(*TreeCommand) error {
	return func(c *TreeCommand) error {
		c.downDepth = d
		return nil
	}
}

func WithScope(s string) func(*TreeCommand) error {
	return func(c *TreeCommand) error {
		c.scope = s
		return nil
	}
}

func NewTreeCommand() *TreeCommand {
	return &TreeCommand{
		upDepth:   2,
		downDepth: 2,
		scope:     "project",
	}
}

func (c *TreeCommand) Cmd() *cobra.Command {
	var upDepth, downDepth int
	var scope string

	cmd := &cobra.Command{
		Use:   "tree <symbol>",
		Short: "Build a call tree centered on a symbol",
		Long: `Build a call tree showing callers and callees of a symbol.

Note: tree operates on functions and methods only.

The symbol is the center point of the tree:
  --up N    Show N levels of callers (what calls this function)
  --down N  Show N levels of callees (what this function calls)

By default, shows 2 levels in each direction.

Scope (filters output, not traversal):
  project           - All project packages (default)
  all               - Include dependencies and stdlib
  pkg1,pkg2         - Specific packages (comma-separated)
  -pkg              - Exclude package (prefix with -)

Pattern syntax:
  internal/lsp      - Exact package match
  internal/...      - Package and all subpackages (Go-style)
  internal/*        - Direct children only
  internal/**       - All descendants
  **/util           - Match anywhere in path

Full call graph is traversed; scope controls which calls appear in
output. Out-of-scope intermediate calls are elided with "...".

Examples:
  wildcat tree main.main
  wildcat tree db.Query --up 3 --down 1
  wildcat tree Server.Start --up 0 --down 4
  wildcat tree Handler.ServeHTTP --scope all
  wildcat tree Execute --scope 'internal/commands/...'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.RunCommand(cmd, c,
				WithSymbol(args[0]),
				WithUpDepth(upDepth),
				WithDownDepth(downDepth),
				WithScope(scope),
			)
		},
	}

	cmd.Flags().IntVar(&upDepth, "up", 2, "Depth of callers to show (0 to skip)")
	cmd.Flags().IntVar(&downDepth, "down", 2, "Depth of callees to show (0 to skip)")
	cmd.Flags().StringVar(&scope, "scope", "project", "Filter output to packages (patterns: internal/..., **/util, -excluded)")

	return cmd
}

func (c *TreeCommand) README() string {
	return "TODO"
}

func (c *TreeCommand) Execute(ctx context.Context, wc *commands.Wildcat, opts ...func(*TreeCommand) error) (commands.Result, error) {

	if c.symbol == "" {
		return commands.NewErrorResultf("invalid_symbol", "symbol is required"), nil
	}

	// Find target symbol
	matches := wc.Index.Lookup(c.symbol)
	if len(matches) == 0 {
		return wc.NewFuncNotFoundErrorResponse(c.symbol), nil
	}
	if len(matches) > 1 {
		return wc.NewSymbolAmbiguousErrorResponse(c.symbol, matches), nil
	}
	target := matches[0]

	if target.Kind != golang.SymbolKindFunc && target.Kind != golang.SymbolKindMethod {
		return commands.NewErrorResultf("invalid_symbol_kind", "tree requires a function or method, got %s", target.Kind), nil
	}

	// Parse scope filter (target package always included)
	scopeFilter, err := wc.ParseScope(ctx, c.scope, target.PackageIdentifier.PkgPath)
	if err != nil {
		return commands.NewErrorResultf("invalid_scope", "invalid scope: %s", err), nil
	}
	c.scopeFilter = scopeFilter

	funcDecl, ok := target.Node.(*ast.FuncDecl)
	if !ok || funcDecl.Body == nil {
		return commands.NewErrorResultf("invalid_symbol", "cannot analyze %q: no function body", c.symbol), nil
	}

	// Build target info
	sig := target.Signature()
	definition := target.PathDefinition()

	// Track all functions for definitions section
	collected := make(map[string]*collectedFunc)
	collectFromSymbol(target, collected)

	// Map symbol names to full package paths for scope filtering
	symbolPkgPaths := make(map[string]string)
	symbolPkgPaths[target.PkgSymbol()] = target.PackageIdentifier.PkgPath

	// Look up the golang.Package for the target
	targetPkg, err := wc.Package(target.PackageIdentifier)
	if err != nil {
		return commands.NewErrorResultf("package_not_found", "package not found: %s", err), nil
	}

	var callers, callees []*output.CallNode
	var maxUpDepth, maxDownDepth int
	var totalCallers, totalCallees int

	// Build callers (--up)
	if c.upDepth > 0 {
		visited := make(map[string]bool)
		callersBottomUp := c.buildCallersTree(wc, targetPkg, funcDecl, 0, visited, collected, symbolPkgPaths, &maxUpDepth, &totalCallers)
		callers = invertCallersTree(callersBottomUp, target.PkgSymbol())
	}

	// Build callees (--down)
	if c.downDepth > 0 {
		visited := make(map[string]bool)
		callees = c.buildCalleesTree(wc, targetPkg, funcDecl, 0, visited, collected, symbolPkgPaths, &maxDownDepth, &totalCallees)
	}

	// Apply scope filtering with elision
	callers = c.elideOutOfScope(callers, symbolPkgPaths)
	callees = c.elideOutOfScope(callees, symbolPkgPaths)

	return &TreeCommandResponse{
		Query: output.TreeQuery{
			Command: "tree",
			Target:  target.PkgSymbol(),
			Up:      c.upDepth,
			Down:    c.downDepth,
			Scope:   string(c.scope),
		},
		Target: output.TreeTargetInfo{
			Symbol:     target.PkgSymbol(),
			Signature:  sig,
			Definition: definition,
		},
		Callers:     callers,
		Calls:       callees,
		Definitions: groupByPackage(collected),
		Summary: output.TreeSummary{
			Callers:       totalCallers,
			Callees:       totalCallees,
			MaxUpDepth:    maxUpDepth,
			MaxDownDepth:  maxDownDepth,
			UpTruncated:   c.upDepth > 0 && maxUpDepth >= c.upDepth,
			DownTruncated: c.downDepth > 0 && maxDownDepth >= c.downDepth,
		},
	}, nil
}

func (c *TreeCommand) buildCalleesTree(
	wc *commands.Wildcat,
	pkg *golang.Package,
	fn *ast.FuncDecl,
	depth int,
	visited map[string]bool,
	collected map[string]*collectedFunc,
	symbolPkgPaths map[string]string,
	maxDepth *int,
	totalCalls *int,
) []*output.CallNode {
	if depth > *maxDepth {
		*maxDepth = depth
	}
	if depth >= c.downDepth {
		return nil
	}

	key := pkg.Identifier.PkgPath + ":" + fn.Name.Name
	if visited[key] {
		return nil
	}
	visited[key] = true
	defer delete(visited, key)

	if fn.Body == nil {
		return nil
	}

	var nodes []*output.CallNode

	golang.WalkCallsInFunc(pkg, fn, func(call golang.Call) bool {
		if call.Called == nil || call.Called.Pkg() == nil {
			return true
		}

		// Find the callee's AST (only works for project packages)
		calleeInfo := golang.FindFuncInfo(wc.Project.Packages, call.Called)

		*totalCalls++

		// Collect function info
		if calleeInfo != nil {
			collectFromFuncInfo(calleeInfo, collected)
		}

		callsite := fmt.Sprintf("%s:%d", call.CallerFile, call.Line)
		symbolName := call.CalledName()

		// Track package path for scope filtering
		symbolPkgPaths[symbolName] = call.Called.Pkg().Path()

		node := &output.CallNode{
			Symbol:   symbolName,
			Callsite: callsite,
		}

		// Recurse if we have the AST
		if calleeInfo != nil && calleeInfo.Decl.Body != nil {
			node.Calls = c.buildCalleesTree(wc, calleeInfo.Pkg, calleeInfo.Decl, depth+1, visited, collected, symbolPkgPaths, maxDepth, totalCalls)
		}

		nodes = append(nodes, node)
		return true
	})

	return nodes
}

func (c *TreeCommand) buildCallersTree(
	wc *commands.Wildcat,
	targetPkg *golang.Package,
	targetFn *ast.FuncDecl,
	depth int,
	visited map[string]bool,
	collected map[string]*collectedFunc,
	symbolPkgPaths map[string]string,
	maxDepth *int,
	totalCalls *int,
) []*output.CallNode {
	if depth > *maxDepth {
		*maxDepth = depth
	}
	if depth >= c.upDepth {
		return nil
	}

	key := targetPkg.Identifier.PkgPath + ":" + targetFn.Name.Name
	if visited[key] {
		return nil
	}
	visited[key] = true
	defer delete(visited, key)

	// Get target's types.Func for comparison
	targetObj := targetPkg.Package.TypesInfo.Defs[targetFn.Name]
	if targetObj == nil {
		// Return an error node so AI knows analysis couldn't continue here
		return []*output.CallNode{{
			Symbol: targetPkg.Identifier.Name + "." + targetFn.Name.Name,
			Error:  "type info unavailable, callers analysis incomplete",
		}}
	}

	var callers []*output.CallNode

	golang.WalkCalls(wc.Project.Packages, func(call golang.Call) bool {
		// Check if this call targets our function
		if call.Called == nil || call.Called != targetObj {
			return true
		}

		*totalCalls++

		// Build caller info
		callerInfo := &golang.FuncInfo{
			Decl:     call.Caller,
			Pkg:      call.Package,
			Filename: call.CallerFile,
		}
		if call.Caller.Recv != nil && len(call.Caller.Recv.List) > 0 {
			callerInfo.Receiver = golang.ReceiverTypeName(call.Caller.Recv.List[0].Type)
		}

		collectFromFuncInfo(callerInfo, collected)

		callsite := fmt.Sprintf("%s:%d", call.CallerFile, call.Line)
		symbolName := call.CallerName()

		// Track package path for scope filtering
		symbolPkgPaths[symbolName] = call.Package.Identifier.PkgPath

		callerNode := &output.CallNode{
			Symbol:   symbolName,
			Callsite: callsite,
			Calls:    c.buildCallersTree(wc, call.Package, call.Caller, depth+1, visited, collected, symbolPkgPaths, maxDepth, totalCalls),
		}

		callers = append(callers, callerNode)
		return true
	})

	return callers
}

// elideOutOfScope filters the call tree to only show in-scope nodes,
// collapsing consecutive out-of-scope nodes into "..." elision markers.
func (c *TreeCommand) elideOutOfScope(nodes []*output.CallNode, pkgPaths map[string]string) []*output.CallNode {
	if c.scopeFilter == nil {
		return nodes
	}

	var result []*output.CallNode
	for _, node := range nodes {
		pkgPath := pkgPaths[node.Symbol]
		if c.scopeFilter.InScope(pkgPath) {
			// In scope: keep node, recurse on children
			filtered := *node
			filtered.Calls = c.elideOutOfScope(node.Calls, pkgPaths)
			result = append(result, &filtered)
		} else {
			// Out of scope: find in-scope descendants and elide
			descendants := c.findInScopeDescendants(node.Calls, pkgPaths)
			if len(descendants) > 0 {
				// Create elision node
				elision := &output.CallNode{
					Symbol: "...",
					Calls:  descendants,
				}
				result = append(result, elision)
			}
		}
	}
	return result
}

// findInScopeDescendants recursively finds all in-scope nodes,
// collapsing out-of-scope intermediate nodes.
func (c *TreeCommand) findInScopeDescendants(nodes []*output.CallNode, pkgPaths map[string]string) []*output.CallNode {
	var result []*output.CallNode
	for _, node := range nodes {
		pkgPath := pkgPaths[node.Symbol]
		if c.scopeFilter.InScope(pkgPath) {
			// In scope: include with filtered children
			filtered := *node
			filtered.Calls = c.elideOutOfScope(node.Calls, pkgPaths)
			result = append(result, &filtered)
		} else {
			// Out of scope: recurse to find in-scope descendants
			result = append(result, c.findInScopeDescendants(node.Calls, pkgPaths)...)
		}
	}
	return result
}

// collectedFunc holds function data for definitions section
type collectedFunc struct {
	name       string
	pkgIdent   *golang.PackageIdentifier
	signature  string
	definition string
}

func collectFromSymbol(sym *golang.Symbol, collected map[string]*collectedFunc) {
	if _, ok := collected[sym.PkgPathSymbol()]; ok {
		return
	}
	collected[sym.PkgPathSymbol()] = &collectedFunc{
		name:       sym.Name,
		pkgIdent:   sym.PackageIdentifier,
		signature:  sym.Signature(),
		definition: sym.PathDefinition(),
	}
}

func collectFromFuncInfo(info *golang.FuncInfo, collected map[string]*collectedFunc) {
	name := info.Decl.Name.Name
	if info.Receiver != "" {
		name = info.Receiver + "." + name
	}
	key := info.Pkg.Identifier.PkgPath + "." + name
	if _, ok := collected[key]; ok {
		return
	}

	start := info.Pkg.Package.Fset.Position(info.Decl.Pos())
	end := info.Pkg.Package.Fset.Position(info.Decl.End())

	collected[key] = &collectedFunc{
		name:       name,
		pkgIdent:   info.Pkg.Identifier,
		signature:  golang.FormatNode(info.Decl),
		definition: fmt.Sprintf("%s:%d:%d", start.Filename, start.Line, end.Line),
	}
}

func groupByPackage(collected map[string]*collectedFunc) []output.TreePackage {
	type pkgData struct {
		dir     string
		symbols []output.TreeFunction
	}
	pkgMap := make(map[string]*pkgData)

	for _, cf := range collected {
		sym := cf.pkgIdent.Name + "." + cf.name
		fn := output.TreeFunction{
			Symbol:     sym,
			Signature:  cf.signature,
			Definition: cf.definition,
		}

		if pd, ok := pkgMap[cf.pkgIdent.PkgPath]; ok {
			pd.symbols = append(pd.symbols, fn)
		} else {
			pkgMap[cf.pkgIdent.PkgPath] = &pkgData{
				dir:     cf.pkgIdent.PkgDir,
				symbols: []output.TreeFunction{fn},
			}
		}
	}

	var packages []output.TreePackage
	for pkgPath, pd := range pkgMap {
		packages = append(packages, output.TreePackage{
			Package: pkgPath,
			Dir:     pd.dir,
			Symbols: pd.symbols,
		})
	}
	return packages
}

func invertCallersTree(bottomUp []*output.CallNode, targetSymbol string) []*output.CallNode {
	if len(bottomUp) == 0 {
		return nil
	}

	var result []*output.CallNode
	for _, node := range bottomUp {
		callSiteLocation := node.Callsite

		if len(node.Calls) == 0 {
			inverted := &output.CallNode{
				Symbol: node.Symbol,
				Calls: []*output.CallNode{
					{Symbol: targetSymbol, Callsite: callSiteLocation},
				},
			}
			result = append(result, inverted)
		} else {
			invertedCallers := invertCallersTree(node.Calls, node.Symbol)
			for _, caller := range invertedCallers {
				addAsLeaf(caller, &output.CallNode{Symbol: targetSymbol, Callsite: callSiteLocation})
			}
			result = append(result, invertedCallers...)
		}
	}
	return result
}

func addAsLeaf(parent, child *output.CallNode) {
	if len(parent.Calls) == 0 {
		parent.Calls = []*output.CallNode{child}
	} else {
		for _, call := range parent.Calls {
			addAsLeaf(call, child)
		}
	}
}
