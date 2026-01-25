package symbol_cmd

import (
	"context"
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/spf13/cobra"
)

type SymbolCommand struct {
	symbols []string
	scope   string
}

var _ commands.Command[*SymbolCommand] = (*SymbolCommand)(nil)

func WithSymbols(symbols []string) func(*SymbolCommand) error {
	return func(c *SymbolCommand) error {
		c.symbols = append(c.symbols, symbols...)
		return nil
	}
}

func WithScope(s string) func(*SymbolCommand) error {
	return func(c *SymbolCommand) error {
		c.scope = s
		return nil
	}
}

// pkgUsage groups callers and references by package
type pkgUsage struct {
	pkg        *golang.Package
	callers    []callerInfo
	references []referenceInfo
}

// getSymbolRefs looks up a symbol and returns its reference counts.
func getSymbolRefs(wc *commands.Wildcat, symbolKey string) *SymbolRefs {
	matches := wc.Index.Lookup(symbolKey)
	if len(matches) == 0 {
		return nil
	}
	if len(matches) > 1 {
		var candidates []string
		for _, m := range matches {
			candidates = append(candidates, m.Package.Identifier.PkgPath+"."+m.Name)
		}
		wc.AddDiagnostic("warning", "", "ambiguous symbol %q matches %v; refs unavailable", symbolKey, candidates)
		return nil
	}
	counts := golang.CountReferences(wc.Project.Packages, matches[0])
	return &SymbolRefs{
		Internal: counts.Internal,
		External: counts.External,
		Packages: counts.PackageCount(),
	}
}

// getSymbolRefsOutput looks up a symbol and returns its reference counts as output.TargetRefs.
func getSymbolRefsOutput(wc *commands.Wildcat, symbolKey string) *output.TargetRefs {
	matches := wc.Index.Lookup(symbolKey)
	if len(matches) == 0 {
		return nil
	}
	if len(matches) > 1 {
		var candidates []string
		for _, m := range matches {
			candidates = append(candidates, m.Package.Identifier.PkgPath+"."+m.Name)
		}
		wc.AddDiagnostic("warning", "", "ambiguous symbol %q matches %v; refs unavailable", symbolKey, candidates)
		return nil
	}
	counts := golang.CountReferences(wc.Project.Packages, matches[0])
	return &output.TargetRefs{
		Internal: counts.Internal,
		External: counts.External,
		Packages: counts.PackageCount(),
	}
}

func NewSymbolCommand() *SymbolCommand {
	return &SymbolCommand{
		scope: "project",
	}
}

func (c *SymbolCommand) Cmd() *cobra.Command {
	var scope string

	cmd := &cobra.Command{
		Use:   "symbol <symbol> [symbol...]",
		Short: "Complete symbol analysis: definition, callers, refs, interfaces",
		Long: `Full profile of a symbol: everything you need to understand and modify it.

Returns:
  - Definition location and signature
  - Direct callers (who calls this)
  - All references (type usage, not just calls)
  - Interface relationships (satisfies/implements)

Scope (filters output, not analysis):
  project       - All project packages (default)
  package       - Target package only
  all           - Include dependencies and stdlib
  pkg1,pkg2     - Specific packages (comma-separated)
  -pkg          - Exclude package (prefix with -)

Pattern syntax:
  internal/lsp       - Exact package match
  internal/...       - Package and all subpackages (Go-style)
  internal/*         - Direct children only
  internal/**        - All descendants
  **/util            - Match anywhere in path

Full project is analyzed; scope controls which usages appear in output.

Examples:
  wildcat symbol Config                              # analyze Config type
  wildcat symbol --scope package Server.Start        # target package only
  wildcat symbol --scope "project,-internal/..."     # exclude internal subtree
  wildcat symbol --scope "**/lsp" Handler            # only packages matching pattern`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.RunCommand(cmd, c,
				WithSymbols(args),
				WithScope(scope),
			)
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "project", "Filter output to packages (patterns: internal/..., **/util, -excluded)")

	return cmd
}

func (c *SymbolCommand) README() string {
	return "TODO"
}

