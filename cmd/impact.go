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
	Use:   "impact <symbol>",
	Short: "Analyze the impact of changing a symbol",
	Long: `Comprehensive impact analysis: everything affected by changing a symbol.

This command answers "What breaks if I change this?" by combining:
  - Transitive callers (recursive, not just direct)
  - All references (type usage, not just calls)
  - Interface implementations (if changing an interface)

Examples:
  wildcat impact config.Config
  wildcat impact Server.Start
  wildcat impact Handler`,
	Args: cobra.ExactArgs(1),
	RunE: runImpact,
}

var (
	impactExcludeTests bool
	impactDepth        int
)

func init() {
	rootCmd.AddCommand(impactCmd)

	impactCmd.Flags().BoolVar(&impactExcludeTests, "exclude-tests", false, "Exclude test files")
	impactCmd.Flags().IntVar(&impactDepth, "depth", 3, "Max depth for transitive callers")
}

func runImpact(cmd *cobra.Command, args []string) error {
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

	// Get transitive callers (for functions/methods)
	if resolved.Kind == lsp.SymbolKindFunction || resolved.Kind == lsp.SymbolKindMethod {
		items, err := client.PrepareCallHierarchy(ctx, resolved.URI, resolved.Position)
		if err == nil && len(items) > 0 {
			traverser := traverse.NewTraverser(client)
			opts := traverse.Options{
				Direction:    traverse.Up,
				MaxDepth:     impactDepth,
				ExcludeTests: impactExcludeTests,
			}

			callers, err := traverser.GetCallers(ctx, items[0], opts)
			if err == nil {
				for _, caller := range callers {
					impact.Callers = append(impact.Callers, output.ImpactCategory{
						Symbol: caller.Symbol,
						File:   output.AbsolutePath(caller.File),
						Line:   caller.Line,
						Reason: "calls this function",
					})
					if caller.InTest {
						inTestsCount++
					}
				}
				callersCount = len(callers)
			}
		}
	}

	// Get all references
	refs, err := client.References(ctx, resolved.URI, resolved.Position, false)
	if err == nil {
		for _, ref := range refs {
			file := lsp.URIToPath(ref.URI)
			isTest := output.IsTestFile(file)

			if impactExcludeTests && isTest {
				continue
			}

			impact.References = append(impact.References, output.ImpactCategory{
				File:   output.AbsolutePath(file),
				Line:   ref.Range.Start.Line + 1,
				Reason: "references this symbol",
			})
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

				impact.Implementations = append(impact.Implementations, output.ImpactCategory{
					File:   output.AbsolutePath(file),
					Line:   impl.Range.Start.Line + 1,
					Reason: "implements this interface",
				})
				if isTest {
					inTestsCount++
				}
			}
			implsCount = len(impact.Implementations)
		}
	}

	totalLocations := callersCount + refsCount + implsCount

	response := output.ImpactResponse{
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
	}

	return writer.Write(response)
}
