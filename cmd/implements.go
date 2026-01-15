package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jasonmoo/wildcat/internal/errors"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/lsp"
	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/jasonmoo/wildcat/internal/symbols"
	"github.com/spf13/cobra"
)

var implementsCmd = &cobra.Command{
	Use:   "implements <interface>...",
	Short: "Find all types implementing an interface",
	Long: `Find all types implementing an interface.

This uses LSP's textDocument/implementation to find concrete types
that implement the specified interface.

Examples:
  wildcat implements io.Reader
  wildcat implements error
  wildcat implements Handler
  wildcat implements Formatter Writer    # multiple interfaces`,
	Args: cobra.MinimumNArgs(1),
	RunE: runImplements,
}

var (
	implementsExcludeTests bool
	implementsCompact      bool
	implementsContext      int
)

func init() {
	rootCmd.AddCommand(implementsCmd)

	implementsCmd.Flags().BoolVar(&implementsExcludeTests, "exclude-tests", false, "Exclude test files")
	implementsCmd.Flags().BoolVar(&implementsCompact, "compact", false, "Omit snippets")
	implementsCmd.Flags().IntVar(&implementsContext, "context", 3, "Lines of context in snippet")
}

func runImplements(cmd *cobra.Command, args []string) error {
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
	var responses []output.ImplementsResponse
	for _, symbolArg := range args {
		response, err := getImplementsForSymbol(ctx, client, symbolArg)
		if err != nil {
			// For multi-symbol queries, include error as a response
			if len(args) > 1 {
				responses = append(responses, output.ImplementsResponse{
					Query: output.QueryInfo{
						Command: "implements",
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

func getImplementsForSymbol(ctx context.Context, client *lsp.Client, symbolArg string) (*output.ImplementsResponse, error) {
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

	// Check if it's an interface
	if resolved.Kind != lsp.SymbolKindInterface {
		return nil, fmt.Errorf("'%s' is not an interface (got %s)", query.Raw, resolved.Kind.String())
	}

	// Prepare type hierarchy to get subtypes (implementing types)
	items, err := client.PrepareTypeHierarchy(ctx, resolved.URI, resolved.Position)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare type hierarchy: %w", err)
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no type hierarchy found for '%s'", query.Raw)
	}

	// Get subtypes (types that implement this interface)
	impls, err := client.Subtypes(ctx, items[0])
	if err != nil {
		return nil, fmt.Errorf("failed to get implementations: %w", err)
	}

	// Build results
	extractor := output.NewSnippetExtractor()
	var results []output.Result
	inTests := 0

	// Get direct deps for filtering indirect dependencies
	workDir, _ := os.Getwd()
	directDeps := golang.DirectDeps(workDir)

	for _, impl := range impls {
		file := lsp.URIToPath(impl.URI)
		isTest := output.IsTestFile(file)

		if implementsExcludeTests && isTest {
			continue
		}

		// Filter indirect dependencies (only show stdlib + direct deps + local)
		if !golang.IsDirectDep(file, directDeps) {
			continue
		}

		result := output.Result{
			Symbol: impl.Name,
			File:   output.AbsolutePath(file),
			Line:   impl.Range.Start.Line + 1,
			InTest: isTest,
		}

		if !implementsCompact {
			line := impl.Range.Start.Line + 1
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

		results = append(results, result)
	}

	// Merge overlapping snippets
	originalCount := len(results)
	if !implementsCompact {
		results = extractor.MergeOverlappingResults(results)
		inTests = 0
		for _, r := range results {
			if r.InTest {
				inTests++
			}
		}
	}

	return &output.ImplementsResponse{
		Query: output.QueryInfo{
			Command:  "implements",
			Target:   query.Raw,
			Resolved: resolved.Name,
		},
		Interface: output.TargetInfo{
			Symbol: resolved.Name,
			Kind:   "interface",
			File:   output.AbsolutePath(lsp.URIToPath(resolved.URI)),
			Line:   resolved.Position.Line + 1,
		},
		Implementations: results,
		Summary: output.Summary{
			Count:   originalCount,
			InTests: inTests,
		},
	}, nil
}

