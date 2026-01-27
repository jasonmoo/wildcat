package ls_cmd

import (
	"context"
	"sort"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/commands/spath"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/sahilm/fuzzy"
	"github.com/spf13/cobra"
)

type LsCommand struct {
	targets []string
	depth   int // 1 = immediate children only, 0 = unlimited depth
}

var _ commands.Command[*LsCommand] = (*LsCommand)(nil)

func WithTargets(targets []string) func(*LsCommand) error {
	return func(c *LsCommand) error {
		c.targets = targets
		return nil
	}
}

func WithDepth(depth int) func(*LsCommand) error {
	return func(c *LsCommand) error {
		c.depth = depth
		return nil
	}
}

func NewLsCommand() *LsCommand {
	return &LsCommand{
		depth: 1, // default to immediate children only
	}
}

func (c *LsCommand) Cmd() *cobra.Command {
	var depth int

	cmd := &cobra.Command{
		Use:   "ls <path> [path...]",
		Short: "List available paths within a scope",
		Long: `Discover semantic paths for code elements.

The ls command shows what paths are available from a given starting point.
Use it to explore the codebase before using read or edit commands.

Arguments:
  <path>    Package path, symbol, or semantic path (multiple allowed)

Flags:
  --depth   How deep to recurse (1 = immediate children, 0 = unlimited)

Examples:
  wildcat ls internal/golang                 # all symbols in package
  wildcat ls golang.Symbol                   # fields, methods of a type
  wildcat ls golang.Symbol golang.Package   # multiple symbols
  wildcat ls golang.Symbol --depth 0         # all paths recursively
  wildcat ls golang.WalkReferences           # params, returns, body of a function
  wildcat ls golang.Symbol.Signature         # parts of a method
  wildcat ls golang.Symbol/fields[Name]      # subpaths of a field

Output shows paths that can be used with read/edit commands.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.RunCommand(cmd, c, WithTargets(args), WithDepth(depth))
		},
	}

	cmd.Flags().IntVar(&depth, "depth", 1, "How deep to recurse (1 = immediate, 0 = unlimited)")

	return cmd
}

func (c *LsCommand) README() string {
	return "TODO"
}

func (c *LsCommand) Execute(ctx context.Context, wc *commands.Wildcat, opts ...func(*LsCommand) error) (commands.Result, error) {
	if len(c.targets) == 0 {
		return commands.NewErrorResultf("invalid_target", "at least one target path is required"), nil
	}

	var sections []TargetSection

	for _, target := range c.targets {
		section, errResult := c.listTarget(ctx, wc, target)
		if errResult != nil {
			// Include error as a section with no paths
			sections = append(sections, TargetSection{
				Target:      target,
				Error:       errResult.Error.Error(),
				Suggestions: errResult.Suggestions,
			})
			continue
		}
		sections = append(sections, *section)
	}

	return &LsResponse{
		Sections: sections,
	}, nil
}

// listTarget lists paths for a single target, returning either a section or an error.
func (c *LsCommand) listTarget(ctx context.Context, wc *commands.Wildcat, target string) (*TargetSection, *commands.ErrorResult) {
	// Try to resolve as a package first
	if pi, err := wc.Project.ResolvePackageName(ctx, target); err == nil {
		if pkg, err := wc.Package(pi); err == nil {
			return c.listPackageSection(ctx, wc, target, pkg), nil
		}
	}

	// Try to resolve as a semantic path (symbol or deeper)
	res, err := wc.ResolveSpath(ctx, target)
	if err != nil {
		return nil, c.notFoundError(wc, target, err)
	}

	return c.listResolutionSection(ctx, wc, target, res), nil
}

// listPackageSection enumerates top-level symbols and subpackages in a package.
func (c *LsCommand) listPackageSection(ctx context.Context, wc *commands.Wildcat, target string, pkg *golang.Package) *TargetSection {
	var paths []PathEntry

	// Sort symbols by name for consistent output
	symbols := make([]*golang.Symbol, len(pkg.Symbols))
	copy(symbols, pkg.Symbols)
	sort.Slice(symbols, func(i, j int) bool {
		return symbols[i].Name < symbols[j].Name
	})

	for _, sym := range symbols {
		symPath := spath.Generate(sym)
		paths = append(paths, PathEntry{
			Path: symPath,
			Kind: string(sym.Kind),
		})

		// Recurse if depth allows
		if c.shouldRecurse(1) {
			childPaths := c.enumerateRecursive(ctx, wc, symPath, 2)
			paths = append(paths, childPaths...)
		}
	}

	// Find all descendant packages and show with relative paths
	prefix := pkg.Identifier.PkgPath + "/"
	var subpkgs []string
	for _, p := range wc.Project.Packages {
		if strings.HasPrefix(p.Identifier.PkgPath, prefix) {
			// Use module-relative path for display
			subpkgs = append(subpkgs, p.Identifier.PkgShortPath)
		}
	}
	sort.Strings(subpkgs)

	for _, subpkg := range subpkgs {
		paths = append(paths, PathEntry{
			Path: subpkg,
			Kind: "package",
		})
	}

	return &TargetSection{
		Target:  target,
		Scope:   "package",
		Package: pkg.Identifier.PkgPath,
		Paths:   paths,
	}
}

// listResolutionSection enumerates children from a resolved semantic path.
func (c *LsCommand) listResolutionSection(ctx context.Context, wc *commands.Wildcat, target string, res *spath.Resolution) *TargetSection {
	// Create a short-form base path for enumeration using package short name
	// Include segments if the original path had them (subpath query)
	shortBasePath := &spath.Path{
		Package:  res.Package.Identifier.Name,
		Symbol:   res.Path.Symbol,
		Method:   res.Path.Method,
		Segments: res.Path.Segments,
	}

	// Start with the resolved target itself as the first entry
	self := spath.EnumerateSelf(res, res.Symbol, shortBasePath)
	paths := []PathEntry{{
		Path: self.Path,
		Kind: self.Kind,
		Type: self.Type,
	}}

	children := spath.EnumerateChildrenWithBase(res, res.Symbol, shortBasePath)

	for _, child := range children {
		paths = append(paths, PathEntry{
			Path: child.Path,
			Kind: child.Kind,
			Type: child.Type,
		})

		// Recurse if depth allows
		if c.shouldRecurse(1) {
			childPaths := c.enumerateRecursive(ctx, wc, child.Path, 2)
			paths = append(paths, childPaths...)
		}
	}

	scope := "symbol"
	if res.Field != nil {
		scope = "field"
	}

	// Symbol for header - include method and subpath if present
	symName := res.Path.Symbol
	if res.Path.Method != "" {
		symName = res.Path.Symbol + "." + res.Path.Method
	}
	// Append subpath segments to the symbol name for the header
	for _, seg := range res.Path.Segments {
		symName += "/" + seg.Category
		if seg.Selector != "" {
			symName += "[" + seg.Selector + "]"
		}
	}

	return &TargetSection{
		Target:  target,
		Scope:   scope,
		Package: res.Package.Identifier.PkgPath,
		Symbol:  symName,
		Paths:   paths,
	}
}

// shouldRecurse returns true if we should recurse at the given current depth.
// depth=0 means unlimited, depth=1 means no recursion, depth>1 means recurse.
func (c *LsCommand) shouldRecurse(currentDepth int) bool {
	if c.depth == 0 {
		return true // unlimited
	}
	return currentDepth < c.depth
}

// enumerateRecursive resolves a path and enumerates its children recursively.
func (c *LsCommand) enumerateRecursive(ctx context.Context, wc *commands.Wildcat, pathStr string, currentDepth int) []PathEntry {
	res, err := wc.ResolveSpath(ctx, pathStr)
	if err != nil {
		return nil // silently skip paths that can't be resolved
	}

	// Use short package name for output
	shortBasePath := &spath.Path{
		Package: res.Package.Identifier.Name,
		Symbol:  res.Path.Symbol,
		Method:  res.Path.Method,
	}

	children := spath.EnumerateChildrenWithBase(res, res.Symbol, shortBasePath)
	var paths []PathEntry

	for _, child := range children {
		paths = append(paths, PathEntry{
			Path: child.Path,
			Kind: child.Kind,
			Type: child.Type,
		})

		// Continue recursing if depth allows
		if c.shouldRecurse(currentDepth) {
			childPaths := c.enumerateRecursive(ctx, wc, child.Path, currentDepth+1)
			paths = append(paths, childPaths...)
		}
	}

	return paths
}

func (c *LsCommand) notFoundError(wc *commands.Wildcat, target string, err error) *commands.ErrorResult {
	e := commands.NewErrorResultf("path_not_found", "not found")

	// Add symbol suggestions
	for _, s := range wc.Suggestions(target, &golang.SearchOptions{Limit: 5}) {
		e.Suggestions = append(e.Suggestions, s.Symbol)
	}

	// Add package suggestions (fuzzy match on short path)
	var pkgPaths []string
	for _, pkg := range wc.Project.Packages {
		pkgPaths = append(pkgPaths, pkg.Identifier.PkgShortPath)
	}
	matches := fuzzy.Find(target, pkgPaths)
	for i, m := range matches {
		if i >= 5 {
			break
		}
		e.Suggestions = append(e.Suggestions, m.Str)
	}

	return e
}
