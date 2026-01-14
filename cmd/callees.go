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
	Use:   "callees <symbol>...",
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
  wildcat callees Server.Start
  wildcat callees FileURI URIToPath    # multiple symbols`,
	Args: cobra.MinimumNArgs(1),
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

	time.Sleep(200 * time.Millisecond)

	// Process each symbol
	var responses []output.CalleesResponse
	for _, symbolArg := range args {
		response, err := getCalleesForSymbol(ctx, client, symbolArg)
		if err != nil {
			// For multi-symbol queries, include error as a response
			if len(args) > 1 {
				responses = append(responses, output.CalleesResponse{
					Query: output.QueryInfo{
						Command: "callees",
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

func getCalleesForSymbol(ctx context.Context, client *lsp.Client, symbolArg string) (*output.CalleesResponse, error) {
	// Parse symbol
	query, err := symbols.Parse(symbolArg)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	// Resolve symbol
	resolver := symbols.NewResolver(client)
	resolved, err := resolver.Resolve(ctx, query)
	if err != nil {
		return nil, err
	}

	// Find similar symbols for navigation aid
	similarSymbols := resolver.FindSimilar(ctx, query, 5)

	// Prepare call hierarchy
	items, err := client.PrepareCallHierarchy(ctx, resolved.URI, resolved.Position)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare call hierarchy: %w", err)
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no call hierarchy found for '%s'", query.Raw)
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
		return nil, fmt.Errorf("failed to get callees: %w", err)
	}

	// Build results
	extractor := output.NewSnippetExtractor()
	var results []output.Result
	packagesSet := make(map[string]bool)
	inTests := 0

	// CallRanges are in the target (caller) file, not the callee file
	targetFile := lsp.URIToPath(items[0].URI)

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
			// Extract snippet from the caller's file where the call happens
			line := callee.CallRanges[0].Start.Line + 1
			snippet, err := extractor.Extract(targetFile, line, calleesContext)
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

	return &output.CalleesResponse{
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
		OtherFuzzyMatches: similarSymbols,
	}, nil
}
