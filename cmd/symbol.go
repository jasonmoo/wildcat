package cmd

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jasonmoo/wildcat/internal/errors"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/lsp"
	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/jasonmoo/wildcat/internal/symbols"
	"github.com/jasonmoo/wildcat/internal/traverse"
	"github.com/spf13/cobra"
)

var symbolCmd = &cobra.Command{
	Use:   "symbol <symbol>...",
	Short: "Complete symbol analysis: definition, callers, refs, interfaces",
	Long: `Full profile of a symbol: everything you need to understand and modify it.

Returns:
  - Definition location and signature
  - Direct callers (who calls this)
  - All references (type usage, not just calls)
  - Interface relationships (satisfies/implements)

Examples:
  wildcat symbol config.Config                  # callers across project (default)
  wildcat symbol --scope package Server.Start   # callers in target package only
  wildcat symbol --scope cmd,lsp Handler        # callers in specific packages
  wildcat symbol --scope -internal/lsp Config     # exclude a package
  wildcat symbol FileURI URIToPath              # multiple symbols`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSymbol,
}

var (
	symbolExcludeTests bool
	symbolScope        string
)

func init() {
	symbolCmd.Flags().BoolVar(&symbolExcludeTests, "exclude-tests", false, "Exclude test files")
	symbolCmd.Flags().StringVar(&symbolScope, "scope", "project", "Scope: 'project', 'package', packages, or -pkg to exclude")
}

func runSymbol(cmd *cobra.Command, args []string) error {
	writer, err := GetWriter(os.Stdout)
	if err != nil {
		return fmt.Errorf("invalid output format: %w", err)
	}

	// Get working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Start LSP client
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	config, err := GetServerConfig(workDir)
	if err != nil {
		return writer.WriteError(
			string(errors.CodeServerNotFound),
			err.Error(),
			nil,
			nil,
		)
	}

	client, err := lsp.NewClient(ctx, config)
	if err != nil {
		return writer.WriteError(
			string(errors.CodeServerNotFound),
			fmt.Sprintf("Failed to start language server: %v", err),
			nil,
			map[string]any{"server": config.Command},
		)
	}
	defer client.Close()

	if err := client.Initialize(ctx); err != nil {
		return writer.WriteError(
			string(errors.CodeLSPError),
			fmt.Sprintf("LSP initialization failed: %v", err),
			nil,
			nil,
		)
	}
	defer client.Shutdown(ctx)

	if err := client.WaitForReady(ctx); err != nil {
		return writer.WriteError(
			string(errors.CodeLSPError),
			fmt.Sprintf("LSP server not ready: %v", err),
			nil,
			nil,
		)
	}

	// Process each symbol
	var responses []output.SymbolResponse
	for _, symbolArg := range args {
		response, err := getImpactForSymbol(ctx, client, symbolArg, symbolScope)
		if err != nil {
			// For multi-symbol queries, include error as a response
			if len(args) > 1 {
				responses = append(responses, output.SymbolResponse{
					Query: output.QueryInfo{
						Command: "symbol",
						Target:  symbolArg,
					},
					Error: err.Error(),
				})
				continue
			}
			// Single symbol - return error directly
			if we, ok := err.(*errors.WildcatError); ok {
				return writer.WriteError(string(we.Code), we.Message, we.Suggestions, we.Context)
			}
			return writer.WriteError(string(errors.CodeSymbolNotFound), err.Error(), nil, nil)
		}
		responses = append(responses, *response)
	}

	// Single symbol: return object; multiple: return array
	if len(args) == 1 {
		return writer.Write(responses[0])
	}
	return writer.Write(responses)
}

