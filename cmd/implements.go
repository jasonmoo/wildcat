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

var implementsCmd = &cobra.Command{
	Use:   "implements <interface>",
	Short: "Find all types implementing an interface",
	Long: `Find all types implementing an interface.

This uses LSP's textDocument/implementation to find concrete types
that implement the specified interface.

Examples:
  wildcat implements io.Reader
  wildcat implements error
  wildcat implements Handler`,
	Args: cobra.ExactArgs(1),
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
	symbolArg := args[0]
	writer, err := GetWriter(os.Stdout)
	if err != nil {
		return fmt.Errorf("invalid output format: %w", err)
	}

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

	// Resolve symbol
	resolver := symbols.NewResolver(client)
	resolved, err := resolver.Resolve(ctx, query)
	if err != nil {
		if we, ok := err.(*errors.WildcatError); ok {
			return writer.WriteError(string(we.Code), we.Message, we.Suggestions, we.Context)
		}
		return writer.WriteError(string(errors.CodeSymbolNotFound), err.Error(), nil, nil)
	}

	// Check if it's an interface
	if resolved.Kind != lsp.SymbolKindInterface {
		return writer.WriteError(
			"invalid_symbol_kind",
			fmt.Sprintf("'%s' is not an interface (got %s)", query.Raw, symbolKindName(resolved.Kind)),
			[]string{"Use an interface type name"},
			map[string]any{"kind": symbolKindName(resolved.Kind)},
		)
	}

	// Get implementations
	impls, err := client.Implementation(ctx, resolved.URI, resolved.Position)
	if err != nil {
		return writer.WriteError(
			string(errors.CodeLSPError),
			fmt.Sprintf("Failed to get implementations: %v", err),
			nil,
			nil,
		)
	}

	// Build results
	extractor := output.NewSnippetExtractor()
	var results []output.Result
	inTests := 0

	for _, impl := range impls {
		file := lsp.URIToPath(impl.URI)
		isTest := output.IsTestFile(file)

		if implementsExcludeTests && isTest {
			continue
		}

		result := output.Result{
			File:   output.AbsolutePath(file),
			Line:   impl.Range.Start.Line + 1,
			InTest: isTest,
		}

		if !implementsCompact {
			line := impl.Range.Start.Line + 1
			snippet, err := extractor.Extract(file, line, implementsContext)
			if err == nil {
				result.Snippet = snippet
			}
		}

		if isTest {
			inTests++
		}

		results = append(results, result)
	}

	response := output.ImplementsResponse{
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
			Count:   len(results),
			InTests: inTests,
		},
	}

	return writer.Write(response)
}

func symbolKindName(kind lsp.SymbolKind) string {
	switch kind {
	case lsp.SymbolKindFunction:
		return "function"
	case lsp.SymbolKindMethod:
		return "method"
	case lsp.SymbolKindClass, lsp.SymbolKindStruct:
		return "type"
	case lsp.SymbolKindInterface:
		return "interface"
	case lsp.SymbolKindVariable:
		return "variable"
	case lsp.SymbolKindConstant:
		return "constant"
	default:
		return "symbol"
	}
}
