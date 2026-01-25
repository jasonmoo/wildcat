package deadcode_cmd

import (
	"context"
	"fmt"
	"go/ast"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/spf13/cobra"
)

type DeadcodeCommand struct {
	scope           string // scope filter for packages (default: project)
	includeExported bool   // include exported symbols (may have external callers)
}

var _ commands.Command[*DeadcodeCommand] = (*DeadcodeCommand)(nil)

func WithScope(scope string) func(*DeadcodeCommand) error {
	return func(c *DeadcodeCommand) error {
		c.scope = scope
		return nil
	}
}

func WithIncludeExported(include bool) func(*DeadcodeCommand) error {
	return func(c *DeadcodeCommand) error {
		c.includeExported = include
		return nil
	}
}

func NewDeadcodeCommand() *DeadcodeCommand {
	return &DeadcodeCommand{
		scope: "project",
	}
}

func (c *DeadcodeCommand) Cmd() *cobra.Command {
	var includeExported bool
	var scope string

	cmd := &cobra.Command{
		Use:   "deadcode",
		Short: "Find unreachable code using static analysis",
		Long: `Find functions and methods not reachable from entry points.

Uses Rapid Type Analysis (RTA) to determine which code is actually
reachable from main() and init() functions. This catches transitively
dead code that simple reference counting misses.

For executables (has main): Reports all unreachable code.
For libraries (no main): Uses exported functions as roots and reports
only unreachable unexported code. Exported symbols are excluded since
they may have external callers.

Scope (filters output, not analysis):
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

Full project is analyzed for reachability; scope controls which dead
symbols appear in output. A symbol is only dead if unreachable from
the entire project, not just the scoped packages.

Flags:
  --scope           Package scope filter (default: project)
  --include-exported Include exported symbols (libraries only)

Examples:
  wildcat deadcode                                    # find all dead code
  wildcat deadcode --scope internal/lsp              # dead code in lsp package
  wildcat deadcode --scope 'internal/...'            # dead code in internal subtree
  wildcat deadcode --scope 'internal/...,-**/test'   # exclude test packages
  wildcat deadcode --include-exported                 # include exported API`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.RunCommand(cmd, c,
				WithScope(scope),
				WithIncludeExported(includeExported),
			)
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "project", "Filter output to packages (patterns: internal/..., **/util, -excluded)")
	cmd.Flags().BoolVar(&includeExported, "include-exported", false, "Include exported symbols (for libraries without main)")

	return cmd
}

func (c *DeadcodeCommand) README() string {
	return "TODO"
}