func getImpactForSymbol(ctx context.Context, client *lsp.Client, symbolArg string, scope string) (*output.SymbolResponse, error) {
	workDir, _ := os.Getwd()

	// Parse symbol
	query, err := symbols.Parse(symbolArg)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	// Resolve symbol
	resolver := symbols.NewDefaultResolver(client)
	resolved, err := resolver.Resolve(ctx, query)
	if err != nil {
		return nil, err
	}

	// Find similar symbols for navigation aid
	similarSymbols := resolver.FindSimilar(ctx, query, 5)

	// Get target file info
	targetFile := lsp.URIToPath(resolved.URI)
	targetLine := resolved.Position.Line + 1

	// Extract signature and line range
	signature, startLine, endLine := extractSymbolInfo(targetFile, targetLine, resolved.Kind, query.Name)
	definition := fmt.Sprintf("%s:%d:%d", output.AbsolutePath(targetFile), startLine, endLine)
	targetDir := filepath.Dir(targetFile)
	targetPkgInfo := getPackageInfo(targetDir)

	extractor := output.NewSnippetExtractor()

	// Collect all callers and references, grouped by directory
	type locationInfo struct {
		file    string
		line    int
		symbol  string
		inTest  bool
		isCaller bool
	}
	allLocations := []locationInfo{}
	callerLocations := make(map[string]bool) // for deduping refs

	// Get callers (for functions/methods)
	if resolved.Kind == lsp.SymbolKindFunction || resolved.Kind == lsp.SymbolKindMethod {
		items, err := client.PrepareCallHierarchy(ctx, resolved.URI, resolved.Position)
		if err == nil && len(items) > 0 {
			traverser := traverse.NewTraverser(client)
			opts := traverse.Options{
				UpDepth:      1,
				ExcludeTests: symbolExcludeTests,
			}

			callers, err := traverser.GetCallers(ctx, items[0], opts)
			if err == nil {
				for _, caller := range callers {
					callLine := caller.Line
					if len(caller.CallRanges) > 0 {
						callLine = caller.CallRanges[0].Start.Line + 1
					}
					absFile := output.AbsolutePath(caller.File)
					key := fmt.Sprintf("%s:%d", absFile, callLine)
					callerLocations[key] = true
					allLocations = append(allLocations, locationInfo{
						file:     caller.File,
						line:     callLine,
						symbol:   caller.Symbol,
						inTest:   caller.InTest,
						isCaller: true,
					})
				}
			}
		}
	}

	// Get references (excluding those already in callers)
	refs, err := client.References(ctx, resolved.URI, resolved.Position, false)
	if err == nil {
		for _, ref := range refs {
			file := lsp.URIToPath(ref.URI)
			isTest := output.IsTestFile(file)
			if symbolExcludeTests && isTest {
				continue
			}
			line := ref.Range.Start.Line + 1
			absFile := output.AbsolutePath(file)
			key := fmt.Sprintf("%s:%d", absFile, line)
			if callerLocations[key] {
				continue
			}
			allLocations = append(allLocations, locationInfo{
				file:     file,
				line:     line,
				symbol:   "", // references don't have a symbol name
				inTest:   isTest,
				isCaller: false,
			})
		}
	}

	// Group locations by package directory
	type pkgData struct {
		info       pkgInfo
		callers    []output.Location
		references []output.Location
		inTests    int
	}
	pkgMap := make(map[string]*pkgData)

	for _, loc := range allLocations {
		dir := filepath.Dir(loc.file)
		if _, ok := pkgMap[dir]; !ok {
			pkgMap[dir] = &pkgData{info: getPackageInfo(dir)}
		}
		pd := pkgMap[dir]

		// Build location
		fileName := filepath.Base(loc.file)
		snippet, snippetStart, snippetEnd, _ := extractor.ExtractSmart(loc.file, loc.line)
		oloc := output.Location{
			Location: fmt.Sprintf("%s:%d", fileName, loc.line),
			Symbol:   loc.symbol,
			Snippet: output.Snippet{
				Location: fmt.Sprintf("%s:%d:%d", fileName, snippetStart, snippetEnd),
				Source:   snippet,
			},
		}

		if loc.isCaller {
			pd.callers = append(pd.callers, oloc)
		} else {
			pd.references = append(pd.references, oloc)
		}
		if loc.inTest {
			pd.inTests++
		}
	}

	// Merge locations within same declaration scope
	for dir, pd := range pkgMap {
		pd.callers = extractor.MergeLocations(dir, pd.callers)
		pd.references = extractor.MergeLocations(dir, pd.references)
	}

	// Get project module root for scope filtering
	projectRoot := ""
	if modPath, err := golang.ResolvePackagePath(".", workDir); err == nil {
		// Get the module path (strip any subpackage path)
		projectRoot = modPath
	}

	// Build PackageUsage list, with target package first
	var packages []output.PackageUsage
	var pkgDirs []string
	for dir := range pkgMap {
		pkgDirs = append(pkgDirs, dir)
	}
	sort.Strings(pkgDirs)

	// Move target package to front
	for i, dir := range pkgDirs {
		if dir == targetDir {
			pkgDirs = append([]string{dir}, append(pkgDirs[:i], pkgDirs[i+1:]...)...)
			break
		}
	}

	// Resolve scope packages to full import paths
	scopeResolved := resolveScopePackages(scope, targetPkgInfo.importPath, workDir)

	// Track filtered packages and their test counts for query summary
	var filteredTests int
	for _, dir := range pkgDirs {
		pd := pkgMap[dir]
		// Filter by scope
		if scopeResolved.includes == nil {
			// Project scope: match packages with project prefix
			if !strings.HasPrefix(pd.info.importPath, projectRoot) {
				continue
			}
		} else if !scopeResolved.includes[pd.info.importPath] {
			continue
		}
		// Apply excludes
		if scopeResolved.excludes[pd.info.importPath] {
			continue
		}
		packages = append(packages, output.PackageUsage{
			Package:    pd.info.importPath,
			Dir:        pd.info.dir,
			Callers:    pd.callers,
			References: pd.references,
		})
		filteredTests += pd.inTests
	}

	// Get implementations (for interfaces)
	var implementations []output.SymbolLocation
	if resolved.Kind == lsp.SymbolKindInterface {
		impls, err := client.Implementation(ctx, resolved.URI, resolved.Position)
		if err == nil {
			for _, impl := range impls {
				file := lsp.URIToPath(impl.URI)
				isTest := output.IsTestFile(file)
				if symbolExcludeTests && isTest {
					continue
				}
				line := impl.Range.Start.Line + 1
				absFile := output.AbsolutePath(file)
				implementations = append(implementations, output.SymbolLocation{
					Location:  fmt.Sprintf("%s:%d", absFile, line),
					Symbol:    "", // TODO: get type name
					Signature: "", // TODO: get signature
				})
			}
		}
	}

	// Get satisfies (for types)
	var satisfies []output.SymbolLocation
	if resolved.Kind == lsp.SymbolKindStruct || resolved.Kind == lsp.SymbolKindClass {
		items, err := client.PrepareTypeHierarchy(ctx, resolved.URI, resolved.Position)
		if err == nil && len(items) > 0 {
			supertypes, err := client.Supertypes(ctx, items[0])
			if err == nil {
				directDeps := golang.DirectDeps(workDir)
				for _, st := range supertypes {
					file := lsp.URIToPath(st.URI)
					if !golang.IsDirectDep(file, directDeps) {
						continue
					}
					line := st.Range.Start.Line + 1
					absFile := output.AbsolutePath(file)
					satisfies = append(satisfies, output.SymbolLocation{
						Location:  fmt.Sprintf("%s:%d", absFile, line),
						Symbol:    st.Name,
						Signature: "", // TODO: get signature
					})
				}
			}
		}
	}

	// Get imported_by - packages that actually use this symbol (not just import the package)
	// Derived from packages that have callers or references to the symbol
	var importedByPkgs []output.DepResult
	for _, pkg := range packages {
		// Skip the target package itself
		if pkg.Package == targetPkgInfo.importPath {
			continue
		}
		// Only include if there are actual usages
		if len(pkg.Callers) > 0 || len(pkg.References) > 0 {
			// Use first caller or reference location
			var location string
			if len(pkg.Callers) > 0 {
				location = filepath.Join(pkg.Dir, pkg.Callers[0].Location)
			} else if len(pkg.References) > 0 {
				location = filepath.Join(pkg.Dir, pkg.References[0].Location)
			}
			importedByPkgs = append(importedByPkgs, output.DepResult{
				Package:  pkg.Package,
				Location: location,
			})
		}
	}

	// Compute summaries
	var projectCallers, projectRefs, projectTests int
	var pkgCallers, pkgRefs, pkgTests int
	for dir, pd := range pkgMap {
		projectCallers += len(pd.callers)
		projectRefs += len(pd.references)
		projectTests += pd.inTests
		if dir == targetDir {
			pkgCallers = len(pd.callers)
			pkgRefs = len(pd.references)
			pkgTests = pd.inTests
		}
	}

	// Query summary reflects the filtered packages array
	var queryCallers, queryRefs int
	for _, pkg := range packages {
		queryCallers += len(pkg.Callers)
		queryRefs += len(pkg.References)
	}

	querySummary := output.SymbolSummary{
		Callers:         queryCallers,
		References:      queryRefs,
		Implementations: len(implementations),
		Satisfies:       len(satisfies),
		InTests:         filteredTests,
	}
	packageSummary := output.SymbolSummary{
		Callers:         pkgCallers,
		References:      pkgRefs,
		Implementations: len(implementations),
		Satisfies:       len(satisfies),
		InTests:         pkgTests,
	}
	projectSummary := output.SymbolSummary{
		Callers:         projectCallers,
		References:      projectRefs,
		Implementations: len(implementations),
		Satisfies:       len(satisfies),
		InTests:         projectTests,
	}

	// Normalize scope for output - empty means "package"
	outputScope := scope
	if outputScope == "" {
		outputScope = "package"
	}

	return &output.SymbolResponse{
		Query: output.QueryInfo{
			Command:  "symbol",
			Target:   query.Raw,
			Resolved: resolved.Name,
			Scope:    outputScope,
		},
		Target: output.TargetInfo{
			Symbol:     resolved.Name,
			Signature:  signature,
			Definition: definition,
		},
		ImportedBy:        importedByPkgs,
		References:        packages,
		Implementations:   implementations,
		Satisfies:         satisfies,
		QuerySummary:      querySummary,
		PackageSummary:    packageSummary,
		ProjectSummary:    projectSummary,
		OtherFuzzyMatches: similarSymbols,
	}, nil
}

