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
	"github.com/jasonmoo/wildcat/internal/traverse"
	"github.com/spf13/cobra"
)

var symbolCmd = &cobra.Command{
	Use:   "symbol <symbol>...",
	Short: "Complete symbol analysis: definition, callers, refs, interfaces",
	Long: `Full profile of a symbol: everything you need to understand and modify it.

Returns:
  - Definition location and signature
  - Direct callers (who calls this)
  - All references (type usage, not just calls)
  - Interface relationships (satisfies/implements)

Examples:
  wildcat symbol config.Config
  wildcat symbol Server.Start
  wildcat symbol Handler
  wildcat symbol FileURI URIToPath    # multiple symbols`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSymbol,
}

var (
	symbolExcludeTests bool
)

func init() {
	rootCmd.AddCommand(symbolCmd)

	symbolCmd.Flags().BoolVar(&symbolExcludeTests, "exclude-tests", false, "Exclude test files")
}

func runSymbol(cmd *cobra.Command, args []string) error {
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
	var responses []output.SymbolResponse
	for _, symbolArg := range args {
		response, err := getImpactForSymbol(ctx, client, symbolArg)
		if err != nil {
			// For multi-symbol queries, include error as a response
			if len(args) > 1 {
				responses = append(responses, output.SymbolResponse{
					Query: output.QueryInfo{
						Command: "symbol",
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

func getImpactForSymbol(ctx context.Context, client *lsp.Client, symbolArg string) (*output.SymbolResponse, error) {
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

	usage := output.SymbolUsage{}
	var callersCount, refsCount, implsCount, inTestsCount int
	extractor := output.NewSnippetExtractor()

	// Get transitive callers (for functions/methods)
	if resolved.Kind == lsp.SymbolKindFunction || resolved.Kind == lsp.SymbolKindMethod {
		items, err := client.PrepareCallHierarchy(ctx, resolved.URI, resolved.Position)
		if err == nil && len(items) > 0 {
			traverser := traverse.NewTraverser(client)
			opts := traverse.Options{
				Direction:    traverse.Up,
				MaxDepth:     1, // Direct callers only
				ExcludeTests: symbolExcludeTests,
			}

			callers, err := traverser.GetCallers(ctx, items[0], opts)
			if err == nil {
				for _, caller := range callers {
					// Use call site line if available
					callLine := caller.Line
					if len(caller.CallRanges) > 0 {
						callLine = caller.CallRanges[0].Start.Line + 1
					}

					cat := output.SymbolLocation{
						Symbol: caller.Symbol,
						File:   output.AbsolutePath(caller.File),
						Line:   callLine,
											}
					if snippet, snippetStart, snippetEnd, err := extractor.ExtractSmart(caller.File, callLine); err == nil {
						cat.Snippet = snippet
						cat.SnippetStart = snippetStart
						cat.SnippetEnd = snippetEnd
					}
					usage.Callers = append(usage.Callers, cat)
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
	for _, caller := range usage.Callers {
		key := fmt.Sprintf("%s:%d", caller.File, caller.Line)
		callerLocations[key] = true
	}

	// Get all references (excluding those already in callers)
	refs, err := client.References(ctx, resolved.URI, resolved.Position, false)
	if err == nil {
		for _, ref := range refs {
			file := lsp.URIToPath(ref.URI)
			isTest := output.IsTestFile(file)

			if symbolExcludeTests && isTest {
				continue
			}

			line := ref.Range.Start.Line + 1
			absFile := output.AbsolutePath(file)

			// Skip if already covered by callers
			key := fmt.Sprintf("%s:%d", absFile, line)
			if callerLocations[key] {
				continue
			}

			cat := output.SymbolLocation{
				File:   absFile,
				Line:   line,
							}
			if snippet, snippetStart, snippetEnd, err := extractor.ExtractSmart(file, line); err == nil {
				cat.Snippet = snippet
				cat.SnippetStart = snippetStart
				cat.SnippetEnd = snippetEnd
			}
			usage.References = append(usage.References, cat)
			if isTest {
				inTestsCount++
			}
		}
		refsCount = len(usage.References)
	}

	// Get implementations (for interfaces)
	if resolved.Kind == lsp.SymbolKindInterface {
		impls, err := client.Implementation(ctx, resolved.URI, resolved.Position)
		if err == nil {
			for _, impl := range impls {
				file := lsp.URIToPath(impl.URI)
				isTest := output.IsTestFile(file)

				if symbolExcludeTests && isTest {
					continue
				}

				line := impl.Range.Start.Line + 1
				cat := output.SymbolLocation{
					File:   output.AbsolutePath(file),
					Line:   line,
									}
				if snippet, snippetStart, snippetEnd, err := extractor.ExtractSmart(file, line); err == nil {
					cat.Snippet = snippet
					cat.SnippetStart = snippetStart
					cat.SnippetEnd = snippetEnd
				}
				usage.Implementations = append(usage.Implementations, cat)
				if isTest {
					inTestsCount++
				}
			}
			implsCount = len(usage.Implementations)
		}
	}

	// Get satisfies (for types - what interfaces they implement)
	var satisfiesCount int
	if resolved.Kind == lsp.SymbolKindStruct || resolved.Kind == lsp.SymbolKindClass {
		items, err := client.PrepareTypeHierarchy(ctx, resolved.URI, resolved.Position)
		if err == nil && len(items) > 0 {
			supertypes, err := client.Supertypes(ctx, items[0])
			if err == nil {
				workDir, _ := os.Getwd()
				directDeps := golang.DirectDeps(workDir)

				for _, st := range supertypes {
					file := lsp.URIToPath(st.URI)

					// Filter indirect dependencies
					if !golang.IsDirectDep(file, directDeps) {
						continue
					}

					line := st.Range.Start.Line + 1
					cat := output.SymbolLocation{
						Symbol: st.Name,
						File:   output.AbsolutePath(file),
						Line:   line,
											}
					if snippet, snippetStart, snippetEnd, err := extractor.ExtractSmart(file, line); err == nil {
						cat.Snippet = snippet
						cat.SnippetStart = snippetStart
						cat.SnippetEnd = snippetEnd
					}
					usage.Satisfies = append(usage.Satisfies, cat)
				}
				satisfiesCount = len(usage.Satisfies)
			}
		}
	}

	totalLocations := callersCount + refsCount + implsCount + satisfiesCount

	return &output.SymbolResponse{
		Query: output.QueryInfo{
			Command:  "symbol",
			Target:   query.Raw,
			Resolved: resolved.Name,
		},
		Target: output.TargetInfo{
			Symbol: resolved.Name,
			Kind:   kind,
			File:   output.AbsolutePath(lsp.URIToPath(resolved.URI)),
			Line:   resolved.Position.Line + 1,
		},
		Usage: usage,
		Summary: output.SymbolSummary{
			TotalLocations:  totalLocations,
			Callers:         callersCount,
			References:      refsCount,
			Implementations: implsCount,
			Satisfies:       satisfiesCount,
			InTests:         inTestsCount,
		},
		OtherFuzzyMatches: similarSymbols,
	}, nil
}
