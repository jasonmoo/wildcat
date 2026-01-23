package search_cmd

import (
	"context"
	"fmt"
	"regexp"
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

func (c *SearchCommand) Cmd() *cobra.Command {
	var limit int
	var scope string
	var kind string

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search for symbols (fuzzy or regex)",
		Long: `Search for symbols using fuzzy matching or regex patterns.

Query Syntax:
  Client         Fuzzy match on symbol name
  lsp.Client     Package-qualified match (use . to search pkg.Symbol)
  DocSym         Abbreviation match - matches "DocumentSymbol"
  ^New.*Client$  Regex match (auto-detected from metacharacters)
  Get[A-Z]+      Regex match on symbol names

Mode Detection:
  Regex metacharacters (* + ? ^ $ [ ] { } ( ) | \) trigger regex mode.
  Otherwise, fuzzy matching is used. Regex is case-insensitive by default.

Sorting:
  Fuzzy: Ranked by match quality, length similarity, case-sensitive bonus.
  Regex: Sorted by symbol name length (shorter first), then alphabetically.

Scope (filters output, not search area):
  project       - All project packages (default)
  all           - Include dependencies and stdlib
  pkg1,pkg2     - Specific packages (comma-separated)
  -pkg          - Exclude package (prefix with -)

Pattern syntax:
  internal/lsp       - Exact package match
  internal/...       - Package and all subpackages (Go-style)
  internal/*         - Direct children only
  internal/**        - All descendants
  **/util            - Match anywhere in path

All symbols are indexed; scope controls which results appear in output.

Examples:
  wildcat search Client                            # fuzzy match "Client"
  wildcat search lsp.Client                        # fuzzy match in lsp package
  wildcat search "^New"                            # regex: names starting with New
  wildcat search --kind func Format                # functions only
  wildcat search --scope all Config                # include dependencies
  wildcat search --scope "project,-internal/..."   # exclude internal subtree
  wildcat search --limit 10 Config                 # top 10 matches`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var kinds []golang.SymbolKind
			if kind != "" {
				var err error
				kinds, err = golang.ParseKinds(kind)
				if err != nil {
					return err
				}
			}
			return commands.RunCommand(cmd, c,
				WithQuery(args[0]),
				WithLimit(limit),
				WithScope(scope),
				WithKinds(kinds),
			)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum results")
	cmd.Flags().StringVar(&scope, "scope", "project", "Filter output to packages (patterns: internal/..., **/util, -excluded)")
	cmd.Flags().StringVar(&kind, "kind", "", "Filter by kind: func, method, type, interface, const, var (comma-separated)")
	return cmd
}

func (c *SearchCommand) README() string {
	return "TODO"
}

func (c *SearchCommand) Execute(ctx context.Context, wc *commands.Wildcat, opts ...func(*SearchCommand) error) (commands.Result, error) {
	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, fmt.Errorf("interal_error: failed to apply opt: %w", err)
		}
	}

	if c.query == "" {
		return commands.NewErrorResultf("invalid_query", "empty search term"), nil
	}

	// Detect if query is a regex pattern
	var results []golang.SearchResult
	var searchMode string
	if golang.IsRegexPattern(c.query) {
		// Compile regex (case-insensitive by default)
		pattern, err := regexp.Compile("(?i)" + c.query)
		if err != nil {
			return commands.NewErrorResultf("invalid_regex", "invalid regex pattern: %s", err), nil
		}
		results = wc.Index.RegexSearch(pattern, &golang.SearchOptions{
			Limit: c.limit,
			Kinds: c.kinds,
		})
		searchMode = "regex"
	} else {
		// Fuzzy search
		results = wc.Index.Search(c.query, &golang.SearchOptions{
			Limit: c.limit,
			Kinds: c.kinds,
		})
		searchMode = "fuzzy"
	}

	// Apply scope filtering using ParseScope for consistent behavior
	scopeFilter, err := wc.ParseScope(ctx, c.scope, ".")
	if err != nil {
		return commands.NewErrorResultf("invalid_scope", "invalid scope: %s", err), nil
	}

	filtered := make([]golang.SearchResult, 0, len(results))
	for _, r := range results {
		if scopeFilter.InScope(r.Symbol.Package.Identifier.PkgPath) {
			filtered = append(filtered, r)
		}
	}
	results = filtered

	// Build flat results list (already sorted by score from Search)
	matches := make([]SearchMatch, 0, len(results))
	kindCounts := make(map[string]int)

	for _, r := range results {
		kindCounts[string(r.Symbol.Kind)]++
		matches = append(matches, SearchMatch{
			Symbol:     r.Symbol.Package.Identifier.Name + "." + r.Symbol.Name,
			Kind:       string(r.Symbol.Kind),
			Package:    r.Symbol.Package.Identifier.PkgPath,
			Signature:  r.Symbol.Signature(),
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

	// Build scope resolved info if patterns were used
	var scopeResolved *output.ScopeResolved
	if len(scopeFilter.ExcludePatterns()) > 0 || len(scopeFilter.IncludePatterns()) > 0 {
		scopeResolved = &output.ScopeResolved{
			Includes: scopeFilter.ResolvedIncludes(),
			Excludes: scopeFilter.ResolvedExcludes(),
		}
	}

	return &SearchCommandResponse{
		Query: output.SearchQuery{
			Command:       "search",
			Pattern:       c.query,
			Mode:          searchMode,
			Scope:         c.scope,
			ScopeResolved: scopeResolved,
			Kind:          ksb.String(),
		},
		Results: matches,
		Summary: output.SearchSummary{
			Count:     len(results),
			ByKind:    kindCounts,
			Truncated: false,
		},
	}, nil
}
