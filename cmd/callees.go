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

var calleesCmd = &cobra.Command{
	Use:   "callees <symbol>",
	Short: "Find all functions called by a function or method",
	Long: `Find all functions called by a function or method.

Symbol formats:
  Function              Resolve in context
  pkg.Function          Package and function
  Type.Method           Method on type
  (*Type).Method        Method on pointer receiver
  path/to/pkg.Function  Full package path

Examples:
  wildcat callees main.main
  wildcat callees Server.Start`,
	Args: cobra.ExactArgs(1),
	RunE: runCallees,
}

var (
	calleesExcludeTests  bool
	calleesExcludeStdlib bool
	calleesLimit         int
	calleesContext       int
	calleesCompact       bool
	calleesDepth         int
)

func init() {
	rootCmd.AddCommand(calleesCmd)

	calleesCmd.Flags().BoolVar(&calleesExcludeTests, "exclude-tests", false, "Exclude test files")
	calleesCmd.Flags().BoolVar(&calleesExcludeStdlib, "exclude-stdlib", false, "Exclude standard library")
	calleesCmd.Flags().IntVar(&calleesLimit, "limit", 0, "Maximum results (0 = unlimited)")
	calleesCmd.Flags().IntVar(&calleesContext, "context", 3, "Lines of context in snippet")
	calleesCmd.Flags().BoolVar(&calleesCompact, "compact", false, "Omit snippets")
	calleesCmd.Flags().IntVar(&calleesDepth, "depth", 1, "Depth of callee traversal (1 = direct only)")
}

func runCallees(cmd *cobra.Command, args []string) error {
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

	config := lsp.ServerConfig{
		Command: "gopls",
		Args:    []string{"serve"},
		WorkDir: workDir,
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

	// Get callees
	traverser := traverse.NewTraverser(client)
	opts := traverse.Options{
		Direction:     traverse.Down,
		MaxDepth:      calleesDepth,
		ExcludeTests:  calleesExcludeTests,
		ExcludeStdlib: calleesExcludeStdlib,
	}

	callees, err := traverser.GetCallees(ctx, items[0], opts)
	if err != nil {
		return writer.WriteError(
			string(errors.CodeLSPError),
			fmt.Sprintf("Failed to get callees: %v", err),
			nil,
			nil,
		)
	}

	// Build results
	extractor := output.NewSnippetExtractor()
	var results []output.Result
	packagesSet := make(map[string]bool)
	inTests := 0

	for _, callee := range callees {
		if calleesLimit > 0 && len(results) >= calleesLimit {
			break
		}

		result := output.Result{
			Symbol: callee.Symbol,
			File:   output.AbsolutePath(callee.File),
			Line:   callee.Line,
			InTest: callee.InTest,
		}

		if !calleesCompact && len(callee.CallRanges) > 0 {
			line := callee.CallRanges[0].Start.Line + 1
			snippet, err := extractor.Extract(callee.File, line, calleesContext)
			if err == nil {
				result.Snippet = snippet
			}
		}

		if callee.InTest {
			inTests++
		}

		packagesSet[callee.File] = true
		results = append(results, result)
	}

	packages := make([]string, 0, len(packagesSet))
	for p := range packagesSet {
		packages = append(packages, p)
	}

	response := output.CalleesResponse{
		Query: output.QueryInfo{
			Command:  "callees",
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
			Truncated: calleesLimit > 0 && len(callees) > calleesLimit,
		},
	}

	return writer.Write(response)
}
