package symbols

import (
	"context"
	"strings"

	"github.com/jasonmoo/wildcat/internal/errors"
	"github.com/jasonmoo/wildcat/internal/lsp"
)

// ResolvedSymbol represents a successfully resolved symbol.
type ResolvedSymbol struct {
	Name     string       // Full symbol name
	Kind     lsp.SymbolKind
	URI      string       // File URI
	Position lsp.Position // Position in file
	Range    lsp.Range    // Full range of symbol
}

// Resolver resolves symbol queries using an LSP client.
type Resolver struct {
	client *lsp.Client
}

// NewResolver creates a new symbol resolver.
func NewResolver(client *lsp.Client) *Resolver {
	return &Resolver{client: client}
}

// Resolve finds a symbol matching the query.
// Returns an error with suggestions if not found or ambiguous.
func (r *Resolver) Resolve(ctx context.Context, query *Query) (*ResolvedSymbol, error) {
	// Search for the symbol using workspace/symbol
	// Pass the raw query - gopls handles qualified names like "Type.Method" natively
	symbols, err := r.client.WorkspaceSymbol(ctx, query.Raw)
	if err != nil {
		return nil, errors.NewLSPError("workspace/symbol", err)
	}

	if len(symbols) == 0 {
		suggestions := r.fallbackSuggestions(ctx, query, 5)
		return nil, errors.NewSymbolNotFound(query.Raw, suggestions)
	}

	// Filter by package/type if specified
	var matches []lsp.SymbolInformation
	for _, sym := range symbols {
		if r.matchesQuery(sym, query) {
			matches = append(matches, sym)
		}
	}

	if len(matches) == 0 {
		// No exact matches - suggest similar symbols
		suggestions := r.suggestFromSymbols(symbols, query, 5)
		return nil, errors.NewSymbolNotFound(query.Raw, suggestions)
	}

	if len(matches) > 1 {
		// Ambiguous - multiple matches
		candidates := make([]string, len(matches))
		for i, m := range matches {
			candidates[i] = r.formatSymbol(m)
		}
		return nil, errors.NewAmbiguousSymbol(query.Raw, candidates)
	}

	// Single match
	match := matches[0]
	return &ResolvedSymbol{
		Name:     r.formatSymbol(match),
		Kind:     match.Kind,
		URI:      match.Location.URI,
		Position: match.Location.Range.Start,
		Range:    match.Location.Range,
	}, nil
}

// FindAll finds all symbols matching the query.
func (r *Resolver) FindAll(ctx context.Context, query *Query) ([]ResolvedSymbol, error) {
	symbols, err := r.client.WorkspaceSymbol(ctx, query.Raw)
	if err != nil {
		return nil, errors.NewLSPError("workspace/symbol", err)
	}

	var results []ResolvedSymbol
	for _, sym := range symbols {
		if r.matchesQuery(sym, query) {
			results = append(results, ResolvedSymbol{
				Name:     r.formatSymbol(sym),
				Kind:     sym.Kind,
				URI:      sym.Location.URI,
				Position: sym.Location.Range.Start,
				Range:    sym.Location.Range,
			})
		}
	}

	return results, nil
}

// matchesQuery checks if a symbol matches the query.
func (r *Resolver) matchesQuery(sym lsp.SymbolInformation, query *Query) bool {
	// If sym.Name matches the full raw query, it's an exact match - no further filtering needed
	// gopls handles qualified queries like "Type.Method" and "pkg.Function" natively
	if sym.Name == query.Raw {
		return true
	}

	// Fallback: check if just the name part matches (for unqualified queries)
	if sym.Name == query.Name {
		// Apply additional filters only for unqualified matches
		if query.Package != "" {
			if !strings.Contains(strings.ToLower(sym.ContainerName), strings.ToLower(query.Package)) {
				if !strings.Contains(strings.ToLower(sym.Location.URI), strings.ToLower(query.Package)) {
					return false
				}
			}
		}
		if query.IsMethod() && sym.Kind != lsp.SymbolKindMethod {
			return false
		}
		return true
	}

	return false
}