func (c *SymbolCommand) Execute(ctx context.Context, wc *commands.Wildcat, opts ...func(*SymbolCommand) error) (commands.Result, error) {
	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, fmt.Errorf("internal_error: failed to apply opt: %w", err)
		}
	}

	if len(c.symbols) == 0 {
		return commands.NewErrorResultf("invalid_symbol", "at least one symbol is required"), nil
	}

	// Single symbol - return SymbolCommandResponse directly for backwards compatibility
	if len(c.symbols) == 1 {
		return c.executeOne(ctx, wc, c.symbols[0])
	}

	// Multiple symbols - return MultiSymbolCommandResponse
	var results []SymbolCommandResponse
	for _, sym := range c.symbols {
		result, err := c.executeOne(ctx, wc, sym)
		if err != nil {
			return nil, err
		}
		// Type assert - executeOne returns *SymbolCommandResponse
		if resp, ok := result.(*SymbolCommandResponse); ok {
			results = append(results, *resp)
		}
	}

	return &MultiSymbolCommandResponse{Symbols: results}, nil
}

func (c *SymbolCommand) executeOne(ctx context.Context, wc *commands.Wildcat, symbol string) (commands.Result, error) {
	if i := strings.IndexByte(symbol, '.'); i > -1 {
		path, name := symbol[:i], symbol[i+1:]
		pi, err := wc.Project.ResolvePackageName(ctx, path)
		if err == nil {
			symbol = pi.PkgPath + "." + name
		}
	}

	// Find target symbol
	target, errResp := wc.LookupSymbol(symbol)
	if errResp != nil {
		return errResp, nil
	}

	// Build target info
	sig := target.Signature()
	definition := fmt.Sprintf("%s:%s", filepath.Base(target.Filename()), target.Location())
	qualifiedSymbol := target.Package.Identifier.PkgPath + "." + target.Name

	// Parse scope
	scopeFilter, err := wc.ParseScope(ctx, c.scope, target.Package.Identifier.PkgPath)
	if err != nil {
		return nil, err
	}

	// Snippet extractor for AST-aware snippet extraction
	extractor := output.NewSnippetExtractor()

	// Collect all usages grouped by package
	usageByPkg := make(map[string]*pkgUsage)

	// Find callers (for functions and methods)
	if target.Kind == golang.SymbolKindFunc || target.Kind == golang.SymbolKindMethod {
		c.findCallers(wc, target, usageByPkg)
	}

	// Find methods and constructors for types (before references so we can exclude them)
	var methods, constructors []FunctionInfo
	excludeFromRefs := make(map[string]bool) // file:line keys to exclude from references
	if target.Kind == golang.SymbolKindType || target.Kind == golang.SymbolKindInterface {
		// Use precomputed methods/constructors from PackageSymbol
		if pkgSym := target.PackageSymbol(); pkgSym != nil {
			for _, m := range pkgSym.Methods {
				methodSymbol := target.Package.Identifier.Name + "." + target.Name + "." + m.Name
				methods = append(methods, FunctionInfo{
					Symbol:     methodSymbol,
					Signature:  m.Signature(),
					Definition: m.FileDefinition(),
					Refs:       getSymbolRefs(wc, methodSymbol),
				})
				excludeFromRefs[m.PathLocation()] = true
			}
			for _, c := range pkgSym.Constructors {
				constructorSymbol := target.Package.Identifier.Name + "." + c.Name
				constructors = append(constructors, FunctionInfo{
					Symbol:     constructorSymbol,
					Signature:  c.Signature(),
					Definition: c.FileDefinition(),
					Refs:       getSymbolRefs(wc, constructorSymbol),
				})
				excludeFromRefs[c.PathLocation()] = true
			}
		}
	}

	// Find references (for all symbol types)
	c.findReferences(wc, target, usageByPkg)

	// Find interface relationships
	var implementations []PackageTypes
	var satisfies []PackageTypes

	var consumers []PackageFunctions

	if target.Kind == golang.SymbolKindInterface {
		implementations = c.findImplementations(wc, target)
		consumers = c.findConsumers(wc, target)
	}
	if target.Kind == golang.SymbolKindType {
		satisfies = c.findSatisfies(wc, target)
	}

	// Find descendants (types that would be orphaned if target removed)
	descendants := c.findDescendants(wc, target)

	// Build output packages list
	var packageUsages []output.PackageUsage
	var importedBy []output.DepResult

	// Sort packages, target package first
	var pkgPaths []string
	for pkgPath := range usageByPkg {
		pkgPaths = append(pkgPaths, pkgPath)
	}
	sort.Strings(pkgPaths)

	// Move target package to front
	targetPkgPath := target.Package.Identifier.PkgPath
	for i, p := range pkgPaths {
		if p == targetPkgPath {
			pkgPaths = append([]string{p}, append(pkgPaths[:i], pkgPaths[i+1:]...)...)
			break
		}
	}

	// Build package usages (filtered by scope)
	for _, pkgPath := range pkgPaths {
		// Apply scope filter to output
		if !scopeFilter.InScope(pkgPath) {
			continue
		}

		usage := usageByPkg[pkgPath]

		var callerLocs []output.Location
		for _, caller := range usage.callers {
			snippet, start, end, _ := extractor.ExtractSmart(caller.file, caller.line)
			callerLocs = append(callerLocs, output.Location{
				Location: fmt.Sprintf("%s:%d", filepath.Base(caller.file), caller.line),
				Symbol:   caller.symbol,
				Snippet: output.Snippet{
					Location: fmt.Sprintf("%s:%d:%d", filepath.Base(caller.file), start, end),
					Source:   snippet,
				},
				Refs: getSymbolRefsOutput(wc, caller.symbol),
			})
		}

		var refLocs []output.Location
		for _, ref := range usage.references {
			// Skip method/constructor definition lines
			if excludeFromRefs[fmt.Sprintf("%s:%d", ref.file, ref.line)] {
				continue
			}
			snippet, start, end, _ := extractor.ExtractSmart(ref.file, ref.line)
			refLocs = append(refLocs, output.Location{
				Location: fmt.Sprintf("%s:%d", filepath.Base(ref.file), ref.line),
				Symbol:   ref.symbol,
				Snippet: output.Snippet{
					Location: fmt.Sprintf("%s:%d:%d", filepath.Base(ref.file), start, end),
					Source:   snippet,
				},
				Refs: getSymbolRefsOutput(wc, ref.symbol),
			})
		}

		// Merge references within same AST scope to reduce duplication
		mergedRefs := extractor.MergeLocations(usage.pkg.Identifier.PkgDir, refLocs)

		packageUsages = append(packageUsages, output.PackageUsage{
			Package:    pkgPath,
			Dir:        usage.pkg.Identifier.PkgDir,
			Callers:    callerLocs,
			References: mergedRefs,
		})

		// Track imported_by (packages other than target that have usages)
		if pkgPath != targetPkgPath && (len(callerLocs) > 0 || len(refLocs) > 0) {
			var loc string
			if len(callerLocs) > 0 {
				loc = filepath.Join(usage.pkg.Identifier.PkgDir, callerLocs[0].Location)
			} else {
				loc = filepath.Join(usage.pkg.Identifier.PkgDir, refLocs[0].Location)
			}
			importedBy = append(importedBy, output.DepResult{
				Package:  pkgPath,
				Location: loc,
			})
		}
	}

	// Compute summaries
	var queryCallers, queryRefs int
	var pkgCallers, pkgRefs int
	var projectCallers, projectRefs int

	for pkgPath, usage := range usageByPkg {
		callerCount := len(usage.callers)
		refCount := len(usage.references)

		projectCallers += callerCount
		projectRefs += refCount

		if pkgPath == targetPkgPath {
			pkgCallers = callerCount
			pkgRefs = refCount
		}

		// Query summary only includes filtered packages
		queryCallers += callerCount
		queryRefs += refCount
	}

	// Build exclusion list from all symbols in the report
	excludeSymbols := []string{
		target.Package.Identifier.Name + "." + target.Name, // target itself
	}
	for _, m := range methods {
		excludeSymbols = append(excludeSymbols, m.Symbol)
	}
	for _, c := range constructors {
		excludeSymbols = append(excludeSymbols, c.Symbol)
	}
	for _, pkg := range implementations {
		for _, impl := range pkg.Types {
			excludeSymbols = append(excludeSymbols, impl.Symbol)
		}
	}
	for _, pkg := range satisfies {
		for _, sat := range pkg.Types {
			excludeSymbols = append(excludeSymbols, sat.Symbol)
		}
	}
	for _, desc := range descendants {
		excludeSymbols = append(excludeSymbols, desc.Symbol)
	}

	// Count total implementations and satisfies across all packages
	totalImplementations := 0
	for _, pkg := range implementations {
		totalImplementations += len(pkg.Types)
	}
	totalSatisfies := 0
	for _, pkg := range satisfies {
		totalSatisfies += len(pkg.Types)
	}

	// Get fuzzy matches for suggestions
	suggestions := wc.Suggestions(symbol, &golang.SearchOptions{Limit: 5, Exclude: excludeSymbols})
	fuzzyMatches := make([]SuggestionInfo, len(suggestions))
	for i, s := range suggestions {
		fuzzyMatches[i] = SuggestionInfo{Symbol: s.Symbol, Kind: s.Kind}
	}

	// Build scope resolved info if patterns were used
	var scopeResolved *output.ScopeResolved
	if len(scopeFilter.ExcludePatterns()) > 0 || len(scopeFilter.IncludePatterns()) > 0 {
		scopeResolved = &output.ScopeResolved{
			Includes: scopeFilter.ResolvedIncludes(),
			Excludes: scopeFilter.ResolvedExcludes(),
		}
	}

	return &SymbolCommandResponse{
		Query: output.QueryInfo{
			Command:       "symbol",
			Target:        symbol,
			Resolved:      qualifiedSymbol,
			Scope:         c.scope,
			ScopeResolved: scopeResolved,
		},
		Package: output.PackageInfo{
			ImportPath: target.Package.Identifier.PkgPath,
			Name:       target.Package.Identifier.Name,
			Dir:        target.Package.Identifier.PkgDir,
		},
		Target: output.TargetInfo{
			Symbol:     qualifiedSymbol,
			Kind:       string(target.Kind),
			Signature:  sig,
			Definition: definition,
			Refs: output.TargetRefs{
				Internal: pkgRefs,
				External: projectRefs - pkgRefs,
				Packages: len(importedBy),
			},
		},
		Methods:         methods,
		Constructors:    constructors,
		Descendants:     descendants,
		ImportedBy:      importedBy,
		References:      packageUsages,
		Implementations: implementations,
		Consumers:       consumers,
		Satisfies:       satisfies,
		QuerySummary: output.SymbolSummary{
			Callers:         queryCallers,
			References:      queryRefs,
			Implementations: totalImplementations,
			Satisfies:       totalSatisfies,
		},
		PackageSummary: output.SymbolSummary{
			Callers:         pkgCallers,
			References:      pkgRefs,
			Implementations: totalImplementations,
			Satisfies:       totalSatisfies,
		},
		ProjectSummary: output.SymbolSummary{
			Callers:         projectCallers,
			References:      projectRefs,
			Implementations: totalImplementations,
			Satisfies:       totalSatisfies,
		},
		OtherFuzzyMatches: fuzzyMatches,
	}, nil
}

