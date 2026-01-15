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

var satisfiesCmd = &cobra.Command{
	Use:   "satisfies <type>...",
	Short: "Find all interfaces a type satisfies",
	Long: `Find all interfaces a type satisfies.

This uses LSP's type hierarchy to find interfaces that the specified
concrete type implements.

Examples:
  wildcat satisfies JSONFormatter
  wildcat satisfies output.Writer
  wildcat satisfies *Server
  wildcat satisfies Writer Registry    # multiple types`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSatisfies,
}

var (
	satisfiesExcludeStdlib bool
	satisfiesCompact       bool
	satisfiesContext       int
)

func init() {
	rootCmd.AddCommand(satisfiesCmd)

	satisfiesCmd.Flags().BoolVar(&satisfiesExcludeStdlib, "exclude-stdlib", false, "Exclude standard library interfaces")
	satisfiesCmd.Flags().BoolVar(&satisfiesCompact, "compact", false, "Omit snippets")
	satisfiesCmd.Flags().IntVar(&satisfiesContext, "context", 3, "Lines of context in snippet")
}

func runSatisfies(cmd *cobra.Command, args []string) error {
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
	var responses []output.SatisfiesResponse
	for _, symbolArg := range args {
		response, err := getSatisfiesForSymbol(ctx, client, symbolArg)
		if err != nil {
			// For multi-symbol queries, include error as a response
			if len(args) > 1 {
				responses = append(responses, output.SatisfiesResponse{
					Query: output.QueryInfo{
						Command: "satisfies",
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

func getSatisfiesForSymbol(ctx context.Context, client *lsp.Client, symbolArg string) (*output.SatisfiesResponse, error) {
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

	// Prepare type hierarchy
	items, err := client.PrepareTypeHierarchy(ctx, resolved.URI, resolved.Position)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare type hierarchy: %w", err)
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no type hierarchy found for '%s'", query.Raw)
	}

	// Get supertypes (interfaces this type satisfies)
	supertypes, err := client.Supertypes(ctx, items[0])
	if err != nil {
		return nil, fmt.Errorf("failed to get supertypes: %w", err)
	}

	// Build results
	extractor := output.NewSnippetExtractor()
	var results []output.InterfaceResult

	for _, st := range supertypes {
		file := lsp.URIToPath(st.URI)

		// Filter stdlib if requested
		if satisfiesExcludeStdlib && isStdlibPath(file) {
			continue
		}

		result := output.InterfaceResult{
			Symbol: st.Name,
			File:   output.AbsolutePath(file),
			Line:   st.Range.Start.Line + 1,
		}

		if !satisfiesCompact {
			line := st.Range.Start.Line + 1
			snippet, snippetStart, snippetEnd, err := extractor.ExtractSmart(file, line)
			if err == nil {
				result.Snippet = snippet
				result.SnippetStart = snippetStart
				result.SnippetEnd = snippetEnd
			}
		}

		results = append(results, result)
	}

	// Determine kind
	kind := resolved.Kind.String()

	return &output.SatisfiesResponse{
		Query: output.QueryInfo{
			Command:  "satisfies",
			Target:   query.Raw,
			Resolved: resolved.Name,
		},
		Type: output.TargetInfo{
			Symbol: resolved.Name,
			Kind:   kind,
			File:   output.AbsolutePath(lsp.URIToPath(resolved.URI)),
			Line:   resolved.Position.Line + 1,
		},
		Interfaces: results,
		Summary: output.Summary{
			Count: len(results),
		},
	}, nil
}

// isStdlibPath checks if a path is from the standard library.
func isStdlibPath(path string) bool {
	return golang.IsStdlibPath(path)
}