// pkgInfo holds package metadata
type pkgInfo struct {
	importPath string
	dir        string
}

// getPackageInfo gets package info for a directory
func getPackageInfo(dir string) pkgInfo {
	absDir := output.AbsolutePath(dir)
	// Try go list to get import path
	if importPath, err := golang.ResolvePackagePath(dir, dir); err == nil {
		return pkgInfo{importPath: importPath, dir: absDir}
	}
	// Fallback to directory path
	return pkgInfo{importPath: absDir, dir: absDir}
}

// scopeFilter holds parsed include/exclude package patterns.
type scopeFilter struct {
	includes []string // packages to include (empty means "project")
	excludes []string // packages to exclude (- prefixed)
}

// parseScopeFilter parses a scope string into includes and excludes.
func parseScopeFilter(scope string) scopeFilter {
	var filter scopeFilter
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

// resolvedScope holds resolved include/exclude package paths.
type resolvedScope struct {
	includes    map[string]bool // nil means project scope (check prefix)
	excludes    map[string]bool // packages to exclude
	packageOnly bool            // true if scope is "package" (target only)
}

// resolveScopePackages resolves scope to include/exclude package import paths.
// Scope can be:
//   - "": default to target package only
//   - "project": project scope (caller checks project prefix)
//   - "pkg1,pkg2,...": comma-separated packages
//   - "-pkg": exclude package (can combine with others)
//
// Examples:
//
//	"project,-internal/test" -> project scope minus internal/test
//	"cmd,lsp,-lsp/test"      -> cmd and lsp minus lsp/test
func resolveScopePackages(scope, targetPkgImportPath, workDir string) resolvedScope {
	if scope == "package" || scope == "" {
		// Target package only
		return resolvedScope{
			includes:    map[string]bool{targetPkgImportPath: true},
			packageOnly: true,
		}
	}

	// Parse scope into includes and excludes
	filter := parseScopeFilter(scope)

	// Resolve excludes
	excludes := make(map[string]bool)
	for _, pkg := range filter.excludes {
		if resolved, err := golang.ResolvePackagePath(pkg, workDir); err == nil {
			excludes[resolved] = true
		}
	}

	// No explicit includes means project scope
	if len(filter.includes) == 0 {
		return resolvedScope{
			includes: nil, // signals project scope
			excludes: excludes,
		}
	}

	// Resolve includes
	includes := make(map[string]bool)
	for _, pkg := range filter.includes {
		if resolved, err := golang.ResolvePackagePath(pkg, workDir); err == nil {
			includes[resolved] = true
		}
	}

	return resolvedScope{
		includes: includes,
		excludes: excludes,
	}
}

// extractSymbolInfo extracts signature and line range for a symbol at the given location.
func extractSymbolInfo(filePath string, line int, kind lsp.SymbolKind, symbolName string) (signature string, startLine, endLine int) {
	if !strings.HasSuffix(filePath, ".go") {
		return "", line, line
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return "", line, line
	}

	// For methods like Type.Method, extract just the method name
	simpleName := symbolName
	if idx := strings.LastIndex(symbolName, "."); idx >= 0 {
		simpleName = symbolName[idx+1:]
	}

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if kind != lsp.SymbolKindFunction && kind != lsp.SymbolKindMethod {
				continue
			}
			pos := fset.Position(d.Pos())
			if pos.Line >= line-1 && pos.Line <= line+1 && d.Name.Name == simpleName {
				endPos := fset.Position(d.End())
				return renderFuncSignature(d), pos.Line, endPos.Line
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if kind != lsp.SymbolKindClass && kind != lsp.SymbolKindStruct && kind != lsp.SymbolKindInterface {
						continue
					}
					pos := fset.Position(s.Pos())
					if pos.Line >= line-1 && pos.Line <= line+1 && s.Name.Name == simpleName {
						endPos := fset.Position(s.End())
						return renderTypeSignature(s), pos.Line, endPos.Line
					}
				case *ast.ValueSpec:
					if kind != lsp.SymbolKindConstant && kind != lsp.SymbolKindVariable {
						continue
					}
					// Check if any name in the spec matches
					for _, name := range s.Names {
						if name.Name == simpleName {
							pos := fset.Position(s.Pos())
							endPos := fset.Position(s.End())
							return renderValueSignature(d.Tok, s, simpleName), pos.Line, endPos.Line
						}
					}
				}
			}
		}
	}

	return "", line, line
}