type callerInfo struct {
	file   string
	line   int
	symbol string
}

type referenceInfo struct {
	file   string
	line   int
	symbol string
}

func (c *SymbolCommand) findCallers(wc *commands.Wildcat, target *golang.Symbol, usageByPkg map[string]*pkgUsage) {
	// Get the target's types.Object for comparison
	node := target.Node()
	funcDecl, ok := node.(*ast.FuncDecl)
	if !ok || funcDecl.Body == nil {
		return
	}

	targetObj := target.Package.Package.TypesInfo.Defs[funcDecl.Name]
	if targetObj == nil {
		wc.Diagnostics = append(wc.Diagnostics, commands.Diagnostics{
			Level:   "warning",
			Package: target.Package.Identifier.PkgPath,
			Message: fmt.Sprintf("callers analysis incomplete: type info unavailable for %s", funcDecl.Name.Name),
		})
		return
	}

	// Search all packages for calls to target
	for _, pkg := range wc.Project.Packages {
		for _, file := range pkg.Package.Syntax {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}

				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}

					calledFn := golang.ResolveCallExpr(pkg.Package.TypesInfo, call)
					if calledFn == nil || calledFn != targetObj {
						return true
					}

					// Found a caller
					callPos := pkg.Package.Fset.Position(call.Pos())

					// Build caller symbol name
					callerSymbol := pkg.Identifier.Name + "."
					if fn.Recv != nil && len(fn.Recv.List) > 0 {
						callerSymbol += golang.ReceiverTypeName(fn.Recv.List[0].Type) + "."
					}
					callerSymbol += fn.Name.Name

					// Get or create package usage
					if usageByPkg[pkg.Identifier.PkgPath] == nil {
						usageByPkg[pkg.Identifier.PkgPath] = &pkgUsage{pkg: pkg}
					}
					usageByPkg[pkg.Identifier.PkgPath].callers = append(
						usageByPkg[pkg.Identifier.PkgPath].callers,
						callerInfo{
							file:   callPos.Filename,
							line:   callPos.Line,
							symbol: callerSymbol,
						},
					)

					return true
				})
			}
		}
	}
}

