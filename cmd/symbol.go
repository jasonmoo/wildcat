package cmd

import (
	"context"
	"fmt"
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
	rootCmd.AddCommand(symbolCmd)

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

	var debugBuf *lsp.DebugBuffer
	if globalDebug {
		debugBuf = lsp.NewDebugBuffer()
		client.DebugLog = debugBuf.Log
	}

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

	// Dump debug logs if enabled and we got nil packages (likely race condition)
	if debugBuf != nil {
		for _, resp := range responses {
			if resp.Packages == nil {
				fmt.Fprintf(os.Stderr, "\n=== DEBUG: nil packages for %s - dumping logs ===\n", resp.Query.Target)
				debugBuf.Dump()
				fmt.Fprintf(os.Stderr, "=== END DEBUG ===\n\n")
				break
			}
		}
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

	// Determine symbol kind for display
	kind := "symbol"
	switch resolved.Kind {
	case lsp.SymbolKindFunction:
		kind = "function"
	case lsp.SymbolKindMethod:
		kind = "method"
	case lsp.SymbolKindClass, lsp.SymbolKindStruct:
		kind = "type"
	case lsp.SymbolKindInterface:
		kind = "interface"
	case lsp.SymbolKindVariable:
		kind = "variable"
	case lsp.SymbolKindConstant:
		kind = "constant"
	}

	// Get target file info
	targetFile := lsp.URIToPath(resolved.URI)
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
				Direction:    traverse.Up,
				MaxDepth:     1,
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
				fileName := filepath.Base(file)
				snippet, snippetStart, snippetEnd, _ := extractor.ExtractSmart(file, line)
				implementations = append(implementations, output.SymbolLocation{
					Location: fmt.Sprintf("%s:%d", absFile, line),
					Symbol:   "", // TODO: get type name
					Snippet: output.Snippet{
						Location: fmt.Sprintf("%s:%d:%d", fileName, snippetStart, snippetEnd),
						Source:   snippet,
					},
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
					fileName := filepath.Base(file)
					snippet, snippetStart, snippetEnd, _ := extractor.ExtractSmart(file, line)
					satisfies = append(satisfies, output.SymbolLocation{
						Location: fmt.Sprintf("%s:%d", absFile, line),
						Symbol:   st.Name,
						Snippet: output.Snippet{
							Location: fmt.Sprintf("%s:%d:%d", fileName, snippetStart, snippetEnd),
							Source:   snippet,
						},
					})
				}
			}
		}
	}

	// Get imported_by - packages that import the target package
	importedBy, _ := findImportedBy(workDir, targetPkgInfo.importPath)
	var importedByPkgs []string
	for _, dep := range importedBy {
		importedByPkgs = append(importedByPkgs, dep.Package)
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
			Symbol: resolved.Name,
			Kind:   kind,
			File:   output.AbsolutePath(targetFile),
			Line:   resolved.Position.Line + 1,
		},
		ImportedBy:        importedByPkgs,
		Packages:          packages,
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
