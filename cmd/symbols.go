package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jasonmoo/wildcat/internal/errors"
	"github.com/jasonmoo/wildcat/internal/lsp"
	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/spf13/cobra"
)

var symbolsCmd = &cobra.Command{
	Use:   "symbols <query>",
	Short: "Search for symbols using fuzzy matching",
	Long: `Search for symbols across the workspace using gopls fuzzy matching.

This is a semantic alternative to grep - returns actual symbols (functions,
types, methods, constants) rather than text matches. Results are ranked by
relevance with workspace symbols prioritized over stdlib.

Query Syntax:
  hello          Fuzzy match - matches "Hello", "helloWorld", "SayHello"
  Hello          Case-sensitive fuzzy (uppercase triggers case-sensitivity)
  DocSym         Abbreviation match - matches "DocumentSymbol"
  cfg            Abbreviation match - matches "Config"

Examples:
  wildcat symbols Resolve           # functions/types matching Resolve
  wildcat symbols NewClient         # exact or fuzzy matches
  wildcat symbols --limit 5 Config  # top 5 matches for Config`,
	Args: cobra.ExactArgs(1),
	RunE: runSymbols,
}

var (
	symbolsLimit int
)

func init() {
	rootCmd.AddCommand(symbolsCmd)

	symbolsCmd.Flags().IntVar(&symbolsLimit, "limit", 20, "Maximum results (max 100)")
}

func runSymbols(cmd *cobra.Command, args []string) error {
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

	// Give LSP server time to index
	time.Sleep(200 * time.Millisecond)

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

	// Apply limit
	limit := symbolsLimit
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
	results := make([]output.SymbolResult, 0, len(symbols))
	kindCounts := make(map[string]int)

	for _, sym := range symbols {
		kind := sym.Kind.String()
		kindCounts[kind]++

		file := output.AbsolutePath(lsp.URIToPath(sym.Location.URI))
		startLine := sym.Location.Range.Start.Line + 1
		endLine := sym.Location.Range.End.Line + 1
		location := fmt.Sprintf("%s:%d:%d", file, startLine, endLine)

		result := output.SymbolResult{
			Symbol:   sym.Name,
			Kind:     kind,
			Location: location,
			Package:  sym.ContainerName,
		}
		results = append(results, result)
	}

	response := output.SymbolsResponse{
		Query: output.SymbolsQuery{
			Command: "symbols",
			Pattern: query,
		},
		Results: results,
		Summary: output.SymbolsSummary{
			Count:     len(results),
			ByKind:    kindCounts,
			Truncated: truncated,
		},
	}

	return writer.Write(response)
}