func (c *SymbolCommand) findReferences(wc *commands.Wildcat, target *golang.Symbol, usageByPkg map[string]*pkgUsage) {
	// Track caller locations to avoid duplicates
	callerLocs := make(map[string]bool)
	for _, usage := range usageByPkg {
		for _, caller := range usage.callers {
			key := fmt.Sprintf("%s:%d", caller.file, caller.line)
			callerLocs[key] = true
		}
	}

	// Get target definition location for exclusion
	defLine, _, _ := strings.Cut(target.Location(), ":")
	targetFile := target.Filename()

	golang.WalkReferences(wc.Project.Packages, target, func(ref golang.Reference) bool {
		key := fmt.Sprintf("%s:%d", ref.File, ref.Line)

		// Skip if already counted as caller
		if callerLocs[key] {
			return true
		}

		// Skip the definition itself
		if ref.File == targetFile && fmt.Sprintf("%d", ref.Line) == defLine {
			return true
		}

		// Get or create package usage
		pkgPath := ref.Package.Identifier.PkgPath
		if usageByPkg[pkgPath] == nil {
			usageByPkg[pkgPath] = &pkgUsage{pkg: ref.Package}
		}
		usageByPkg[pkgPath].references = append(
			usageByPkg[pkgPath].references,
			referenceInfo{
				file:   ref.File,
				line:   ref.Line,
				symbol: ref.Containing,
			},
		)

		return true
	})
}