// renderFuncSignature renders a function declaration as a one-line signature.
func renderFuncSignature(decl *ast.FuncDecl) string {
	cleaned := *decl
	cleaned.Doc = nil
	cleaned.Body = nil

	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), &cleaned); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

// renderTypeSignature renders a type declaration as a signature.
func renderTypeSignature(spec *ast.TypeSpec) string {
	var buf bytes.Buffer
	buf.WriteString("type ")
	buf.WriteString(spec.Name.Name)
	buf.WriteString(" ")

	switch t := spec.Type.(type) {
	case *ast.StructType:
		buf.WriteString("struct {...}")
	case *ast.InterfaceType:
		buf.WriteString("interface {...}")
	case *ast.Ident:
		buf.WriteString(t.Name)
	default:
		if err := format.Node(&buf, token.NewFileSet(), spec.Type); err == nil {
			return buf.String()
		}
		buf.WriteString("...")
	}
	return buf.String()
}

// renderValueSignature renders a const/var declaration as a signature.
func renderValueSignature(tok token.Token, spec *ast.ValueSpec, name string) string {
	var buf bytes.Buffer
	if tok == token.CONST {
		buf.WriteString("const ")
	} else {
		buf.WriteString("var ")
	}

	buf.WriteString(name)
	if spec.Type != nil {
		buf.WriteString(" ")
		format.Node(&buf, token.NewFileSet(), spec.Type)
	}
	return buf.String()
}
