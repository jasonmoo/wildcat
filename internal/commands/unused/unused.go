package unused_cmd

import (
	"context"
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"strings"
	"unicode"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/spf13/cobra"
)

type UnusedCommand struct {
	target          string // which package's symbols to check (empty = all project)
	scope           string // where to look for references
	includeExported bool
}

var _ commands.Command[*UnusedCommand] = (*UnusedCommand)(nil)

func WithTarget(target string) func(*UnusedCommand) error {
	return func(c *UnusedCommand) error {
		c.target = target
		return nil
	}
}

func WithScope(scope string) func(*UnusedCommand) error {
	return func(c *UnusedCommand) error {
		c.scope = scope
		return nil
	}
}

func WithIncludeExported(include bool) func(*UnusedCommand) error {
	return func(c *UnusedCommand) error {
		c.includeExported = include
		return nil
	}
}

func NewUnusedCommand() *UnusedCommand {
	return &UnusedCommand{
		scope: "project",
	}
}

func (c *UnusedCommand) Cmd() *cobra.Command {
	var scope string
	var includeExported bool

	cmd := &cobra.Command{
		Use:   "unused [package]",
		Short: "Find unused symbols in the codebase",
		Long: `Find symbols with no references.

Reports functions, methods, types, constants, and variables that are
defined but never used within the analyzed scope.

Target (optional):
  If specified, only check symbols in packages matching the target.
  If omitted, check all project packages.

Scope (--scope):
  Controls where to look for references.
  project       - All project packages (default)
  all           - Include dependencies
  pkg1,pkg2     - Specific package substrings
  -pkg          - Exclude packages matching substring
  package       - Only within target package (requires target)

Examples:
  wildcat unused                              # all project, project-wide refs
  wildcat unused lsp                          # lsp symbols, project-wide refs
  wildcat unused lsp --scope package          # lsp symbols, only lsp refs
  wildcat unused lsp --scope lsp,commands     # lsp symbols, refs in lsp+commands
  wildcat unused --include-exported           # include public API symbols`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wc, err := commands.LoadWildcat(cmd.Context(), ".")
			if err != nil {
				return err
			}

			var target string
			if len(args) > 0 {
				target = args[0]
			}

			result, err := c.Execute(cmd.Context(), wc,
				WithTarget(target),
				WithScope(scope),
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

	cmd.Flags().StringVar(&scope, "scope", "project", "Scope: 'project', 'all', or package substrings")
	cmd.Flags().BoolVar(&includeExported, "include-exported", false, "Include symbols exported from the module (may have external callers). Symbols in internal/ packages are always included.")

	return cmd
}

func (c *UnusedCommand) README() string {
	return "TODO"
}

func (c *UnusedCommand) Execute(ctx context.Context, wc *commands.Wildcat, opts ...func(*UnusedCommand) error) (commands.Result, error) {
	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, fmt.Errorf("internal_error: failed to apply opt: %w", err)
		}
	}

	modulePath := wc.Project.Module.Path

	// Build target filter (which symbols to check)
	targetFilter := c.parseTarget(modulePath)

	// Build scope filter (where to look for references)
	// Special case: "package" means use target as scope
	scopeStr := c.scope
	if scopeStr == "package" && c.target != "" {
		scopeStr = c.target
	}
	scopeFilter := c.parseScopeFilter(scopeStr, modulePath)

	// Collect all symbols matching target
	var candidates []golang.Symbol
	for _, sym := range wc.Index.Symbols() {
		// Apply target filter
		if !c.inScope(sym.Package.Identifier.PkgPath, targetFilter) {
			continue
		}

		// Skip exported unless requested (but internal/ exports are always included)
		if !c.includeExported && isExported(sym.Name) && !isInternalPkg(sym.Package.Identifier.PkgPath) {
			continue
		}

		// Skip special functions
		if isSpecialFunc(sym) {
			continue
		}

		// Skip blank identifiers (interface compliance checks)
		if sym.Name == "_" {
			continue
		}

		candidates = append(candidates, sym)
	}

	// For each candidate, count references within scope
	var unused []UnusedSymbol
	for _, sym := range candidates {
		refCount := c.countReferences(wc, &sym, scopeFilter)
		if refCount == 0 {
			sig, _ := sym.Signature()
			u := UnusedSymbol{
				Symbol:     sym.Package.Identifier.Name + "." + sym.Name,
				Kind:       string(sym.Kind),
				Signature:  sig,
				Definition: fmt.Sprintf("%s:%s", sym.Filename(), sym.Location()),
			}

			// Get parent type for methods and constructors
			if node, ok := sym.Node().(*ast.FuncDecl); ok {
				if node.Recv != nil && len(node.Recv.List) > 0 {
					// Method - get receiver type
					u.ParentType = sym.Package.Identifier.Name + "." + golang.ReceiverTypeName(node.Recv.List[0].Type)
				} else if ctorType := golang.ConstructorTypeName(node.Type); ctorType != "" {
					// Constructor - get return type
					u.ParentType = sym.Package.Identifier.Name + "." + ctorType
				}
			}

			unused = append(unused, u)
		}
	}

	return &UnusedCommandResponse{
		Query: QueryInfo{
			Command: "unused",
			Target:  c.target,
			Scope:   c.scope,
		},
		Unused: unused,
		Summary: Summary{
			Candidates: len(candidates),
			Unused:     len(unused),
		},
	}, nil
}