func (c *SymbolCommand) findImplementations(wc *commands.Wildcat, target *golang.Symbol) []PackageTypes {
	// Get the interface type using shared helper
	iface := golang.GetInterfaceType(target)
	if iface == nil {
		return nil
	}

	// Get the types.Object for comparison (to skip the interface itself)
	targetObj := golang.GetTypesObject(target)

	// Group implementations by package
	byPkg := make(map[string]*PackageTypes)

	// Search all packages for types that implement this interface
	for _, pkg := range wc.Project.Packages {
		for ident, obj := range pkg.Package.TypesInfo.Defs {
			// Only interested in type definitions
			typeName, ok := obj.(*types.TypeName)
			if !ok {
				continue
			}

			// Skip the interface itself
			if obj == targetObj {
				continue
			}

			// Check if this type implements the interface
			// Use pointer type as well since methods might be on pointer receiver
			typ := typeName.Type()
			ptrTyp := types.NewPointer(typ)

			if types.Implements(typ, iface) || types.Implements(ptrTyp, iface) {
				pos := pkg.Package.Fset.Position(ident.Pos())
				symbolKey := pkg.Identifier.Name + "." + typeName.Name()

				// Format signature from type info
				sig := "type " + typeName.Name() + " " + types.TypeString(typ.Underlying(), nil)

				pkgPath := pkg.Identifier.PkgPath
				if byPkg[pkgPath] == nil {
					byPkg[pkgPath] = &PackageTypes{
						Package: pkgPath,
						Dir:     pkg.Identifier.PkgDir,
					}
				}
				byPkg[pkgPath].Types = append(byPkg[pkgPath].Types, TypeInfo{
					Symbol:     symbolKey,
					Signature:  sig,
					Definition: fmt.Sprintf("%s:%d", filepath.Base(pos.Filename), pos.Line),
					Refs:       getSymbolRefs(wc, symbolKey),
				})
			}
		}
	}

	// Convert map to sorted slice
	var result []PackageTypes
	var pkgPaths []string
	for pkgPath := range byPkg {
		pkgPaths = append(pkgPaths, pkgPath)
	}
	sort.Strings(pkgPaths)
	for _, pkgPath := range pkgPaths {
		result = append(result, *byPkg[pkgPath])
	}

	return result
}