func (c *DeadcodeCommand) Execute(ctx context.Context, wc *commands.Wildcat, opts ...func(*DeadcodeCommand) error) (commands.Result, error) {

	// Parse scope filter
	scopeFilter, err := wc.ParseScope(ctx, c.scope, ".")
	if err != nil {
		return commands.NewErrorResultf("invalid_scope", "invalid scope: %s", err), nil
	}

	// Run RTA analysis
	analysis, err := golang.AnalyzeDeadCode(wc.Project)
	if err != nil {
		return nil, fmt.Errorf("dead code analysis failed: %w", err)
	}

	// Emit diagnostic if no entry points (library mode)
	if !analysis.HasEntryPoints {
		wc.Diagnostics = append(wc.Diagnostics, commands.NewInfoDiagnostic(nil, "no main/init entry points found; using exported functions as roots, exported symbols excluded from results"))
	}

	// Track file info per package (total symbols, dead symbols)
	type fileStats struct {
		total int
		dead  int
	}
	type pkgStats struct {
		dir   string
		files map[string]*fileStats
	}
	stats := make(map[string]*pkgStats) // pkgPath -> stats

	// Track total methods per type for dead method grouping
	totalMethodsByType := make(map[string]int) // "pkg.Type" -> count

	// Collect dead symbols grouped by package
	var packages []*PackageDeadCode

	// Temporary storage for grouping
	type tempDeadSymbol struct {
		ds         DeadSymbol
		parentType string // for methods/constructors
	}
	deadByPkg := make(map[string][]tempDeadSymbol)

	var totalDeadSymbols int

	for _, sym := range wc.Index.Symbols() {
		// Filter by scope
		if !scopeFilter.InScope(sym.PackageIdentifier.PkgPath) {
			continue
		}

		// Skip exported symbols we can't reason about (internal/ exports are always included)
		if ast.IsExported(sym.Name) && !sym.PackageIdentifier.IsInternal() {
			// In library mode: always skip exported (can't reason about external callers)
			// In executable mode: skip unless --include-exported
			if !analysis.HasEntryPoints || !c.includeExported {
				continue
			}
		}

		// Skip special functions (entry points)
		if isEntryPoint(sym) {
			continue
		}

		// Skip blank identifiers
		if sym.Name == "_" {
			continue
		}

		// Track file stats
		pkgPath := sym.PackageIdentifier.PkgPath
		pos := sym.Package.Fset.Position(sym.Node.Pos())
		filename := filepath.Base(pos.Filename)

		if stats[pkgPath] == nil {
			stats[pkgPath] = &pkgStats{
				dir:   sym.PackageIdentifier.PkgDir,
				files: make(map[string]*fileStats),
			}
		}
		if stats[pkgPath].files[filename] == nil {
			stats[pkgPath].files[filename] = &fileStats{}
		}
		stats[pkgPath].files[filename].total++

		// Count methods by type for grouping
		var parentType string
		if sym.Kind == golang.SymbolKindMethod {
			if node, ok := sym.Node.(*ast.FuncDecl); ok {
				if node.Recv != nil && len(node.Recv.List) > 0 {
					parentType = sym.PackageIdentifier.Name + "." + golang.ReceiverTypeName(node.Recv.List[0].Type)
					totalMethodsByType[parentType]++
				}
			}
		}

		// Check if reachable via SSA
		reachable, analyzed := analysis.IsReachable(sym)
		if !analyzed {
			// Couldn't analyze this symbol - add diagnostic and skip
			wc.Diagnostics = append(wc.Diagnostics, commands.NewWarningDiagnostic(sym.PackageIdentifier, fmt.Sprintf("could not analyze reachability for %s (position invalid or analysis incomplete)", sym.Name)))
			continue
		}
		if reachable {
			continue
		}

		// Check if has non-call references (escaping function value).
		// Functions passed to external code (e.g., cobra handlers) may not be
		// traceable via SSA but are still used.
		if golang.CountNonCallReferences(wc.Project.Packages, sym) > 0 {
			continue
		}

		// Check if interface has implementations in the project.
		// Interfaces with implementations are not dead - the implementations depend on the interface definition.
		if sym.Kind == golang.SymbolKindInterface {
			if iface := golang.GetInterfaceType(sym); iface != nil {
				implementors := golang.FindImplementors(iface, sym.PackageIdentifier.PkgPath, sym.Name, wc.Project.Packages)
				if len(implementors) > 0 {
					continue
				}
			}
		}

		// Check if method implements an interface.
		// Interface methods are required if the type is used, even if never called directly.
		if sym.Kind == golang.SymbolKindMethod && golang.IsInterfaceMethod(sym, wc.Project, wc.Stdlib) {
			// Check if the receiver type is used (has any references)
			typeSym := findReceiverTypeSymbol(wc, sym)
			if typeSym != nil && golang.CountReferences(wc.Project.Packages, typeSym).Total() > 0 {
				continue
			}
		}

		// Update dead count
		stats[pkgPath].files[filename].dead++
		totalDeadSymbols++

		// Build dead symbol info
		ds := DeadSymbol{
			Symbol:     sym.PackageIdentifier.Name + "." + sym.Name,
			Kind:       string(sym.Kind),
			Signature:  sym.Signature(),
			Definition: fmt.Sprintf("%s:%d", filename, pos.Line),
		}

		// Get parent type for methods and constructors
		if node, ok := sym.Node.(*ast.FuncDecl); ok {
			if node.Recv != nil && len(node.Recv.List) > 0 {
				parentType = sym.PackageIdentifier.Name + "." + golang.ReceiverTypeName(node.Recv.List[0].Type)
			} else if ctorType := golang.ConstructorTypeName(node.Type); ctorType != "" {
				parentType = sym.PackageIdentifier.Name + "." + ctorType
			}
		}

		deadByPkg[pkgPath] = append(deadByPkg[pkgPath], tempDeadSymbol{ds: ds, parentType: parentType})
	}

	// Build structured response for each package
	var deadPackages []string

	for pkgPath, deadSymbols := range deadByPkg {
		pkgStat := stats[pkgPath]

		// Build FileInfo map and detect dead files
		fileInfo := make(map[string]FileInfo)
		var deadFiles []string
		totalSymbols := 0
		deadSymbolCount := 0

		for filename, fs := range pkgStat.files {
			fileInfo[filename] = FileInfo{
				TotalSymbols: fs.total,
				DeadSymbols:  fs.dead,
			}
			totalSymbols += fs.total
			deadSymbolCount += fs.dead
			if fs.total > 0 && fs.dead == fs.total {
				deadFiles = append(deadFiles, filename)
			}
		}
		sort.Strings(deadFiles)

		// Check if entire package is dead
		isDead := totalSymbols > 0 && deadSymbolCount == totalSymbols
		if isDead {
			deadPackages = append(deadPackages, pkgPath)
		}

		// Build set of dead types for grouping
		deadTypeSymbols := make(map[string]DeadSymbol) // "pkg.Type" -> symbol
		for _, tds := range deadSymbols {
			if tds.ds.Kind == "type" || tds.ds.Kind == "interface" {
				deadTypeSymbols[tds.ds.Symbol] = tds.ds
			}
		}

		// Group symbols
		var constants, variables, functions []DeadSymbol
		methodsByType := make(map[string][]DeadSymbol)           // dead type -> methods
		constructorsByType := make(map[string][]DeadSymbol)      // dead type -> constructors
		standaloneMethodsByType := make(map[string][]DeadSymbol) // live type -> dead methods

		for _, tds := range deadSymbols {
			switch tds.ds.Kind {
			case "type", "interface":
				// handled separately
			case "const":
				constants = append(constants, tds.ds)
			case "var":
				variables = append(variables, tds.ds)
			case "method":
				if tds.parentType != "" {
					if _, isDeadType := deadTypeSymbols[tds.parentType]; isDeadType {
						methodsByType[tds.parentType] = append(methodsByType[tds.parentType], tds.ds)
					} else {
						standaloneMethodsByType[tds.parentType] = append(standaloneMethodsByType[tds.parentType], tds.ds)
					}
				} else {
					standaloneMethodsByType["(no receiver)"] = append(standaloneMethodsByType["(no receiver)"], tds.ds)
				}
			case "func":
				if tds.parentType != "" {
					if _, isDeadType := deadTypeSymbols[tds.parentType]; isDeadType {
						constructorsByType[tds.parentType] = append(constructorsByType[tds.parentType], tds.ds)
					} else {
						functions = append(functions, tds.ds)
					}
				} else {
					functions = append(functions, tds.ds)
				}
			}
		}

		// Build DeadType list
		var types []DeadType
		for _, tds := range deadSymbols {
			if tds.ds.Kind != "type" && tds.ds.Kind != "interface" {
				continue
			}
			types = append(types, DeadType{
				Symbol:       tds.ds,
				Constructors: constructorsByType[tds.ds.Symbol],
				Methods:      methodsByType[tds.ds.Symbol],
			})
		}

		// Build DeadMethodGroup list
		var deadMethods []DeadMethodGroup
		for parentType, methods := range standaloneMethodsByType {
			allDead := totalMethodsByType[parentType] > 0 && len(methods) == totalMethodsByType[parentType]
			deadMethods = append(deadMethods, DeadMethodGroup{
				ParentType: parentType,
				AllDead:    allDead,
				Methods:    methods,
			})
		}
		// Sort for consistent output
		sort.Slice(deadMethods, func(i, j int) bool {
			return deadMethods[i].ParentType < deadMethods[j].ParentType
		})

		packages = append(packages, &PackageDeadCode{
			Package:     pkgPath,
			Dir:         pkgStat.dir,
			IsDead:      isDead,
			DeadFiles:   deadFiles,
			FileInfo:    fileInfo,
			Constants:   constants,
			Variables:   variables,
			Functions:   functions,
			Types:       types,
			DeadMethods: deadMethods,
		})
	}

	sort.Strings(deadPackages)

	return &DeadcodeCommandResponse{
		Query: QueryInfo{
			Command:        "deadcode",
			Scope:          c.scope,
			HasEntryPoints: analysis.HasEntryPoints,
		},
		Summary: Summary{
			TotalSymbols: len(wc.Index.Symbols()),
			DeadSymbols:  totalDeadSymbols,
		},
		DeadPackages:       deadPackages,
		Packages:           packages,
		totalMethodsByType: totalMethodsByType,
	}, nil
}