// formatSymbol creates a display name for a symbol.
func (r *Resolver) formatSymbol(sym lsp.SymbolInformation) string {
	if sym.ContainerName != "" {
		// gopls often returns Name as "pkg.Symbol" (e.g., "model.Task")
		// and ContainerName as full path (e.g., "github.com/.../model")
		// Avoid duplication by checking if Name already has the short package prefix
		shortPkg := sym.ContainerName
		if idx := strings.LastIndex(sym.ContainerName, "/"); idx >= 0 {
			shortPkg = sym.ContainerName[idx+1:]
		}
		if strings.HasPrefix(sym.Name, shortPkg+".") {
			// Name already includes package prefix, use container path without short pkg
			basePath := sym.ContainerName
			if idx := strings.LastIndex(basePath, "/"); idx >= 0 {
				basePath = basePath[:idx]
			}
			return basePath + "/" + sym.Name
		}
		return sym.ContainerName + "." + sym.Name
	}
	return sym.Name
}

// formatSymbolShort creates a short display name for suggestions.
// gopls returns names like "config.Load" which are already user-friendly.
func (r *Resolver) formatSymbolShort(sym lsp.SymbolInformation) string {
	return sym.Name
}

// suggestFromSymbols generates suggestions from a list of symbols.
func (r *Resolver) suggestFromSymbols(symbols []lsp.SymbolInformation, query *Query, limit int) []string {
	candidates := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		candidates = append(candidates, r.formatSymbolShort(sym))
	}
	return errors.SuggestSimilar(query.Raw, candidates, limit)
}

// fallbackSuggestions tries alternate queries when original returns nothing.
func (r *Resolver) fallbackSuggestions(ctx context.Context, query *Query, limit int) []string {
	var allSymbols []lsp.SymbolInformation
	seen := make(map[string]bool)

	// Try name-only query (e.g., "Task" instead of "db.Task")
	if query.Name != "" && query.Name != query.Raw {
		if symbols, err := r.client.WorkspaceSymbol(ctx, query.Name); err == nil {
			for _, sym := range symbols {
				key := r.formatSymbolShort(sym)
				if !seen[key] {
					seen[key] = true
					allSymbols = append(allSymbols, sym)
				}
			}
		}
	}

	// Try package/type prefix (e.g., "db" to find db.* symbols)
	prefix := query.Package
	if prefix == "" {
		prefix = query.Type
	}
	if prefix != "" && prefix != query.Raw && prefix != query.Name {
		if symbols, err := r.client.WorkspaceSymbol(ctx, prefix); err == nil {
			for _, sym := range symbols {
				key := r.formatSymbolShort(sym)
				if !seen[key] {
					seen[key] = true
					allSymbols = append(allSymbols, sym)
				}
			}
		}
	}

	if len(allSymbols) == 0 {
		return nil
	}

	return r.suggestFromSymbols(allSymbols, query, limit)
}

// FindSimilar finds symbols similar to the given query.
// Used to populate similar_symbols in successful responses.
func (r *Resolver) FindSimilar(ctx context.Context, query *Query, limit int) []string {
	// Query workspace symbols using the name part
	symbols, err := r.client.WorkspaceSymbol(ctx, query.Name)
	if err != nil || len(symbols) == 0 {
		return nil
	}

	// Collect unique symbols, excluding exact match
	seen := make(map[string]bool)
	var candidates []lsp.SymbolInformation
	for _, sym := range symbols {
		short := r.formatSymbolShort(sym)
		// Skip exact match to the query or just the name part
		if short == query.Raw || short == query.Name {
			continue
		}
		if !seen[short] {
			seen[short] = true
			candidates = append(candidates, sym)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	return r.suggestFromSymbols(candidates, query, limit)
}
