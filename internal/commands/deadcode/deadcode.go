package deadcode_cmd

import (
	"context"
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/spf13/cobra"
)

type DeadcodeCommand struct {
	targets         []string // which packages' symbols to report (empty = all project)
	includeTests    bool     // include test entry points in reachability analysis
	includeExported bool     // include exported symbols (may have external callers)
}

var _ commands.Command[*DeadcodeCommand] = (*DeadcodeCommand)(nil)

func WithTargets(targets []string) func(*DeadcodeCommand) error {
	return func(c *DeadcodeCommand) error {
		c.targets = targets
		return nil
	}
}

func WithIncludeTests(include bool) func(*DeadcodeCommand) error {
	return func(c *DeadcodeCommand) error {
		c.includeTests = include
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
		includeTests: true, // include tests by default
	}
}

func (c *DeadcodeCommand) Cmd() *cobra.Command {
	var includeTests bool
	var includeExported bool

	cmd := &cobra.Command{
		Use:   "deadcode [packages...]",
		Short: "Find unreachable code using static analysis",
		Long: `Find functions and methods not reachable from entry points.

Uses Rapid Type Analysis (RTA) to determine which code is actually
reachable from main(), init(), and test functions. This catches
transitively dead code that simple reference counting misses.

Target (optional):
  If one or more packages specified, only report dead code in those packages.
  If omitted, report all dead code in the project.

Flags:
  --tests         Include Test/Benchmark/Example functions as entry points (default: true)
  --exported      Include exported symbols (may have external callers)

Examples:
  wildcat deadcode                              # find all dead code
  wildcat deadcode internal/lsp                 # dead code in lsp package
  wildcat deadcode internal/lsp internal/errors # dead code in multiple packages
  wildcat deadcode --tests=false                # ignore test entry points
  wildcat deadcode --include-exported           # include exported API`,
		RunE: func(cmd *cobra.Command, args []string) error {
			wc, err := commands.LoadWildcat(cmd.Context(), ".")
			if err != nil {
				return err
			}

			result, err := c.Execute(cmd.Context(), wc,
				WithTargets(args),
				WithIncludeTests(includeTests),
				WithIncludeExported(includeExported),
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

	cmd.Flags().BoolVar(&includeTests, "tests", true, "Include Test/Benchmark/Example/Fuzz functions as entry points")
	cmd.Flags().BoolVar(&includeExported, "include-exported", false, "Include exported symbols (may have external callers)")

	return cmd
}

func (c *DeadcodeCommand) README() string {
	return "TODO"
}

func (c *DeadcodeCommand) Execute(ctx context.Context, wc *commands.Wildcat, opts ...func(*DeadcodeCommand) error) (commands.Result, error) {
	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, fmt.Errorf("internal_error: failed to apply opt: %w", err)
		}
	}

	// Resolve target package paths if specified
	targetPkgPaths := make(map[string]bool)
	for _, target := range c.targets {
		pi, err := wc.Project.ResolvePackageName(ctx, target)
		if err != nil {
			return commands.NewErrorResultf("invalid_target", "cannot resolve package %q: %v", target, err), nil
		}
		targetPkgPaths[pi.PkgPath] = true
	}

	// Run RTA analysis
	analysis, err := golang.AnalyzeDeadCode(wc.Project, c.includeTests)
	if err != nil {
		return nil, fmt.Errorf("dead code analysis failed: %w", err)
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
		// Filter by target packages if specified
		if len(targetPkgPaths) > 0 && !targetPkgPaths[sym.Package.Identifier.PkgPath] {
			continue
		}

		// Only report project packages
		if !strings.HasPrefix(sym.Package.Identifier.PkgPath, wc.Project.Module.Path) {
			continue
		}

		// Skip exported unless requested (but internal/ exports are always included)
		if !c.includeExported && ast.IsExported(sym.Name) && !sym.Package.Identifier.IsInternal() {
			continue
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
		pkgPath := sym.Package.Identifier.PkgPath
		filename := filepath.Base(sym.Filename())

		if stats[pkgPath] == nil {
			stats[pkgPath] = &pkgStats{
				dir:   sym.Package.Identifier.PkgDir,
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
			if node, ok := sym.Node().(*ast.FuncDecl); ok {
				if node.Recv != nil && len(node.Recv.List) > 0 {
					parentType = sym.Package.Identifier.Name + "." + golang.ReceiverTypeName(node.Recv.List[0].Type)
					totalMethodsByType[parentType]++
				}
			}
		}

		// Check if reachable via SSA
		if analysis.IsReachable(&sym) {
			continue
		}

		// Check if has non-call references (escaping function value).
		// Functions passed to external code (e.g., cobra handlers) may not be
		// traceable via SSA but are still used.
		if golang.CountNonCallReferences(wc.Project.Packages, &sym) > 0 {
			continue
		}

		// Check if method implements an interface.
		// Interface methods are required if the type is used, even if never called directly.
		if sym.Kind == golang.SymbolKindMethod && golang.IsInterfaceMethod(&sym, wc.Project, wc.Stdlib) {
			// Check if the receiver type is used (has any references)
			typeSym := findReceiverTypeSymbol(wc.Index, &sym)
			if typeSym != nil && golang.CountReferences(wc.Project.Packages, typeSym).Total() > 0 {
				continue
			}
		}

		// Update dead count
		stats[pkgPath].files[filename].dead++
		totalDeadSymbols++

		// Build dead symbol info
		sig, _ := sym.Signature()
		ds := DeadSymbol{
			Symbol:     sym.Package.Identifier.Name + "." + sym.Name,
			Kind:       string(sym.Kind),
			Signature:  sig,
			Definition: filename + ":" + sym.Location(),
		}

		// Get parent type for methods and constructors
		if node, ok := sym.Node().(*ast.FuncDecl); ok {
			if node.Recv != nil && len(node.Recv.List) > 0 {
				parentType = sym.Package.Identifier.Name + "." + golang.ReceiverTypeName(node.Recv.List[0].Type)
			} else if ctorType := golang.ConstructorTypeName(node.Type); ctorType != "" {
				parentType = sym.Package.Identifier.Name + "." + ctorType
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
		methodsByType := make(map[string][]DeadSymbol)    // dead type -> methods
		constructorsByType := make(map[string][]DeadSymbol) // dead type -> constructors
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

	// Convert target map keys to slice for response
	var targetsList []string
	for pkgPath := range targetPkgPaths {
		targetsList = append(targetsList, pkgPath)
	}

	return &DeadcodeCommandResponse{
		Query: QueryInfo{
			Command:      "deadcode",
			Targets:      targetsList,
			IncludeTests: c.includeTests,
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
func isEntryPoint(sym golang.Symbol) bool {
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
func findReceiverTypeSymbol(idx *golang.SymbolIndex, methodSym *golang.Symbol) *golang.Symbol {
	node, ok := methodSym.Node().(*ast.FuncDecl)
	if !ok || node.Recv == nil || len(node.Recv.List) == 0 {
		return nil
	}

	typeName := golang.ReceiverTypeName(node.Recv.List[0].Type)
	if typeName == "" {
		return nil
	}

	// Look up the type symbol - Lookup handles just the type name
	return idx.Lookup(typeName)
}