// isEntryPoint returns true for main, init, and test functions
func isEntryPoint(sym *golang.Symbol) bool {
	name := sym.Name

	if sym.Kind == golang.SymbolKindFunc {
		if name == "main" || name == "init" {
			return true
		}

		// Test functions
		if strings.HasPrefix(name, "Test") ||
			strings.HasPrefix(name, "Benchmark") ||
			strings.HasPrefix(name, "Example") ||
			strings.HasPrefix(name, "Fuzz") {
			return true
		}
	}

	return false
}

// findReceiverTypeSymbol finds the type symbol for a method's receiver.
func findReceiverTypeSymbol(wc *commands.Wildcat, methodSym *golang.Symbol) *golang.Symbol {
	node, ok := methodSym.Node.(*ast.FuncDecl)
	if !ok || node.Recv == nil || len(node.Recv.List) == 0 {
		return nil
	}

	typeName := golang.ReceiverTypeName(node.Recv.List[0].Type)
	if typeName == "" {
		return nil
	}

	// Look up the type symbol
	matches := wc.Index.Lookup(typeName)
	if len(matches) == 0 {
		return nil
	}
	if len(matches) > 1 {
		var candidates []string
		for _, m := range matches {
			candidates = append(candidates, m.PackageIdentifier.PkgPath+"."+m.Name)
		}
		wc.AddDiagnostic("warning", "", "ambiguous type %q for method %s; matches %v", typeName, methodSym.Name, candidates)
		return nil
	}
	return matches[0]
}
