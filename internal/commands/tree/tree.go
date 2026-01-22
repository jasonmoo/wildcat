package tree_cmd

import (
	"context"
	"fmt"
	"go/ast"
	"os"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/spf13/cobra"
)

type Scope string

const (
	ScopeAll     Scope = "all"
	ScopeProject Scope = "project"
	ScopePackage Scope = "package"
)

type TreeCommand struct {
	symbol        string
	upDepth       int
	downDepth     int
	scope         Scope
	targetPkgPath string // set after symbol resolution, used for ScopePackage
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

func WithScope(s Scope) func(*TreeCommand) error {
	return func(c *TreeCommand) error {
		c.scope = s
		return nil
	}
}

func NewTreeCommand() *TreeCommand {
	return &TreeCommand{
		upDepth:   2,
		downDepth: 2,
		scope:     ScopeProject,
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

Scope:
  all     - Include everything (stdlib, dependencies)
  project - Project packages only (default)
  package - Same package as starting symbol only

Examples:
  wildcat tree main.main
  wildcat tree db.Query --up 3 --down 1
  wildcat tree Server.Start --up 0 --down 4
  wildcat tree Handler.ServeHTTP --scope all`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wc, err := commands.LoadWildcat(cmd.Context(), ".")
			if err != nil {
				return err
			}

			result, err := c.Execute(cmd.Context(), wc,
				WithSymbol(args[0]),
				WithUpDepth(upDepth),
				WithDownDepth(downDepth),
				WithScope(Scope(scope)),
			)
			if err != nil {
				return err
			}

			if outputFlag := cmd.Flag("output"); outputFlag != nil && outputFlag.Changed && outputFlag.Value.String() == "json" {
				data, err := result.MarshalJSON()
				if err != nil {
					return err
				}
				os.Stdout.Write(data)
				os.Stdout.WriteString("\n")
				return nil
			}

			md, err := result.MarshalMarkdown()
			if err != nil {
				return err
			}
			os.Stdout.Write(md)
			os.Stdout.WriteString("\n")
			return nil
		},
	}

	cmd.Flags().IntVar(&upDepth, "up", 2, "Depth of callers to show (0 to skip)")
	cmd.Flags().IntVar(&downDepth, "down", 2, "Depth of callees to show (0 to skip)")
	cmd.Flags().StringVar(&scope, "scope", "project", "Traversal scope: all, project, package")

	return cmd
}

func (c *TreeCommand) README() string {
	return "TODO"
}

func (c *TreeCommand) Execute(ctx context.Context, wc *commands.Wildcat, opts ...func(*TreeCommand) error) (commands.Result, error) {
	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, fmt.Errorf("interal_error: failed to apply opt: %w", err)
		}
	}

	if c.symbol == "" {
		return commands.NewErrorResultf("invalid_symbol", "symbol is required"), nil
	}

	// Find target symbol
	target := wc.Index.Lookup(c.symbol)
	if target == nil {
		return wc.NewFuncNotFoundErrorResponse(c.symbol), nil
	}

	if target.Kind != golang.SymbolKindFunc && target.Kind != golang.SymbolKindMethod {
		return commands.NewErrorResultf("invalid_symbol_kind", "tree requires a function or method, got %s", target.Kind), nil
	}

	// Store target package for ScopePackage filtering
	c.targetPkgPath = target.Package.Identifier.PkgPath

	funcDecl, ok := target.Node().(*ast.FuncDecl)
	if !ok || funcDecl.Body == nil {
		return commands.NewErrorResultf("invalid_symbol", "cannot analyze %q: no function body", c.symbol), nil
	}

	// Build target info
	sig, _ := target.Signature()
	definition := fmt.Sprintf("%s:%s", target.Filename(), target.Location())
	qualifiedSymbol := target.Package.Identifier.Name + "." + target.Name

	// Track all functions for definitions section
	collected := make(map[string]*collectedFunc)
	collectFromSymbol(target, collected)

	var callers, callees []*output.CallNode
	var maxUpDepth, maxDownDepth int
	var totalCallers, totalCallees int

	// Build callers (--up)
	if c.upDepth > 0 {
		visited := make(map[string]bool)
		callersBottomUp := c.buildCallersTree(wc, target.Package, funcDecl, 0, visited, collected, &maxUpDepth, &totalCallers)
		callers = invertCallersTree(callersBottomUp, qualifiedSymbol)
	}

	// Build callees (--down)
	if c.downDepth > 0 {
		visited := make(map[string]bool)
		callees = c.buildCalleesTree(wc, target.Package, funcDecl, 0, visited, collected, &maxDownDepth, &totalCallees)
	}

	return &TreeCommandResponse{
		Query: output.TreeQuery{
			Command: "tree",
			Target:  qualifiedSymbol,
			Up:      c.upDepth,
			Down:    c.downDepth,
			Scope:   string(c.scope),
		},
		Target: output.TreeTargetInfo{
			Symbol:     qualifiedSymbol,
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

		if !c.inScope(call.Called.Pkg().Path(), wc) {
			return true
		}

		// Find the callee's AST
		calleeInfo := golang.FindFuncInfo(wc.Project.Packages, call.Called)

		*totalCalls++

		// Collect function info
		if calleeInfo != nil {
			collectFromFuncInfo(calleeInfo, collected)
		}

		callsite := fmt.Sprintf("%s:%d", call.CallerFile, call.Line)

		node := &output.CallNode{
			Symbol:   call.CalledName(),
			Callsite: callsite,
		}

		// Recurse if we have the AST
		if calleeInfo != nil && calleeInfo.Decl.Body != nil {
			node.Calls = c.buildCalleesTree(wc, calleeInfo.Pkg, calleeInfo.Decl, depth+1, visited, collected, maxDepth, totalCalls)
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
		return nil
	}

	var callers []*output.CallNode

	golang.WalkCalls(wc.Project.Packages, func(call golang.Call) bool {
		// Scope filter
		if !c.inScope(call.Package.Identifier.PkgPath, wc) {
			return true
		}

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

		callerNode := &output.CallNode{
			Symbol:   call.CallerName(),
			Callsite: callsite,
			Calls:    c.buildCallersTree(wc, call.Package, call.Caller, depth+1, visited, collected, maxDepth, totalCalls),
		}

		callers = append(callers, callerNode)
		return true
	})

	return callers
}

func (c *TreeCommand) inScope(pkgPath string, wc *commands.Wildcat) bool {
	switch c.scope {
	case ScopeAll:
		return true
	case ScopeProject:
		return strings.HasPrefix(pkgPath, wc.Project.Module.Path)
	case ScopePackage:
		return pkgPath == c.targetPkgPath
	}
	return false
}

// collectedFunc holds function data for definitions section
type collectedFunc struct {
	name       string
	pkg        *golang.Package
	signature  string
	definition string
}

func collectFromSymbol(sym *golang.Symbol, collected map[string]*collectedFunc) {
	key := sym.Package.Identifier.PkgPath + "." + sym.Name
	if _, ok := collected[key]; ok {
		return
	}
	sig, _ := sym.Signature()
	collected[key] = &collectedFunc{
		name:       sym.Name,
		pkg:        sym.Package,
		signature:  sig,
		definition: fmt.Sprintf("%s:%s", sym.Filename(), sym.Location()),
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

	sig, _ := golang.FormatFuncDecl(info.Decl)
	start := info.Pkg.Package.Fset.Position(info.Decl.Pos())
	end := info.Pkg.Package.Fset.Position(info.Decl.End())

	collected[key] = &collectedFunc{
		name:       name,
		pkg:        info.Pkg,
		signature:  sig,
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
		sym := cf.pkg.Identifier.Name + "." + cf.name
		fn := output.TreeFunction{
			Symbol:     sym,
			Signature:  cf.signature,
			Definition: cf.definition,
		}

		if pd, ok := pkgMap[cf.pkg.Identifier.PkgPath]; ok {
			pd.symbols = append(pd.symbols, fn)
		} else {
			pkgMap[cf.pkg.Identifier.PkgPath] = &pkgData{
				dir:     cf.pkg.Identifier.PkgDir,
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
