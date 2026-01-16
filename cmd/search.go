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
  wildcat search --limit 5 Config               # top 5 matches for Config`,
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
	searchCmd.Flags().StringVar(&searchScope, "scope", "", "Filter to specific packages (comma-separated)")
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

	// Build results with decorations
	results := make([]output.SearchResult, 0, len(symbols))
	kindCounts := make(map[string]int)

	for _, sym := range symbols {
		kind := sym.Kind.String()
		kindCounts[kind]++

		file := output.AbsolutePath(lsp.URIToPath(sym.Location.URI))
		startLine := sym.Location.Range.Start.Line + 1
		endLine := sym.Location.Range.End.Line + 1
		location := fmt.Sprintf("%s:%d:%d", file, startLine, endLine)

		result := output.SearchResult{
			Symbol:   sym.Name,
			Kind:     kind,
			Location: location,
			Package:  sym.ContainerName,
		}
		results = append(results, result)
	}

	response := output.SearchResponse{
		Query: output.SearchQuery{
			Command: "search",
			Pattern: query,
		},
		Results: results,
		Summary: output.SearchSummary{
			Count:     len(results),
			ByKind:    kindCounts,
			Truncated: truncated,
		},
	}

	return writer.Write(response)
}

// filterSymbolsByScope filters symbols to those in specified packages.
// Scope is a comma-separated list of package paths (resolved via golang.ResolvePackagePath).
func filterSymbolsByScope(symbols []lsp.SymbolInformation, scope, workDir string) []lsp.SymbolInformation {
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