// findConsumers finds functions and methods that accept the interface as a parameter.
// This helps distinguish consumers (who depend on the contract) from implementers (who fulfill it).
func (c *SymbolCommand) findConsumers(wc *commands.Wildcat, target *golang.Symbol) []PackageFunctions {
	// Verify it's an interface using shared helper
	if golang.GetInterfaceType(target) == nil {
		return nil
	}

	// Get the types.Object for parameter type comparison
	obj := golang.GetTypesObject(target)
	if obj == nil {
		wc.Diagnostics = append(wc.Diagnostics, commands.Diagnostics{
			Level:   "warning",
			Package: target.Package.Identifier.PkgPath,
			Message: fmt.Sprintf("consumers analysis incomplete: type info unavailable for %s", target.Name),
		})
		return nil
	}

	// Group consumers by package
	byPkg := make(map[string]*PackageFunctions)

	// Search all packages for functions that accept this interface
	for _, pkg := range wc.Project.Packages {
		for _, file := range pkg.Package.Syntax {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Type == nil || fn.Type.Params == nil {
					continue
				}

				// Check if any parameter is the target interface
				if !c.hasInterfaceParam(pkg, fn, obj) {
					continue
				}

				// Build function info
				pos := pkg.Package.Fset.Position(fn.Pos())
				endPos := pkg.Package.Fset.Position(fn.End())

				// Build qualified symbol
				symbol := pkg.Identifier.Name + "."
				if fn.Recv != nil && len(fn.Recv.List) > 0 {
					symbol += golang.ReceiverTypeName(fn.Recv.List[0].Type) + "."
				}
				symbol += fn.Name.Name

				pkgPath := pkg.Identifier.PkgPath
				if byPkg[pkgPath] == nil {
					byPkg[pkgPath] = &PackageFunctions{
						Package: pkgPath,
						Dir:     pkg.Identifier.PkgDir,
					}
				}
				byPkg[pkgPath].Functions = append(byPkg[pkgPath].Functions, FunctionInfo{
					Symbol:     symbol,
					Signature:  golang.FormatNode(fn),
					Definition: fmt.Sprintf("%s:%d:%d", filepath.Base(pos.Filename), pos.Line, endPos.Line),
					Refs:       getSymbolRefs(wc, symbol),
				})
			}
		}
	}

	// Convert map to sorted slice
	var result []PackageFunctions
	var pkgPaths []string
	for pkgPath := range byPkg {
		pkgPaths = append(pkgPaths, pkgPath)
	}
	sort.Strings(pkgPaths)
	for _, pkgPath := range pkgPaths {
		result = append(result, *byPkg[pkgPath])
	}

	return result
}

// hasInterfaceParam checks if a function has a parameter of the target interface type.
func (c *SymbolCommand) hasInterfaceParam(pkg *golang.Package, fn *ast.FuncDecl, targetObj types.Object) bool {
	for _, field := range fn.Type.Params.List {
		// Resolve the type of the parameter
		paramType := pkg.Package.TypesInfo.TypeOf(field.Type)
		if paramType == nil {
			continue
		}

		// Check if it's the target interface (or a pointer to it)
		if named, ok := paramType.(*types.Named); ok {
			if named.Obj() == targetObj {
				return true
			}
		}
		// Check pointer to interface
		if ptr, ok := paramType.(*types.Pointer); ok {
			if named, ok := ptr.Elem().(*types.Named); ok {
				if named.Obj() == targetObj {
					return true
				}
			}
		}
	}
	return false
}

