package symbols

import (
	"context"
	"os"
	"strings"

	"github.com/jasonmoo/wildcat/internal/errors"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/lsp"
)

// ResolvedSymbol represents a successfully resolved symbol.
type ResolvedSymbol struct {
	Name     string // Full symbol name
	Kind     lsp.SymbolKind
	URI      string       // File URI
	Position lsp.Position // Position in file
	Range    lsp.Range    // Full range of symbol
}

// Resolver resolves symbol queries using an LSP client.
type Resolver struct {
	client  *lsp.Client
	workDir string
}

// NewResolver creates a new symbol resolver.
func NewDefaultResolver(client *lsp.Client) *Resolver {
	workDir, _ := os.Getwd()
	return &Resolver{client: client, workDir: workDir}
}

// NewResolver creates a new symbol resolver.
func NewResolver(client *lsp.Client, workDir string) *Resolver {
	return &Resolver{client: client, workDir: workDir}
}

// Resolve finds a symbol matching the query.
// Returns an error with suggestions if not found or ambiguous.
func (r *Resolver) Resolve(ctx context.Context, query *Query) (*ResolvedSymbol, error) {
	// Determine search term and resolved package path
	searchTerm := query.Raw
	resolvedPkg := ""

	// If package is path-qualified (contains "/"), resolve it and search by name only
	// gopls doesn't understand path prefixes like "internal/lsp.Client"
	if query.Package != "" && strings.Contains(query.Package, "/") {
		if resolved, err := golang.ResolvePackagePath(query.Package, r.workDir); err == nil {
			resolvedPkg = resolved
			searchTerm = query.Name // Search "Client" not "internal/lsp.Client"
		}
	}

	// Search for the symbol using workspace/symbol
	symbols, err := r.client.WorkspaceSymbol(ctx, searchTerm)
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
		if r.matchesQuery(sym, query, resolvedPkg) {
			matches = append(matches, sym)
		}
	}

	if len(matches) == 0 {
		// No exact matches - suggest similar symbols
		suggestions := r.suggestFromSymbols(ctx, query, 5)
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
	// Determine search term and resolved package path
	searchTerm := query.Raw
	resolvedPkg := ""

	if query.Package != "" && strings.Contains(query.Package, "/") {
		if resolved, err := golang.ResolvePackagePath(query.Package, r.workDir); err == nil {
			resolvedPkg = resolved
			searchTerm = query.Name
		}
	}

	symbols, err := r.client.WorkspaceSymbol(ctx, searchTerm)
	if err != nil {
		return nil, errors.NewLSPError("workspace/symbol", err)
	}

	var results []ResolvedSymbol
	for _, sym := range symbols {
		if r.matchesQuery(sym, query, resolvedPkg) {
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
// resolvedPkg is the fully resolved import path when query had a path-qualified package.
func (r *Resolver) matchesQuery(sym lsp.SymbolInformation, query *Query, resolvedPkg string) bool {
	// If sym.Name matches the full raw query, it's an exact match - no further filtering needed
	// gopls handles qualified queries like "Type.Method" and "pkg.Function" natively
	if sym.Name == query.Raw {
		return true
	}

	// Fallback: check if just the name part matches (for unqualified queries)
	if sym.Name == query.Name {
		// If we have a resolved package path, require exact match
		if resolvedPkg != "" {
			if sym.ContainerName != resolvedPkg {
				return false
			}
		} else if query.Package != "" {
			// Original behavior for non-path-qualified packages
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

// formatSymbolShort creates a short display name like "pkg.Symbol".
// Always includes the package prefix for consistent filtering.
func (r *Resolver) formatSymbolShort(sym lsp.SymbolInformation) string {
	return sym.ShortName()
}

// suggestFromSymbols queries gopls with the name part and filters by package prefix.
func (r *Resolver) suggestFromSymbols(ctx context.Context, query *Query, limit int) []string {
	// Query with just the name for better fuzzy matching
	searchTerm := query.Name
	if searchTerm == "" {
		searchTerm = query.Raw
	}

	symbols, err := r.client.WorkspaceSymbol(ctx, searchTerm)
	if err != nil || len(symbols) == 0 {
		return nil
	}

	// Determine prefix filter from query
	prefix := ""
	if query.Package != "" {
		prefix = strings.ToLower(query.Package + ".")
	} else if query.Type != "" {
		prefix = strings.ToLower(query.Type + ".")
	}

	var candidates []string
	for _, sym := range symbols {
		name := sym.ShortName()
		// Filter by prefix if specified
		if prefix != "" && !strings.HasPrefix(strings.ToLower(name), prefix) {
			continue
		}
		candidates = append(candidates, name)
		if len(candidates) >= limit {
			break
		}
	}

	return candidates
}

// fallbackSuggestions tries alternate queries when original returns nothing.
func (r *Resolver) fallbackSuggestions(ctx context.Context, query *Query, limit int) []string {
	// Query with just the name part, filter by package prefix
	return r.suggestFromSymbols(ctx, query, limit)
}

// FindSimilar finds symbols similar to the given query.
// Used to populate similar_symbols in successful responses.
func (r *Resolver) FindSimilar(ctx context.Context, query *Query, limit int) []string {
	// Get suggestions and filter out exact matches
	suggestions := r.suggestFromSymbols(ctx, query, limit+1)

	// Exclude exact match to the query
	var result []string
	for _, s := range suggestions {
		if s == query.Raw {
			continue
		}
		result = append(result, s)
		if len(result) >= limit {
			break
		}
	}
	return result
}
