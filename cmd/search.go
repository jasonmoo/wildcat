package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jasonmoo/wildcat/internal/errors"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/lsp"
	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Fuzzy search for symbols across the workspace",
	Long: `Search for symbols across the workspace using gopls fuzzy matching.

Semantic alternative to grep - returns actual symbols (functions, types,
methods, constants) rather than text matches. Results ranked by relevance.

Query Syntax:
  hello          Fuzzy match - matches "Hello", "helloWorld", "SayHello"
  Hello          Case-sensitive fuzzy (uppercase triggers case-sensitivity)
  DocSym         Abbreviation match - matches "DocumentSymbol"
  cfg            Abbreviation match - matches "Config"

Examples:
  wildcat search Resolve                        # project packages (default)
  wildcat search --scope all Config             # include external dependencies
  wildcat search --scope internal/lsp Client    # specific package
  wildcat search --scope -internal/lsp Config     # exclude a package
  wildcat search --limit 5 Config               # top 5 matches`,
	Args: cobra.ExactArgs(1),
	RunE: runSearch,
}

var (
	searchLimit int
	searchScope string
)

func init() {
	rootCmd.AddCommand(searchCmd)

	searchCmd.Flags().IntVar(&searchLimit, "limit", 20, "Maximum results (max 100)")
	searchCmd.Flags().StringVar(&searchScope, "scope", "project", "Scope: 'project', 'all', packages, or -pkg to exclude")
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
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

	// Query workspace symbols via gopls fuzzy matching
	symbols, err := client.WorkspaceSymbol(ctx, query)
	if err != nil {
		return writer.WriteError(
			string(errors.CodeLSPError),
			fmt.Sprintf("workspace/symbol query failed: %v", err),
			nil,
			nil,
		)
	}

	// Apply scope filter (default is "project", "all" skips filtering)
	if searchScope != "all" {
		symbols = filterSymbolsByScope(symbols, searchScope, workDir)
	}

	// Filter out /internal/ packages from external dependencies
	symbols = filterExternalInternal(symbols)

	// Apply limit
	limit := searchLimit
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}
	truncated := len(symbols) > limit
	if len(symbols) > limit {
		symbols = symbols[:limit]
	}

	// Group symbols by package
	type pkgData struct {
		dir     string
		matches []output.SearchMatch
	}
	pkgMap := make(map[string]*pkgData)
	pkgOrder := make([]string, 0)
	kindCounts := make(map[string]int)
	totalCount := 0

	for _, sym := range symbols {
		kind := sym.Kind.String()
		kindCounts[kind]++
		totalCount++

		file := output.AbsolutePath(lsp.URIToPath(sym.Location.URI))
		startLine := sym.Location.Range.Start.Line + 1

		// Get or create package entry
		pkg := sym.ContainerName
		if pkg == "" {
			pkg = "(unknown)"
		}
		data, exists := pkgMap[pkg]
		if !exists {
			// Derive dir from file path
			dir := file
			if idx := strings.LastIndex(file, "/"); idx >= 0 {
				dir = file[:idx]
			}
			data = &pkgData{dir: dir}
			pkgMap[pkg] = data
			pkgOrder = append(pkgOrder, pkg)
		}

		// Build match with short symbol name
		shortName := sym.ShortName()
		// Strip package prefix if present (since package is in parent)
		if strings.Contains(shortName, ".") {
			if idx := strings.Index(shortName, "."); idx >= 0 {
				shortName = shortName[idx+1:]
			}
		}

		// Location as "file.go:line"
		fileName := file
		if idx := strings.LastIndex(file, "/"); idx >= 0 {
			fileName = file[idx+1:]
		}
		location := fmt.Sprintf("%s:%d", fileName, startLine)

		match := output.SearchMatch{
			Location: location,
			Symbol:   shortName,
			Kind:     kind,
		}
		data.matches = append(data.matches, match)
	}

	// Build packages slice in order
	packages := make([]output.SearchPackage, 0, len(pkgOrder))
	for _, pkg := range pkgOrder {
		data := pkgMap[pkg]
		packages = append(packages, output.SearchPackage{
			Package: pkg,
			Dir:     data.dir,
			Matches: data.matches,
		})
	}

	response := output.SearchResponse{
		Query: output.SearchQuery{
			Command: "search",
			Pattern: query,
			Scope:   searchScope,
		},
		Packages: packages,
		Summary: output.SearchSummary{
			Count:     totalCount,
			ByKind:    kindCounts,
			Truncated: truncated,
		},
	}

	// Dump debug logs if enabled and we got 0 results (likely race condition)
	if debugBuf != nil && totalCount == 0 {
		fmt.Fprintf(os.Stderr, "\n=== DEBUG: 0 results - dumping logs ===\n")
		debugBuf.Dump()
		fmt.Fprintf(os.Stderr, "=== END DEBUG ===\n\n")
	}

	return writer.Write(response)
}