// findDescendants finds types that would be orphaned if the target type is removed.
// A descendant is a type that is only referenced by the target.
func (c *SymbolCommand) findDescendants(wc *commands.Wildcat, target *golang.Symbol) []DescendantInfo {
	// Only applies to types
	if target.Kind != golang.SymbolKindType && target.Kind != golang.SymbolKindInterface {
		return nil
	}

	node := target.Node()
	typeSpec, ok := node.(*ast.TypeSpec)
	if !ok {
		return nil
	}

	// Find all types referenced by the target's definition
	referencedTypes := c.findReferencedTypes(wc, target.Package, typeSpec)
	if len(referencedTypes) == 0 {
		return nil
	}

	// Build set of symbols that are "part of" the target (target + its methods)
	targetSymbols := make(map[string]bool)
	targetSymbolName := target.Package.Identifier.Name + "." + target.Name
	targetSymbols[targetSymbolName] = true

	// Add target's methods
	for _, fn := range golang.FindMethods(target.Package, target.Name) {
		methodName := target.Package.Identifier.Name + "." + target.Name + "." + fn.Name.Name
		targetSymbols[methodName] = true
	}

	// For each referenced type, find all symbols that reference it
	var descendants []DescendantInfo
	for _, refType := range referencedTypes {
		// Skip if not in our project
		if refType.pkg == nil {
			continue
		}

		// Get all symbols that reference this type
		referrers := c.countTypeReferencingSymbols(wc, refType)

		// Check if all referrers are part of the target
		allInTarget := true
		for referrer := range referrers {
			if !targetSymbols[referrer] {
				allInTarget = false
				break
			}
		}

		// If all referrers are within target's scope, it's a descendant
		if allInTarget && len(referrers) > 0 {
			// Use last segment of import path for symbol key (matches how Lookup parses short names)
			pkgShortName := refType.pkg.Identifier.PkgPath
			if lastSlash := strings.LastIndex(pkgShortName, "/"); lastSlash >= 0 {
				pkgShortName = pkgShortName[lastSlash+1:]
			}
			symbolKey := pkgShortName + "." + refType.name
			descendants = append(descendants, DescendantInfo{
				Symbol:     symbolKey,
				Signature:  refType.signature,
				Definition: refType.definition,
				Reason:     fmt.Sprintf("only referenced by %s", target.Name),
				Refs:       getSymbolRefs(wc, symbolKey),
			})
		}
	}

	return descendants
}

type referencedType struct {
	name       string
	pkg        *golang.Package
	obj        types.Object
	signature  string
	definition string
}

// findReferencedTypes extracts all types referenced in a type's definition.
func (c *SymbolCommand) findReferencedTypes(wc *commands.Wildcat, pkg *golang.Package, typeSpec *ast.TypeSpec) []referencedType {
	var refs []referencedType
	seen := make(map[types.Object]bool)

	ast.Inspect(typeSpec.Type, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}

		obj := pkg.Package.TypesInfo.Uses[ident]
		if obj == nil {
			return true
		}

		// Only interested in type names
		_, isTypeName := obj.(*types.TypeName)
		if !isTypeName {
			return true
		}

		// Skip if already seen
		if seen[obj] {
			return true
		}
		seen[obj] = true

		// Skip builtin types
		if obj.Pkg() == nil {
			return true
		}

		// Find the package in our project
		var refPkg *golang.Package
		for _, p := range wc.Project.Packages {
			if p.Package.Types.Path() == obj.Pkg().Path() {
				refPkg = p
				break
			}
		}

		// Skip types not in our project (stdlib, external deps)
		if refPkg == nil {
			return true
		}

		// Get signature and definition
		// Use last segment of import path for lookup (matches how Lookup parses short names)
		pkgShortName := refPkg.Identifier.PkgPath
		if lastSlash := strings.LastIndex(pkgShortName, "/"); lastSlash >= 0 {
			pkgShortName = pkgShortName[lastSlash+1:]
		}
		symbolKey := pkgShortName + "." + obj.Name()
		matches := wc.Index.Lookup(symbolKey)
		if len(matches) != 1 {
			// Can't resolve this type (e.g., type parameter like T in generics) - skip it
			return true
		}
		refs = append(refs, referencedType{
			name:       obj.Name(),
			pkg:        refPkg,
			obj:        obj,
			signature:  matches[0].Signature(),
			definition: fmt.Sprintf("%s:%s", matches[0].Filename(), matches[0].Location()),
		})

		return true
	})

	return refs
}

