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

var impactCmd = &cobra.Command{
	Use:   "impact <symbol>...",
	Short: "Analyze the impact of changing a symbol",
	Long: `Comprehensive impact analysis: everything affected by changing a symbol.

This command answers "What breaks if I change this?" by combining:
  - Direct callers (functions that call this symbol)
  - All references (type usage, not just calls)
  - Interface implementations (if changing an interface)

Examples:
  wildcat impact config.Config
  wildcat impact Server.Start
  wildcat impact Handler
  wildcat impact FileURI URIToPath    # multiple symbols`,
	Args: cobra.MinimumNArgs(1),
	RunE: runImpact,
}

var (
	impactExcludeTests bool
)

func init() {
	rootCmd.AddCommand(impactCmd)

	impactCmd.Flags().BoolVar(&impactExcludeTests, "exclude-tests", false, "Exclude test files")
}

func runImpact(cmd *cobra.Command, args []string) error {
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
	var responses []output.ImpactResponse
	for _, symbolArg := range args {
		response, err := getImpactForSymbol(ctx, client, symbolArg)
		if err != nil {
			// For multi-symbol queries, include error as a response
			if len(args) > 1 {
				responses = append(responses, output.ImpactResponse{
					Query: output.QueryInfo{
						Command: "impact",
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

func getImpactForSymbol(ctx context.Context, client *lsp.Client, symbolArg string) (*output.ImpactResponse, error) {
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

	// Determine symbol kind for display
	kind := "symbol"
	switch resolved.Kind {
	case lsp.SymbolKindFunction:
		kind = "function"
	case lsp.SymbolKindMethod:
		kind = "method"
	case lsp.SymbolKindClass, lsp.SymbolKindStruct:
		kind = "type"
	case lsp.SymbolKindInterface:
		kind = "interface"
	case lsp.SymbolKindVariable:
		kind = "variable"
	case lsp.SymbolKindConstant:
		kind = "constant"
	}

	impact := output.Impact{}
	var callersCount, refsCount, implsCount, inTestsCount int
	extractor := output.NewSnippetExtractor()

	// Get transitive callers (for functions/methods)
	if resolved.Kind == lsp.SymbolKindFunction || resolved.Kind == lsp.SymbolKindMethod {
		items, err := client.PrepareCallHierarchy(ctx, resolved.URI, resolved.Position)
		if err == nil && len(items) > 0 {
			traverser := traverse.NewTraverser(client)
			opts := traverse.Options{
				Direction:    traverse.Up,
				MaxDepth:     1, // Direct callers only for impact analysis
				ExcludeTests: impactExcludeTests,
			}

			callers, err := traverser.GetCallers(ctx, items[0], opts)
			if err == nil {
				for _, caller := range callers {
					// Use call site line if available
					callLine := caller.Line
					if len(caller.CallRanges) > 0 {
						callLine = caller.CallRanges[0].Start.Line + 1
					}

					cat := output.ImpactCategory{
						Symbol: caller.Symbol,
						File:   output.AbsolutePath(caller.File),
						Line:   callLine,
						Reason: "calls this function",
					}
					if snippet, snippetStart, snippetEnd, err := extractor.ExtractSmart(caller.File, callLine); err == nil {
						cat.Snippet = snippet
						cat.SnippetStart = snippetStart
						cat.SnippetEnd = snippetEnd
					}
					impact.Callers = append(impact.Callers, cat)
					if caller.InTest {
						inTestsCount++
					}
				}
				callersCount = len(callers)
			}
		}
	}

	// Build set of caller locations to dedupe references
	callerLocations := make(map[string]bool)
	for _, caller := range impact.Callers {
		key := fmt.Sprintf("%s:%d", caller.File, caller.Line)
		callerLocations[key] = true
	}

	// Get all references (excluding those already in callers)
	refs, err := client.References(ctx, resolved.URI, resolved.Position, false)
	if err == nil {
		for _, ref := range refs {
			file := lsp.URIToPath(ref.URI)
			isTest := output.IsTestFile(file)

			if impactExcludeTests && isTest {
				continue
			}

			line := ref.Range.Start.Line + 1
			absFile := output.AbsolutePath(file)

			// Skip if already covered by callers
			key := fmt.Sprintf("%s:%d", absFile, line)
			if callerLocations[key] {
				continue
			}

			cat := output.ImpactCategory{
				File:   absFile,
				Line:   line,
				Reason: "references this symbol",
			}
			if snippet, snippetStart, snippetEnd, err := extractor.ExtractSmart(file, line); err == nil {
				cat.Snippet = snippet
				cat.SnippetStart = snippetStart
				cat.SnippetEnd = snippetEnd
			}
			impact.References = append(impact.References, cat)
			if isTest {
				inTestsCount++
			}
		}
		refsCount = len(impact.References)
	}

	// Get implementations (for interfaces)
	if resolved.Kind == lsp.SymbolKindInterface {
		impls, err := client.Implementation(ctx, resolved.URI, resolved.Position)
		if err == nil {
			for _, impl := range impls {
				file := lsp.URIToPath(impl.URI)
				isTest := output.IsTestFile(file)

				if impactExcludeTests && isTest {
					continue
				}

				line := impl.Range.Start.Line + 1
				cat := output.ImpactCategory{
					File:   output.AbsolutePath(file),
					Line:   line,
					Reason: "implements this interface",
				}
				if snippet, snippetStart, snippetEnd, err := extractor.ExtractSmart(file, line); err == nil {
					cat.Snippet = snippet
					cat.SnippetStart = snippetStart
					cat.SnippetEnd = snippetEnd
				}
				impact.Implementations = append(impact.Implementations, cat)
				if isTest {
					inTestsCount++
				}
			}
			implsCount = len(impact.Implementations)
		}
	}

	totalLocations := callersCount + refsCount + implsCount

	return &output.ImpactResponse{
		Query: output.QueryInfo{
			Command:  "impact",
			Target:   query.Raw,
			Resolved: resolved.Name,
		},
		Target: output.TargetInfo{
			Symbol: resolved.Name,
			Kind:   kind,
			File:   output.AbsolutePath(lsp.URIToPath(resolved.URI)),
			Line:   resolved.Position.Line + 1,
		},
		Impact: impact,
		Summary: output.ImpactSummary{
			TotalLocations:  totalLocations,
			Callers:         callersCount,
			References:      refsCount,
			Implementations: implsCount,
			InTests:         inTestsCount,
		},
		OtherFuzzyMatches: similarSymbols,
	}, nil
}
