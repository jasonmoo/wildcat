package symbol_cmd

import (
	"context"
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/spf13/cobra"
)

type SymbolCommand struct {
	symbol string
	scope  string
}

var _ commands.Command[*SymbolCommand] = (*SymbolCommand)(nil)

func WithSymbol(s string) func(*SymbolCommand) error {
	return func(c *SymbolCommand) error {
		c.symbol = s
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
	sym := wc.Index.Lookup(symbolKey)
	if sym == nil {
		return nil
	}
	counts := golang.CountReferences(wc.Project.Packages, sym)
	return &SymbolRefs{
		Internal: counts.Internal,
		External: counts.External,
		Packages: counts.PackageCount(),
	}
}

// getSymbolRefsOutput looks up a symbol and returns its reference counts as output.TargetRefs.
func getSymbolRefsOutput(wc *commands.Wildcat, symbolKey string) *output.TargetRefs {
	sym := wc.Index.Lookup(symbolKey)
	if sym == nil {
		return nil
	}
	counts := golang.CountReferences(wc.Project.Packages, sym)
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
		Use:   "symbol <symbol>",
		Short: "Complete symbol analysis: definition, callers, refs, interfaces",
		Long: `Full profile of a symbol: everything you need to understand and modify it.

Returns:
  - Definition location and signature
  - Direct callers (who calls this)
  - All references (type usage, not just calls)
  - Interface relationships (satisfies/implements)

Scope:
  project       - All project packages (default)
  package       - Target package only
  pkg1,pkg2     - Specific packages
  -pkg          - Exclude package

Examples:
  wildcat symbol Config                         # analyze Config type
  wildcat symbol --scope package Server.Start   # callers in target package only
  wildcat symbol --scope cmd,lsp Handler        # callers in specific packages
  wildcat symbol --scope -internal/lsp Config   # exclude a package`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wc, err := commands.LoadWildcat(cmd.Context(), ".")
			if err != nil {
				return err
			}

			result, err := c.Execute(cmd.Context(), wc,
				WithSymbol(args[0]),
				WithScope(scope),
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

	cmd.Flags().StringVar(&scope, "scope", "project", "Scope: 'project', 'package', packages, or -pkg to exclude")

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

	if c.symbol == "" {
		return commands.NewErrorResultf("invalid_symbol", "symbol is required"), nil
	}

	if i := strings.IndexByte(c.symbol, '.'); i > -1 {
		path, name := c.symbol[:i], c.symbol[i+1:]
		pi, err := wc.Project.ResolvePackageName(ctx, path)
		if err == nil {
			c.symbol = pi.PkgPath + "." + name
		}
	}

	// Find target symbol
	target := wc.Index.Lookup(c.symbol)
	if target == nil {
		return wc.NewSymbolNotFoundErrorResponse(c.symbol), nil
	}

	// Build target info
	sig, _ := target.Signature()
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
		c.findCallers(wc, target, scopeFilter, usageByPkg)
	}

	// Find methods and constructors for types (before references so we can exclude them)
	var methods, constructors []FunctionInfo
	excludeFromRefs := make(map[string]bool) // file:line keys to exclude from references
	if target.Kind == golang.SymbolKindType || target.Kind == golang.SymbolKindInterface {
		foundMethods := golang.FindMethods(target.Package, target.Name)
		foundConstructors := golang.FindConstructors(target.Package, target.Name)
		for _, fn := range foundMethods {
			sig, _ := golang.FormatFuncDecl(fn)
			pos := target.Package.Package.Fset.Position(fn.Pos())
			endPos := target.Package.Package.Fset.Position(fn.End())
			// Build qualified symbol: pkg.Type.Method
			methodSymbol := target.Package.Identifier.Name + "." + target.Name + "." + fn.Name.Name
			methods = append(methods, FunctionInfo{
				Symbol:     methodSymbol,
				Signature:  sig,
				Definition: fmt.Sprintf("%s:%d:%d", filepath.Base(pos.Filename), pos.Line, endPos.Line),
				Refs:       getSymbolRefs(wc, methodSymbol),
			})
			excludeFromRefs[fmt.Sprintf("%s:%d", pos.Filename, pos.Line)] = true
		}
		for _, fn := range foundConstructors {
			sig, _ := golang.FormatFuncDecl(fn)
			pos := target.Package.Package.Fset.Position(fn.Pos())
			endPos := target.Package.Package.Fset.Position(fn.End())
			// Build qualified symbol: pkg.FuncName
			constructorSymbol := target.Package.Identifier.Name + "." + fn.Name.Name
			constructors = append(constructors, FunctionInfo{
				Symbol:     constructorSymbol,
				Signature:  sig,
				Definition: fmt.Sprintf("%s:%d:%d", filepath.Base(pos.Filename), pos.Line, endPos.Line),
				Refs:       getSymbolRefs(wc, constructorSymbol),
			})
			excludeFromRefs[fmt.Sprintf("%s:%d", pos.Filename, pos.Line)] = true
		}
	}

	// Find references (for all symbol types)
	c.findReferences(wc, target, scopeFilter, usageByPkg)

	// Find interface relationships
	var implementations []PackageTypes
	var satisfies []PackageTypes

	if target.Kind == golang.SymbolKindInterface {
		implementations = c.findImplementations(wc, target)
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

	// Build package usages
	for _, pkgPath := range pkgPaths {
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
	suggestions := wc.Suggestions(c.symbol, &golang.SearchOptions{Limit: 5, Exclude: excludeSymbols})
	fuzzyMatches := make([]SuggestionInfo, len(suggestions))
	for i, s := range suggestions {
		fuzzyMatches[i] = SuggestionInfo{Symbol: s.Symbol, Kind: s.Kind}
	}

	return &SymbolCommandResponse{
		Query: output.QueryInfo{
			Command:  "symbol",
			Target:   c.symbol,
			Resolved: qualifiedSymbol,
			Scope:    c.scope,
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

func (c *SymbolCommand) findCallers(wc *commands.Wildcat, target *golang.Symbol, filter *commands.ScopeFilter, usageByPkg map[string]*pkgUsage) {
	// Get the target's types.Object for comparison
	node := target.Node()
	funcDecl, ok := node.(*ast.FuncDecl)
	if !ok || funcDecl.Body == nil {
		return
	}

	targetObj := target.Package.Package.TypesInfo.Defs[funcDecl.Name]
	if targetObj == nil {
		return
	}

	// Search all packages for calls to target
	for _, pkg := range wc.Project.Packages {
		if !filter.InScope(pkg.Identifier.PkgPath) {
			continue
		}

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

func (c *SymbolCommand) findReferences(wc *commands.Wildcat, target *golang.Symbol, filter *commands.ScopeFilter, usageByPkg map[string]*pkgUsage) {
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
		// Scope filter
		if !filter.InScope(ref.Package.Identifier.PkgPath) {
			return true
		}

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
	// Get the interface type
	node := target.Node()
	typeSpec, ok := node.(*ast.TypeSpec)
	if !ok {
		return nil
	}

	ifaceType, ok := typeSpec.Type.(*ast.InterfaceType)
	if !ok {
		return nil
	}

	// Get the types.Interface
	obj := target.Package.Package.TypesInfo.Defs[typeSpec.Name]
	if obj == nil {
		return nil
	}
	named, ok := obj.Type().(*types.Named)
	if !ok {
		return nil
	}
	iface, ok := named.Underlying().(*types.Interface)
	if !ok {
		return nil
	}

	_ = ifaceType // used for validation

	// Group implementations by package
	byPkg := make(map[string]*PackageTypes)

	// Search all packages for types that implement this interface
	for _, pkg := range wc.Project.Packages {
		for _, file := range pkg.Package.Syntax {
			for _, decl := range file.Decls {
				genDecl, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range genDecl.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}

					typeObj := pkg.Package.TypesInfo.Defs[ts.Name]
					if typeObj == nil {
						continue
					}

					// Check if this type implements the interface
					// Use pointer type as well since methods might be on pointer receiver
					typ := typeObj.Type()
					ptrTyp := types.NewPointer(typ)

					if types.Implements(typ, iface) || types.Implements(ptrTyp, iface) {
						// Skip the interface itself
						if typeObj == obj {
							continue
						}

						pos := pkg.Package.Fset.Position(ts.Pos())
						sig, _ := golang.FormatTypeSpec(genDecl.Tok, ts)
						symbolKey := pkg.Identifier.Name + "." + ts.Name.Name

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
		sym := wc.Index.Lookup(symbolKey)
		if sym == nil {
			// Can't resolve this type (e.g., type parameter like T in generics) - skip it
			return true
		}
		sig, _ := sym.Signature()
		def := fmt.Sprintf("%s:%s", sym.Filename(), sym.Location())

		refs = append(refs, referencedType{
			name:       obj.Name(),
			pkg:        refPkg,
			obj:        obj,
			signature:  sig,
			definition: def,
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
	// Get the type
	node := target.Node()
	typeSpec, ok := node.(*ast.TypeSpec)
	if !ok {
		return nil
	}

	typeObj := target.Package.Package.TypesInfo.Defs[typeSpec.Name]
	if typeObj == nil {
		return nil
	}

	typ := typeObj.Type()
	ptrTyp := types.NewPointer(typ)

	// Group satisfies by package
	byPkg := make(map[string]*PackageTypes)

	// Search all packages for interfaces this type implements
	for _, pkg := range wc.Project.Packages {
		for _, file := range pkg.Package.Syntax {
			for _, decl := range file.Decls {
				genDecl, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range genDecl.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}

					_, isIface := ts.Type.(*ast.InterfaceType)
					if !isIface {
						continue
					}

					ifaceObj := pkg.Package.TypesInfo.Defs[ts.Name]
					if ifaceObj == nil {
						continue
					}

					named, ok := ifaceObj.Type().(*types.Named)
					if !ok {
						continue
					}
					iface, ok := named.Underlying().(*types.Interface)
					if !ok {
						continue
					}

					// Skip empty interface
					if iface.NumMethods() == 0 {
						continue
					}

					if types.Implements(typ, iface) || types.Implements(ptrTyp, iface) {
						pos := pkg.Package.Fset.Position(ts.Pos())
						sig, _ := golang.FormatTypeSpec(genDecl.Tok, ts)
						symbolKey := pkg.Identifier.Name + "." + ts.Name.Name

						// Count implementations of this interface
						implementors := golang.FindImplementors(iface, pkg.Identifier.PkgPath, ts.Name.Name, wc.Project.Packages)
						projectCount := len(implementors)
						packageCount := 0
						for _, impl := range implementors {
							if impl.PkgPath() == target.Package.Identifier.PkgPath {
								packageCount++
							}
						}

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
							Impls: &ImplCounts{
								Package: packageCount,
								Project: projectCount,
							},
						})
					}
				}
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