// scopeFilter holds parsed include/exclude package patterns.
type scopeFilter struct {
	includes []string // packages to include (empty means "project")
	excludes []string // packages to exclude (- prefixed)
}

// parseScopeFilter parses a scope string into includes and excludes.
// Excludes are prefixed with -. If no includes specified, defaults to project scope.
// Examples:
//
//	"project"                  -> includes=[], excludes=[] (project scope)
//	"cmd,lsp"                  -> includes=[cmd,lsp], excludes=[]
//	"project,-internal/test"   -> includes=[], excludes=[internal/test]
//	"-internal/test"           -> includes=[], excludes=[internal/test] (implicit project)
//	"cmd,lsp,-lsp/test"        -> includes=[cmd,lsp], excludes=[lsp/test]
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

// filterSymbolsByScope filters symbols to those in specified packages.
// Scope can be "project" (all project packages), comma-separated package paths,
// or include exclusions with - prefix (e.g., "project,-internal/test").
func filterSymbolsByScope(symbols []lsp.SymbolInformation, scope, workDir string) []lsp.SymbolInformation {
	filter := parseScopeFilter(scope)

	// Resolve project root for project scope or exclusion matching
	projectRoot, err := golang.ResolvePackagePath(".", workDir)
	if err != nil {
		return symbols // Can't determine project root, return all
	}

	// Resolve excludes to full import paths
	excludes := make(map[string]bool)
	for _, pkg := range filter.excludes {
		if resolved, err := golang.ResolvePackagePath(pkg, workDir); err == nil {
			excludes[resolved] = true
		}
	}

	// If no explicit includes, use project scope
	if len(filter.includes) == 0 {
		filtered := make([]lsp.SymbolInformation, 0, len(symbols))
		for _, sym := range symbols {
			if strings.HasPrefix(sym.ContainerName, projectRoot) && !excludes[sym.ContainerName] {
				filtered = append(filtered, sym)
			}
		}
		return filtered
	}

	// Resolve includes to full import paths
	includes := make(map[string]bool)
	for _, pkg := range filter.includes {
		if resolved, err := golang.ResolvePackagePath(pkg, workDir); err == nil {
			includes[resolved] = true
		}
	}

	filtered := make([]lsp.SymbolInformation, 0, len(symbols))
	for _, sym := range symbols {
		if includes[sym.ContainerName] && !excludes[sym.ContainerName] {
			filtered = append(filtered, sym)
		}
	}
	return filtered
}

// filterExternalInternal removes symbols from /internal/ paths in external packages and stdlib.
// Project-local /internal/ paths are kept; only external dependencies and stdlib internals are filtered.
func filterExternalInternal(symbols []lsp.SymbolInformation) []lsp.SymbolInformation {
	filtered := make([]lsp.SymbolInformation, 0, len(symbols))
	for _, sym := range symbols {
		path := lsp.URIToPath(sym.Location.URI)
		// Skip stdlib internal packages (e.g., "internal/poll", "internal/reflectlite")
		if strings.HasPrefix(sym.ContainerName, "internal/") {
			continue
		}
		// Skip external dependencies with /internal/ in their import path
		if strings.Contains(path, "/go/pkg/mod/") && strings.Contains(sym.ContainerName, "/internal/") {
			continue
		}
		filtered = append(filtered, sym)
	}
	return filtered
}
