package deadcode_cmd

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
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

	// Build package/file info for dead file/package detection
	packages := make(map[string]PackageInfo)

	// First pass: count total symbols per file/package and track dead symbols
	var deadSymbols []DeadSymbol
	totalMethodsByType := make(map[string]int)

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

		// Track package/file info
		pkgPath := sym.Package.Identifier.PkgPath
		filename := filepath.Base(sym.Filename())

		if _, ok := packages[pkgPath]; !ok {
			packages[pkgPath] = PackageInfo{
				Package: pkgPath,
				Files:   make(map[string]FileInfo),
			}
		}
		pkg := packages[pkgPath]
		pkg.TotalSymbols++

		if fi, ok := pkg.Files[filename]; ok {
			fi.TotalSymbols++
			pkg.Files[filename] = fi
		} else {
			pkg.Files[filename] = FileInfo{
				Filename:     filename,
				TotalSymbols: 1,
			}
		}
		packages[pkgPath] = pkg

		// Count methods by type for grouping
		if sym.Kind == golang.SymbolKindMethod {
			if node, ok := sym.Node().(*ast.FuncDecl); ok {
				if node.Recv != nil && len(node.Recv.List) > 0 {
					parentType := sym.Package.Identifier.Name + "." + golang.ReceiverTypeName(node.Recv.List[0].Type)
					totalMethodsByType[parentType]++
				}
			}
		}

		// Check if reachable
		if analysis.IsReachable(&sym) {
			continue
		}

		// Update dead counts
		pkg = packages[pkgPath]
		pkg.DeadSymbols++
		fi := pkg.Files[filename]
		fi.DeadSymbols++
		pkg.Files[filename] = fi
		packages[pkgPath] = pkg

		// Build dead symbol info
		sig, _ := sym.Signature()
		ds := DeadSymbol{
			Symbol:    sym.Package.Identifier.Name + "." + sym.Name,
			Kind:      string(sym.Kind),
			Signature: sig,
			Package:   pkgPath,
			Filename:  filename,
			Location:  sym.Location(),
		}

		// Get parent type for methods and constructors
		if node, ok := sym.Node().(*ast.FuncDecl); ok {
			if node.Recv != nil && len(node.Recv.List) > 0 {
				ds.ParentType = sym.Package.Identifier.Name + "." + golang.ReceiverTypeName(node.Recv.List[0].Type)
			} else if ctorType := golang.ConstructorTypeName(node.Type); ctorType != "" {
				ds.ParentType = sym.Package.Identifier.Name + "." + ctorType
			}
		}

		deadSymbols = append(deadSymbols, ds)
	}

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
		Dead:               deadSymbols,
		TotalMethodsByType: totalMethodsByType,
		Packages:           packages,
		Summary: Summary{
			TotalSymbols: len(wc.Index.Symbols()),
			DeadSymbols:  len(deadSymbols),
		},
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

// positionKey creates a unique key for a token.Position
func positionKey(pos token.Position) string {
	return fmt.Sprintf("%s:%d", pos.Filename, pos.Line)
}
