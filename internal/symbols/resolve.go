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
	symbols, err := r.client.WorkspaceSymbol(ctx, query.Name)
	if err != nil {
		return nil, errors.NewLSPError("workspace/symbol", err)
	}

	if len(symbols) == 0 {
		return nil, errors.NewSymbolNotFound(query.Raw, nil)
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
	symbols, err := r.client.WorkspaceSymbol(ctx, query.Name)
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
	// Name must match
	if sym.Name != query.Name {
		return false
	}

	// If query specifies a package, container should contain it
	if query.Package != "" {
		if !strings.Contains(strings.ToLower(sym.ContainerName), strings.ToLower(query.Package)) {
			// Also check URI for package path
			if !strings.Contains(strings.ToLower(sym.Location.URI), strings.ToLower(query.Package)) {
				return false
			}
		}
	}

	// If query specifies a type, container should match
	if query.Type != "" {
		if !strings.Contains(sym.ContainerName, query.Type) {
			return false
		}
	}

	// If query is for a method, symbol should be a method
	if query.IsMethod() && sym.Kind != lsp.SymbolKindMethod {
		return false
	}

	return true
}

// formatSymbol creates a display name for a symbol.
func (r *Resolver) formatSymbol(sym lsp.SymbolInformation) string {
	if sym.ContainerName != "" {
		return sym.ContainerName + "." + sym.Name
	}
	return sym.Name
}

// suggestFromSymbols generates suggestions from a list of symbols.
func (r *Resolver) suggestFromSymbols(symbols []lsp.SymbolInformation, query *Query, limit int) []string {
	candidates := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		candidates = append(candidates, r.formatSymbol(sym))
	}
	return errors.SuggestSimilar(query.Raw, candidates, limit)
}
