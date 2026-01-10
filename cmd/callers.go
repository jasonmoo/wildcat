package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jasonmoo/wildcat/internal/errors"
	"github.com/jasonmoo/wildcat/internal/lsp"
	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/jasonmoo/wildcat/internal/symbols"
	"github.com/jasonmoo/wildcat/internal/traverse"
	"github.com/spf13/cobra"
)

var callersCmd = &cobra.Command{
	Use:   "callers <symbol>",
	Short: "Find all callers of a function or method",
	Long: `Find all callers of a function or method.

Symbol formats:
  Function              Resolve in context
  pkg.Function          Package and function
  Type.Method           Method on type
  (*Type).Method        Method on pointer receiver
  path/to/pkg.Function  Full package path

Examples:
  wildcat callers config.Load
  wildcat callers Server.Start
  wildcat callers (*Handler).ServeHTTP`,
	Args: cobra.ExactArgs(1),
	RunE: runCallers,
}

var (
	callersExcludeTests bool
	callersPackage      string
	callersLimit        int
	callersContext      int
	callersCompact      bool
	callersDepth        int
)

func init() {
	rootCmd.AddCommand(callersCmd)

	callersCmd.Flags().BoolVar(&callersExcludeTests, "exclude-tests", false, "Exclude test files")
	callersCmd.Flags().StringVar(&callersPackage, "package", "", "Limit to package pattern")
	callersCmd.Flags().IntVar(&callersLimit, "limit", 0, "Maximum results (0 = unlimited)")
	callersCmd.Flags().IntVar(&callersContext, "context", 3, "Lines of context in snippet")
	callersCmd.Flags().BoolVar(&callersCompact, "compact", false, "Omit snippets")
	callersCmd.Flags().IntVar(&callersDepth, "depth", 1, "Depth of caller traversal (1 = direct only)")
}

func runCallers(cmd *cobra.Command, args []string) error {
	symbolArg := args[0]
	writer := output.NewWriter(os.Stdout, true)

	// Parse symbol
	query, err := symbols.Parse(symbolArg)
	if err != nil {
		return writer.WriteError("parse_error", err.Error(), nil, nil)
	}

	// Get working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Start LSP client
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Get language server configuration
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

	// Initialize
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

	// Resolve symbol
	resolver := symbols.NewResolver(client)
	resolved, err := resolver.Resolve(ctx, query)
	if err != nil {
		if we, ok := err.(*errors.WildcatError); ok {
			return writer.WriteError(string(we.Code), we.Message, we.Suggestions, we.Context)
		}
		return writer.WriteError(string(errors.CodeSymbolNotFound), err.Error(), nil, nil)
	}

	// Prepare call hierarchy
	items, err := client.PrepareCallHierarchy(ctx, resolved.URI, resolved.Position)
	if err != nil {
		return writer.WriteError(
			string(errors.CodeLSPError),
			fmt.Sprintf("Failed to prepare call hierarchy: %v", err),
			nil,
			nil,
		)
	}

	if len(items) == 0 {
		return writer.WriteError(
			string(errors.CodeSymbolNotFound),
			fmt.Sprintf("No call hierarchy found for '%s'", query.Raw),
			nil,
			nil,
		)
	}

	// Get callers
	traverser := traverse.NewTraverser(client)
	opts := traverse.Options{
		Direction:    traverse.Up,
		MaxDepth:     callersDepth,
		ExcludeTests: callersExcludeTests,
	}

	callers, err := traverser.GetCallers(ctx, items[0], opts)
	if err != nil {
		return writer.WriteError(
			string(errors.CodeLSPError),
			fmt.Sprintf("Failed to get callers: %v", err),
			nil,
			nil,
		)
	}

	// Build results
	extractor := output.NewSnippetExtractor()
	var results []output.Result
	packagesSet := make(map[string]bool)
	inTests := 0

	for _, caller := range callers {
		// Apply limit
		if callersLimit > 0 && len(results) >= callersLimit {
			break
		}

		// Apply package filter
		if callersPackage != "" {
			// TODO: implement package filtering
		}

		result := output.Result{
			Symbol: caller.Symbol,
			File:   output.AbsolutePath(caller.File),
			Line:   caller.Line,
			InTest: caller.InTest,
		}

		// Extract snippet if not compact
		if !callersCompact && len(caller.CallRanges) > 0 {
			line := caller.CallRanges[0].Start.Line + 1
			snippet, err := extractor.Extract(caller.File, line, callersContext)
			if err == nil {
				result.Snippet = snippet
			}

			// Extract call expression
			callExpr, err := extractor.ExtractCallExpr(
				caller.File,
				line,
				caller.CallRanges[0].Start.Character,
				caller.CallRanges[0].End.Character,
			)
			if err == nil {
				result.CallExpr = callExpr
			}
		}

		if caller.InTest {
			inTests++
		}

		// Track packages (extract from file path)
		// This is a simplified version - could be improved
		packagesSet[caller.File] = true

		results = append(results, result)
	}

	// Build package list
	packages := make([]string, 0, len(packagesSet))
	for p := range packagesSet {
		packages = append(packages, p)
	}

	// Build response
	response := output.CallersResponse{
		Query: output.QueryInfo{
			Command:  "callers",
			Target:   query.Raw,
			Resolved: resolved.Name,
		},
		Target: output.TargetInfo{
			Symbol: resolved.Name,
			File:   output.AbsolutePath(lsp.URIToPath(resolved.URI)),
			Line:   resolved.Position.Line + 1,
		},
		Results: results,
		Summary: output.Summary{
			Count:     len(results),
			Packages:  packages,
			InTests:   inTests,
			Truncated: callersLimit > 0 && len(callers) > callersLimit,
		},
	}

	return writer.Write(response)
}