type scopeFilter struct {
	projectOnly bool
	includes    []string
	excludes    []string
	modulePath  string
}

func (c *UnusedCommand) parseTarget(modulePath string) scopeFilter {
	filter := scopeFilter{modulePath: modulePath}

	if c.target == "" {
		// No target = all project packages
		filter.projectOnly = true
		return filter
	}

	// Target specified - parse as includes
	for _, part := range strings.Split(c.target, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "-") {
			filter.excludes = append(filter.excludes, strings.TrimPrefix(part, "-"))
		} else {
			filter.includes = append(filter.includes, part)
		}
	}

	return filter
}

func (c *UnusedCommand) parseScopeFilter(scope, modulePath string) scopeFilter {
	filter := scopeFilter{modulePath: modulePath}

	if scope == "project" || scope == "" {
		filter.projectOnly = true
		return filter
	}

	if scope == "all" {
		return filter
	}

	// Parse includes/excludes
	for _, part := range strings.Split(scope, ",") {
		part = strings.TrimSpace(part)
		if part == "" || part == "project" {
			continue
		}
		if strings.HasPrefix(part, "-") {
			filter.excludes = append(filter.excludes, strings.TrimPrefix(part, "-"))
		} else {
			filter.includes = append(filter.includes, part)
		}
	}

	return filter
}

func (c *UnusedCommand) inScope(pkgPath string, filter scopeFilter) bool {
	// Project-only filter
	if filter.projectOnly {
		return strings.HasPrefix(pkgPath, filter.modulePath)
	}

	// Check excludes
	for _, ex := range filter.excludes {
		if strings.Contains(pkgPath, ex) {
			return false
		}
	}

	// Check includes (if specified)
	if len(filter.includes) > 0 {
		for _, inc := range filter.includes {
			if strings.Contains(pkgPath, inc) {
				return true
			}
		}
		return false
	}

	return true
}

func isExported(name string) bool {
	if name == "" {
		return false
	}
	r := []rune(name)
	return unicode.IsUpper(r[0])
}

func isInternalPkg(pkgPath string) bool {
	return strings.Contains(pkgPath, "/internal/") || strings.HasSuffix(pkgPath, "/internal")
}

func isSpecialFunc(sym golang.Symbol) bool {
	if sym.Kind != golang.SymbolKindFunc {
		return false
	}

	name := sym.Name

	// main and init are entry points
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

	return false
}

func (c *UnusedCommand) countReferences(wc *commands.Wildcat, target *golang.Symbol, filter scopeFilter) int {
	targetObj := c.getTargetObject(target)
	if targetObj == nil {
		return 0
	}

	count := 0
	targetFile := target.Filename()
	targetLine := strings.Split(target.Location(), ":")[0]

	for _, pkg := range wc.Project.Packages {
		if !c.inScope(pkg.Identifier.PkgPath, filter) {
			continue
		}

		for _, file := range pkg.Package.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				ident, ok := n.(*ast.Ident)
				if !ok {
					return true
				}

				obj := pkg.Package.TypesInfo.Uses[ident]
				if obj == nil {
					return true
				}

				if !c.sameObject(obj, targetObj) {
					return true
				}

				// Skip the definition itself
				pos := pkg.Package.Fset.Position(ident.Pos())
				if pos.Filename == targetFile && fmt.Sprintf("%d", pos.Line) == targetLine {
					return true
				}

				count++
				return true
			})
		}
	}

	return count
}

func (c *UnusedCommand) getTargetObject(target *golang.Symbol) types.Object {
	node := target.Node()

	switch n := node.(type) {
	case *ast.FuncDecl:
		return target.Package.Package.TypesInfo.Defs[n.Name]
	case *ast.TypeSpec:
		return target.Package.Package.TypesInfo.Defs[n.Name]
	case *ast.ValueSpec:
		for _, name := range n.Names {
			if name.Name == target.Name {
				return target.Package.Package.TypesInfo.Defs[name]
			}
		}
	}
	return nil
}

func (c *UnusedCommand) sameObject(obj, target types.Object) bool {
	if obj == target {
		return true
	}
	if obj.Pkg() == nil || target.Pkg() == nil {
		return false
	}
	return obj.Pkg().Path() == target.Pkg().Path() &&
		obj.Name() == target.Name() &&
		obj.Pos() == target.Pos()
}
