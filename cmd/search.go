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
  wildcat search Resolve                        # functions/types matching Resolve
  wildcat search NewClient                      # exact or fuzzy matches
  wildcat search --limit 5 Config               # top 5 matches for Config
  wildcat search --scope project Config         # only project packages
  wildcat search --scope internal/lsp Client    # specific package`,
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
	searchCmd.Flags().StringVar(&searchScope, "scope", "", "Scope: 'project', or comma-separated packages (default: all)")
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

	// Apply scope filter if specified
	if searchScope != "" {
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

	return writer.Write(response)
}

// filterSymbolsByScope filters symbols to those in specified packages.
// Scope can be "project" (all project packages) or comma-separated package paths.
func filterSymbolsByScope(symbols []lsp.SymbolInformation, scope, workDir string) []lsp.SymbolInformation {
	// Handle "project" scope - filter to project packages by prefix
	if scope == "project" {
		projectRoot, err := golang.ResolvePackagePath(".", workDir)
		if err != nil {
			return symbols // Can't determine project root, return all
		}
		filtered := make([]lsp.SymbolInformation, 0, len(symbols))
		for _, sym := range symbols {
			if strings.HasPrefix(sym.ContainerName, projectRoot) {
				filtered = append(filtered, sym)
			}
		}
		return filtered
	}

	// Comma-separated list of packages
	packages := make(map[string]bool)
	for _, pkg := range strings.Split(scope, ",") {
		pkg = strings.TrimSpace(pkg)
		if pkg == "" {
			continue
		}
		// Resolve to full import path
		if resolved, err := golang.ResolvePackagePath(pkg, workDir); err == nil {
			packages[resolved] = true
		}
		// If resolution fails, skip silently
	}

	filtered := make([]lsp.SymbolInformation, 0, len(symbols))
	for _, sym := range symbols {
		if packages[sym.ContainerName] {
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
