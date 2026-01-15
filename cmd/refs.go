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
	"github.com/spf13/cobra"
)

var refsCmd = &cobra.Command{
	Use:   "refs <symbol>...",
	Short: "Find all references to a symbol",
	Long: `Find all references to a symbol (not just calls).

This includes:
  - Function calls
  - Variable references
  - Type references
  - Constant usage
  - Functions passed as values

Examples:
  wildcat refs config.Load
  wildcat refs Config
  wildcat refs MaxRetries
  wildcat refs FileURI URIToPath    # multiple symbols`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRefs,
}

var (
	refsExcludeTests        bool
	refsPackage             string
	refsLimit               int
	refsContext             int
	refsCompact             bool
	refsIncludeDeclaration  bool
)

func init() {
	rootCmd.AddCommand(refsCmd)

	refsCmd.Flags().BoolVar(&refsExcludeTests, "exclude-tests", false, "Exclude test files")
	refsCmd.Flags().StringVar(&refsPackage, "package", "", "Limit to package pattern")
	refsCmd.Flags().IntVar(&refsLimit, "limit", 0, "Maximum results (0 = unlimited)")
	refsCmd.Flags().IntVar(&refsContext, "context", 3, "Lines of context in snippet")
	refsCmd.Flags().BoolVar(&refsCompact, "compact", false, "Omit snippets")
	refsCmd.Flags().BoolVar(&refsIncludeDeclaration, "include-declaration", true, "Include the declaration in results")
}

func runRefs(cmd *cobra.Command, args []string) error {
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
	var responses []output.RefsResponse
	for _, symbolArg := range args {
		response, err := getRefsForSymbol(ctx, client, symbolArg)
		if err != nil {
			// For multi-symbol queries, include error as a response
			if len(args) > 1 {
				responses = append(responses, output.RefsResponse{
					Query: output.QueryInfo{
						Command: "refs",
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

func getRefsForSymbol(ctx context.Context, client *lsp.Client, symbolArg string) (*output.RefsResponse, error) {
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

	// Get references
	refs, err := client.References(ctx, resolved.URI, resolved.Position, refsIncludeDeclaration)
	if err != nil {
		return nil, fmt.Errorf("failed to get references: %w", err)
	}

	// Build results
	extractor := output.NewSnippetExtractor()
	var results []output.Result
	packagesSet := make(map[string]bool)
	inTests := 0

	for _, ref := range refs {
		file := lsp.URIToPath(ref.URI)
		isTest := output.IsTestFile(file)

		// Apply filters
		if refsExcludeTests && isTest {
			continue
		}
		if refsLimit > 0 && len(results) >= refsLimit {
			break
		}

		result := output.Result{
			File:   output.AbsolutePath(file),
			Line:   ref.Range.Start.Line + 1,
			InTest: isTest,
		}

		if !refsCompact {
			line := ref.Range.Start.Line + 1
			snippet, snippetStart, snippetEnd, err := extractor.ExtractSmart(file, line)
			if err == nil {
				result.Snippet = snippet
				result.SnippetStart = snippetStart
				result.SnippetEnd = snippetEnd
			}
		}

		if isTest {
			inTests++
		}

		packagesSet[file] = true
		results = append(results, result)
	}

	packages := make([]string, 0, len(packagesSet))
	for p := range packagesSet {
		packages = append(packages, p)
	}

	// Merge overlapping snippets
	originalCount := len(results)
	if !refsCompact {
		results = extractor.MergeOverlappingResults(results)
		// Recalculate inTests after merge
		inTests = 0
		for _, r := range results {
			if r.InTest {
				inTests++
			}
		}
	}

	return &output.RefsResponse{
		Query: output.QueryInfo{
			Command:  "refs",
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
			Count:     originalCount,
			Packages:  packages,
			InTests:   inTests,
			Truncated: refsLimit > 0 && len(refs) > refsLimit,
		},
		OtherFuzzyMatches: similarSymbols,
	}, nil
}
