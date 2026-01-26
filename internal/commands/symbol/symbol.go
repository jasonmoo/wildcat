package symbol_cmd

import (
	"context"
	"fmt"
	"go/ast"
	"path/filepath"
	"sort"

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
			candidates = append(candidates, m.PkgPathSymbol())
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
			candidates = append(candidates, m.PkgPathSymbol())
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
	// Find target symbol
	target, errResp := wc.LookupSymbol(ctx, symbol)
	if errResp != nil {
		return errResp, nil
	}

	// Build target info
	sig := target.Signature()
	definition := target.FileDefinition()

	// Parse scope
	scopeFilter, err := wc.ParseScope(ctx, c.scope, target.PackageIdentifier.PkgPath)
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
		// Use precomputed methods/constructors from Symbol
		for _, m := range target.Methods {
			methods = append(methods, FunctionInfo{
				Symbol:     m.PkgTypeSymbol(),
				Signature:  m.Signature(),
				Definition: m.FileDefinition(),
				Refs:       getSymbolRefs(wc, m.PkgTypeSymbol()),
			})
			excludeFromRefs[m.PathLocation()] = true
		}
		for _, ctor := range target.Constructors {
			constructors = append(constructors, FunctionInfo{
				Symbol:     ctor.PkgSymbol(),
				Signature:  ctor.Signature(),
				Definition: ctor.FileDefinition(),
				Refs:       getSymbolRefs(wc, ctor.PkgSymbol()),
			})
			excludeFromRefs[ctor.PathLocation()] = true
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
	var descendants []DescendantInfo
	for _, desc := range target.Descendants {
		descendants = append(descendants, DescendantInfo{
			Symbol:     desc.PkgSymbol(),
			Signature:  desc.Signature(),
			Definition: desc.FileDefinition(),
			Reason:     fmt.Sprintf("only referenced by %s", target.Name),
			Refs:       getSymbolRefs(wc, desc.PkgSymbol()),
		})
	}

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
	targetPkgPath := target.PackageIdentifier.PkgPath
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
			snippet, start, end, err := extractor.ExtractSmart(caller.file, caller.line)
			if err != nil {
				wc.AddDiagnostic("warning", pkgPath, "snippet extraction failed for %s:%d: %v", filepath.Base(caller.file), caller.line, err)
			}
			unique, err := extractor.IsUnique(caller.file, snippet)
			if err != nil {
				wc.AddDiagnostic("warning", pkgPath, "uniqueness check failed for %s:%d: %v", filepath.Base(caller.file), caller.line, err)
			}
			callerLocs = append(callerLocs, output.Location{
				Location: fmt.Sprintf("%s:%d", filepath.Base(caller.file), caller.line),
				Symbol:   caller.symbol,
				Snippet: output.Snippet{
					Location: fmt.Sprintf("%s:%d:%d", filepath.Base(caller.file), start, end),
					Source:   snippet,
					Unique:   unique,
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
			snippet, start, end, err := extractor.ExtractSmart(ref.file, ref.line)
			if err != nil {
				wc.AddDiagnostic("warning", pkgPath, "snippet extraction failed for %s:%d: %v", filepath.Base(ref.file), ref.line, err)
			}
			unique, err := extractor.IsUnique(ref.file, snippet)
			if err != nil {
				wc.AddDiagnostic("warning", pkgPath, "uniqueness check failed for %s:%d: %v", filepath.Base(ref.file), ref.line, err)
			}
			refLocs = append(refLocs, output.Location{
				Location: fmt.Sprintf("%s:%d", filepath.Base(ref.file), ref.line),
				Symbol:   ref.symbol,
				Snippet: output.Snippet{
					Location: fmt.Sprintf("%s:%d:%d", filepath.Base(ref.file), start, end),
					Source:   snippet,
					Unique:   unique,
				},
				Refs: getSymbolRefsOutput(wc, ref.symbol),
			})
		}

		// Merge references within same AST scope to reduce duplication
		mergedRefs, mergeErrs := extractor.MergeLocations(usage.pkg.Identifier.PkgDir, refLocs)
		for _, err := range mergeErrs {
			wc.AddDiagnostic("warning", pkgPath, "uniqueness check failed during merge: %v", err)
		}

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
		target.PkgSymbol(), // target itself
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
	for _, pkg := range consumers {
		for _, fn := range pkg.Functions {
			excludeSymbols = append(excludeSymbols, fn.Symbol)
		}
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
			Resolved:      target.PkgPathSymbol(),
			Scope:         c.scope,
			ScopeResolved: scopeResolved,
		},
		Package: output.PackageInfo{
			ImportPath: target.PackageIdentifier.PkgPath,
			Name:       target.PackageIdentifier.Name,
			Dir:        target.PackageIdentifier.PkgDir,
		},
		Target: output.TargetInfo{
			Symbol:     target.PkgPathSymbol(),
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
	funcDecl, ok := target.Node.(*ast.FuncDecl)
	if !ok || funcDecl.Body == nil {
		return
	}

	if target.Object == nil {
		wc.Diagnostics = append(wc.Diagnostics, commands.NewWarningDiagnostic(target.PackageIdentifier, fmt.Sprintf("callers analysis incomplete: type info unavailable for %s", funcDecl.Name.Name)))
		return
	}
	targetObj := target.Object

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
	targetPos := target.Package.Fset.Position(target.Object.Pos())
	defLine := targetPos.Line
	targetFile := targetPos.Filename

	golang.WalkReferences(wc.Project.Packages, target, func(ref golang.Reference) bool {
		key := fmt.Sprintf("%s:%d", ref.File, ref.Line)

		// Skip if already counted as caller
		if callerLocs[key] {
			return true
		}

		// Skip the definition itself
		if ref.File == targetFile && ref.Line == defLine {
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
	// Verify it's an interface
	if golang.GetInterfaceType(target) == nil {
		return nil
	}

	// Get precomputed implementors from Symbol
	if len(target.ImplementedBy) == 0 {
		return nil
	}

	// Group implementations by package
	byPkg := make(map[string]*PackageTypes)

	for _, impl := range target.ImplementedBy {
		pkgPath := impl.Package.PkgPath

		// Find the Package for directory info
		var pkgDir string
		for _, pkg := range wc.Project.Packages {
			if pkg.Identifier.PkgPath == pkgPath {
				pkgDir = pkg.Identifier.PkgDir
				break
			}
		}

		if byPkg[pkgPath] == nil {
			byPkg[pkgPath] = &PackageTypes{
				Package: pkgPath,
				Dir:     pkgDir,
			}
		}

		byPkg[pkgPath].Types = append(byPkg[pkgPath].Types, TypeInfo{
			Symbol:     impl.PkgSymbol(),
			Signature:  impl.Signature(),
			Definition: impl.FileDefinition(),
			Refs:       getSymbolRefs(wc, impl.PkgSymbol()),
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

// findConsumers finds functions and methods that accept the interface as a parameter.
// This helps distinguish consumers (who depend on the contract) from implementers (who fulfill it).
func (c *SymbolCommand) findConsumers(wc *commands.Wildcat, target *golang.Symbol) []PackageFunctions {
	// Verify it's an interface
	if golang.GetInterfaceType(target) == nil {
		return nil
	}

	// Get precomputed consumers from Symbol
	if len(target.Consumers) == 0 {
		return nil
	}

	// Group consumers by package
	byPkg := make(map[string]*PackageFunctions)

	for _, consumer := range target.Consumers {
		pkgPath := consumer.PackageIdentifier.PkgPath

		// Find the Package for directory info
		var pkgDir string
		for _, pkg := range wc.Project.Packages {
			if pkg.Identifier.PkgPath == pkgPath {
				pkgDir = pkg.Identifier.PkgDir
				break
			}
		}

		if byPkg[pkgPath] == nil {
			byPkg[pkgPath] = &PackageFunctions{
				Package: pkgPath,
				Dir:     pkgDir,
			}
		}

		byPkg[pkgPath].Functions = append(byPkg[pkgPath].Functions, FunctionInfo{
			Symbol:     consumer.PkgTypeSymbol(),
			Signature:  consumer.Signature(),
			Definition: consumer.FileDefinition(),
			Refs:       getSymbolRefs(wc, consumer.PkgTypeSymbol()),
		})
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

func (c *SymbolCommand) findSatisfies(wc *commands.Wildcat, target *golang.Symbol) []PackageTypes {
	// Use precomputed Satisfies from Symbol
	if len(target.Satisfies) == 0 {
		return nil
	}

	// Group satisfies by package
	byPkg := make(map[string]*PackageTypes)

	for _, ifaceSym := range target.Satisfies {
		pkgPath := ifaceSym.Package.PkgPath

		// Count implementations from precomputed ImplementedBy
		projectCount := len(ifaceSym.ImplementedBy)
		packageCount := 0
		for _, impl := range ifaceSym.ImplementedBy {
			if impl.PackageIdentifier.PkgPath == target.PackageIdentifier.PkgPath {
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
			Symbol:     ifaceSym.PkgSymbol(),
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