// countTypeReferencingSymbols counts unique symbols that reference a type.
// Returns the set of symbol names (pkg.Type or pkg.Func) that reference this type.
func (c *SymbolCommand) countTypeReferencingSymbols(wc *commands.Wildcat, refType referencedType) map[string]bool {
	referrers := make(map[string]bool)

	for _, pkg := range wc.Project.Packages {
		for _, file := range pkg.Package.Syntax {
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					symbolName := pkg.Identifier.Name + "."
					if d.Recv != nil && len(d.Recv.List) > 0 {
						symbolName += golang.ReceiverTypeName(d.Recv.List[0].Type) + "."
					}
					symbolName += d.Name.Name

					if c.nodeReferencesType(pkg, d, refType) {
						referrers[symbolName] = true
					}

				case *ast.GenDecl:
					for _, spec := range d.Specs {
						switch s := spec.(type) {
						case *ast.TypeSpec:
							symbolName := pkg.Identifier.Name + "." + s.Name.Name
							if c.nodeReferencesType(pkg, s, refType) {
								referrers[symbolName] = true
							}
						case *ast.ValueSpec:
							for _, name := range s.Names {
								symbolName := pkg.Identifier.Name + "." + name.Name
								if c.nodeReferencesType(pkg, s, refType) {
									referrers[symbolName] = true
								}
							}
						}
					}
				}
			}
		}
	}

	return referrers
}

// nodeReferencesType checks if an AST node references the given type.
func (c *SymbolCommand) nodeReferencesType(pkg *golang.Package, node ast.Node, refType referencedType) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}

		obj := pkg.Package.TypesInfo.Uses[ident]
		if obj == nil {
			return true
		}

		if obj == refType.obj || golang.SameObject(obj, refType.obj) {
			found = true
			return false
		}
		return true
	})
	return found
}

func (c *SymbolCommand) findSatisfies(wc *commands.Wildcat, target *golang.Symbol) []PackageTypes {
	// Use precomputed Satisfies from PackageSymbol
	pkgSym := target.PackageSymbol()
	if pkgSym == nil {
		return nil
	}

	// Group satisfies by package
	byPkg := make(map[string]*PackageTypes)

	for _, ifaceSym := range pkgSym.Satisfies {
		pkgPath := ifaceSym.Package.PkgPath
		symbolKey := ifaceSym.Package.Name + "." + ifaceSym.Name

		// Count implementations from precomputed ImplementedBy
		projectCount := len(ifaceSym.ImplementedBy)
		packageCount := 0
		for _, impl := range ifaceSym.ImplementedBy {
			if impl.Package.PkgPath == target.Package.Identifier.PkgPath {
				packageCount++
			}
		}

		// Find the Package for directory info
		var pkgDir string
		for _, p := range wc.Project.Packages {
			if p.Identifier.PkgPath == pkgPath {
				pkgDir = p.Identifier.PkgDir
				break
			}
		}
		// Check stdlib if not found in project
		if pkgDir == "" {
			for _, p := range wc.Stdlib {
				if p.Identifier.PkgPath == pkgPath {
					pkgDir = p.Identifier.PkgDir
					break
				}
			}
		}

		if byPkg[pkgPath] == nil {
			byPkg[pkgPath] = &PackageTypes{
				Package: pkgPath,
				Dir:     pkgDir,
			}
		}
		byPkg[pkgPath].Types = append(byPkg[pkgPath].Types, TypeInfo{
			Symbol:     symbolKey,
			Signature:  ifaceSym.Signature(),
			Definition: ifaceSym.FileLocation(),
			Impls: &ImplCounts{
				Package: packageCount,
				Project: projectCount,
			},
		})
	}

	// Convert map to sorted slice
	var result []PackageTypes
	var pkgPaths []string
	for pkgPath := range byPkg {
		pkgPaths = append(pkgPaths, pkgPath)
	}
	sort.Strings(pkgPaths)
	for _, pkgPath := range pkgPaths {
		result = append(result, *byPkg[pkgPath])
	}

	return result
}
