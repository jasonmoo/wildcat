package search_cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/spf13/cobra"
)

type SearchCommand struct {
	query string
	limit int
	scope string
	kinds []golang.SymbolKind
}

var _ commands.Command[*SearchCommand] = (*SearchCommand)(nil)

func WithQuery(q string) func(*SearchCommand) error {
	return func(c *SearchCommand) error {
		c.query = q
		return nil
	}
}

func WithLimit(limit int) func(*SearchCommand) error {
	return func(c *SearchCommand) error {
		c.limit = limit
		return nil
	}
}

func WithScope(scope string) func(*SearchCommand) error {
	return func(c *SearchCommand) error {
		c.scope = scope
		return nil
	}
}

func WithKinds(kinds []golang.SymbolKind) func(*SearchCommand) error {
	return func(c *SearchCommand) error {
		c.kinds = kinds
		return nil
	}
}

func NewSearchCommand() *SearchCommand {
	return &SearchCommand{
		limit: 20,
		scope: "project",
	}
}

func (c *SearchCommand) Execute(ctx context.Context, wc *commands.Wildcat, opts ...func(*SearchCommand) error) (commands.Result, *commands.Error) {
	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, commands.NewErrorf("opts_error", "failed to apply opt: %w", err)
		}
	}

	if c.query == "" {
		return nil, commands.NewErrorf("invalid_query", "query is required")
	}

	// Search with options
	results := wc.Index.Search(c.query, &golang.SearchOptions{
		Limit: c.limit,
		Kinds: c.kinds,
	})

	// Apply scope filtering
	if c.scope != "all" {
		modulePath := ""
		if wc.Project.Module != nil {
			modulePath = wc.Project.Module.Path
		}
		results = filterByScope(results, c.scope, modulePath)
	}

	// Build flat results list (already sorted by score from Search)
	matches := make([]SearchMatch, 0, len(results))
	kindCounts := make(map[string]int)

	for _, r := range results {
		kindCounts[string(r.Symbol.Kind)]++

		sig, err := r.Symbol.Signature()
		if err != nil {
			sig = r.Symbol.Name
		}

		matches = append(matches, SearchMatch{
			Symbol:     r.Symbol.Name,
			Kind:       string(r.Symbol.Kind),
			Package:    r.Symbol.PkgPath,
			Signature:  sig,
			Definition: fmt.Sprintf("%s:%s", r.Symbol.Filename(), r.Symbol.Location()),
		})
	}

	// Build kind filter string for query display
	var ksb strings.Builder
	for i, k := range c.kinds {
		if i > 0 {
			ksb.WriteByte(',')
		}
		ksb.WriteString(string(k))
	}

	return &SearchCommandResponse{
		Query: output.SearchQuery{
			Command: "search",
			Pattern: c.query,
			Scope:   c.scope,
			Kind:    ksb.String(),
		},
		Results: matches,
		Summary: output.SearchSummary{
			Count:     len(results),
			ByKind:    kindCounts,
			Truncated: false,
		},
	}, nil
}

func filterByScope(results []golang.SearchResult, scope, modulePath string) []golang.SearchResult {
	if scope == "project" {
		// Filter to project packages only
		filtered := make([]golang.SearchResult, 0, len(results))
		for _, r := range results {
			if strings.HasPrefix(r.Symbol.PkgPath, modulePath) {
				filtered = append(filtered, r)
			}
		}
		return filtered
	}

	// Parse scope for includes/excludes
	var includes, excludes []string
	for _, part := range strings.Split(scope, ",") {
		part = strings.TrimSpace(part)
		if part == "" || part == "project" {
			continue
		}
		if strings.HasPrefix(part, "-") {
			excludes = append(excludes, strings.TrimPrefix(part, "-"))
		} else {
			includes = append(includes, part)
		}
	}

	filtered := make([]golang.SearchResult, 0, len(results))
	for _, r := range results {
		pkgPath := r.Symbol.PkgPath

		// Check excludes
		excluded := false
		for _, ex := range excludes {
			if strings.Contains(pkgPath, ex) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		// Check includes (if specified)
		if len(includes) > 0 {
			included := false
			for _, inc := range includes {
				if strings.Contains(pkgPath, inc) {
					included = true
					break
				}
			}
			if !included {
				continue
			}
		}

		filtered = append(filtered, r)
	}
	return filtered
}

func (c *SearchCommand) Cmd() *cobra.Command {
	var limit int
	var scope string
	var kind string

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Fuzzy search for symbols",
		Long: `Search for symbols using fuzzy matching.

Query Syntax:
  Client         Fuzzy match on symbol name
  lsp.Client     Package-qualified match (use . to search pkg.Symbol)
  DocSym         Abbreviation match - matches "DocumentSymbol"

Scoring:
  Results are ranked by match quality. Exact matches and shorter symbols
  score higher. Case-sensitive matches get a bonus.

Examples:
  wildcat search Client                       # fuzzy match "Client"
  wildcat search lsp.Client                   # match in lsp package
  wildcat search --kind func Format           # functions only
  wildcat search --kind type,interface Node   # types and interfaces
  wildcat search --scope all Config           # include dependencies
  wildcat search --scope lsp Client           # only lsp package
  wildcat search --scope commands,-test Cmd   # commands, exclude test
  wildcat search --limit 10 Config            # top 10 matches`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wc, err := commands.LoadWildcat(cmd.Context(), ".")
			if err != nil {
				return err
			}

			// Parse kinds
			var kinds []golang.SymbolKind
			if kind != "" {
				kinds = golang.ParseKinds(kind)
			}

			result, cmdErr := c.Execute(cmd.Context(), wc,
				WithQuery(args[0]),
				WithLimit(limit),
				WithScope(scope),
				WithKinds(kinds),
			)
			if cmdErr != nil {
				return fmt.Errorf("%s: %w", cmdErr.Code, cmdErr.Error)
			}

			// Check if JSON output requested
			if outputFlag := cmd.Flag("output"); outputFlag != nil && outputFlag.Changed && outputFlag.Value.String() == "json" {
				data, err := result.MarshalJSON()
				if err != nil {
					return err
				}
				os.Stdout.Write(data)
				os.Stdout.WriteString("\n")
				return nil
			}

			// Default to markdown
			md, err := result.MarshalMarkdown()
			if err != nil {
				return err
			}
			os.Stdout.Write(md)
			os.Stdout.WriteString("\n")
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum results")
	cmd.Flags().StringVar(&scope, "scope", "project", "Scope: 'project', 'all', or package substrings (lsp, commands,-test)")
	cmd.Flags().StringVar(&kind, "kind", "", "Filter by kind: func, method, type, interface, const, var (comma-separated)")
	return cmd
}

func (c *SearchCommand) README() string {
	return "TODO"
}
