package golang

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/sahilm/fuzzy"
)

// SymbolKind represents the kind of symbol
type SymbolKind string

const (
	SymbolKindFunc      SymbolKind = "func"
	SymbolKindMethod    SymbolKind = "method"
	SymbolKindType      SymbolKind = "type" // struct, type alias
	SymbolKindInterface SymbolKind = "interface"
	SymbolKindConst     SymbolKind = "const"
	SymbolKindVar       SymbolKind = "var"
	SymbolKindPkgName   SymbolKind = "package" // imported package name
	SymbolKindLabel     SymbolKind = "label"   // goto label
	SymbolKindBuiltin   SymbolKind = "builtin" // builtin function
	SymbolKindNil       SymbolKind = "nil"     // predeclared nil
	SymbolKindUnknown   SymbolKind = "unknown" // unrecognized types.Object
)

// KindAliases maps accepted kind names to their SymbolKind.
var KindAliases = map[string]SymbolKind{
	"func": SymbolKindFunc, "function": SymbolKindFunc,
	"method": SymbolKindMethod,
	"type":   SymbolKindType, "struct": SymbolKindType,
	"interface": SymbolKindInterface, "iface": SymbolKindInterface,
	"const": SymbolKindConst, "constant": SymbolKindConst,
	"var": SymbolKindVar, "variable": SymbolKindVar,
}

// ParseKind parses a kind string into SymbolKind.
// Returns empty string if not recognized.
func ParseKind(s string) SymbolKind {
	return KindAliases[strings.ToLower(strings.TrimSpace(s))]
}

// ParseKinds parses a comma-separated list of kind strings.
// Returns an error if any kind is not recognized.
func ParseKinds(s string) ([]SymbolKind, error) {
	if s == "" {
		return nil, nil
	}
	var kinds []SymbolKind
	var unknown []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if k := ParseKind(part); k != "" {
			kinds = append(kinds, k)
		} else {
			unknown = append(unknown, part)
		}
	}
	if len(unknown) > 0 {
		valid := make([]string, 0, len(KindAliases))
		for k := range KindAliases {
			valid = append(valid, k)
		}
		slices.Sort(valid)
		return nil, fmt.Errorf("unknown kind(s): %v; valid: %v", unknown, valid)
	}
	return kinds, nil
}

// indexedSymbol holds a symbol entry in the search index.
// Name may differ from Symbol.Name for methods (e.g., "Type.Method" vs "Method").
type indexedSymbol struct {
	Name   string
	Symbol *PackageSymbol
}

// SymbolIndex holds symbols for fuzzy searching
type SymbolIndex struct {
	symbols    []indexedSymbol
	modulePath string
}

// Symbols returns all indexed PackageSymbols
func (idx *SymbolIndex) Symbols() []*PackageSymbol {
	result := make([]*PackageSymbol, len(idx.symbols))
	for i := range idx.symbols {
		result[i] = idx.symbols[i].Symbol
	}
	return result
}

func (idx *SymbolIndex) Len() int {
	return len(idx.symbols)
}

// Lookup finds symbols by exact match. Query formats:
//   - "FuncName" - matches any symbol with that name
//   - "pkg.FuncName" - matches by short package name + symbol
//   - "Type.Method" - matches method on type
//   - "pkg.Type.Method" - matches by short package + type + method
//   - "github.com/user/repo/pkg.Symbol" - matches by full import path
//
// Returns all matching symbols:
//   - Empty slice: symbol not found
//   - Single element: exact match
//   - Multiple elements: ambiguous query, returns all candidates
func (idx *SymbolIndex) Lookup(query string) []*PackageSymbol {
	// Check if query contains a path (has slashes)
	if strings.Count(query, "/") > 0 {
		// Path-based lookup: full path or relative path
		// Find first dot after last slash to separate package from symbol
		lastSlash := strings.LastIndex(query, "/")
		dotAfterSlash := strings.Index(query[lastSlash+1:], ".")
		if dotAfterSlash == -1 {
			return nil
		}
		splitPos := lastSlash + 1 + dotAfterSlash
		pkgPath := query[:splitPos]
		symbolName := query[splitPos+1:]

		// Try exact match first (full import path)
		for _, entry := range idx.symbols {
			if entry.Symbol.PackageIdentifier.PkgPath == pkgPath && entry.Name == symbolName {
				return []*PackageSymbol{entry.Symbol}
			}
		}

		// Try as relative path within module (e.g., "internal/commands/package")
		if idx.modulePath != "" {
			fullPath := path.Join(idx.modulePath, pkgPath)
			for _, entry := range idx.symbols {
				if entry.Symbol.PackageIdentifier.PkgPath == fullPath && entry.Name == symbolName {
					return []*PackageSymbol{entry.Symbol}
				}
			}
		}
		return nil
	}

	// Short form: might be "Name", "pkg.Name", "Type.Method", or "pkg.Type.Method"
	parts := strings.Split(query, ".")

	switch len(parts) {
	case 1:
		// Just "Name" - find all matches
		var matches []*PackageSymbol
		for _, entry := range idx.symbols {
			if entry.Name == parts[0] {
				matches = append(matches, entry.Symbol)
			}
		}
		return matches

	case 2:
		// Could be "pkg.Name" or "Type.Method"
		// Try pkg.Name first (using actual Go package name, not directory)
		var matches []*PackageSymbol
		for _, entry := range idx.symbols {
			pkgName := entry.Symbol.PackageIdentifier.Name
			if pkgName == parts[0] && entry.Name == parts[1] {
				matches = append(matches, entry.Symbol)
			}
		}
		if len(matches) > 0 {
			return matches
		}
		// Try Type.Method (symbol name includes receiver)
		methodName := parts[0] + "." + parts[1]
		for _, entry := range idx.symbols {
			if entry.Name == methodName {
				matches = append(matches, entry.Symbol)
			}
		}
		return matches

	case 3:
		// "pkg.Type.Method" (using actual Go package name, not directory)
		methodName := parts[1] + "." + parts[2]
		var matches []*PackageSymbol
		for _, entry := range idx.symbols {
			pkgName := entry.Symbol.PackageIdentifier.Name
			if pkgName == parts[0] && entry.Name == methodName {
				matches = append(matches, entry.Symbol)
			}
		}
		return matches
	}

	return nil
}

// nameSource adapts SymbolIndex to match against Name only
type nameSource struct{ idx *SymbolIndex }

func (s nameSource) String(i int) string { return s.idx.symbols[i].Name }
func (s nameSource) Len() int            { return s.idx.Len() }

// fullSource adapts SymbolIndex to match against PkgPath.Name
type fullSource struct{ idx *SymbolIndex }

func (s fullSource) String(i int) string {
	entry := s.idx.symbols[i]
	if entry.Symbol.PackageIdentifier == nil {
		return entry.Name
	}
	return entry.Symbol.PackageIdentifier.PkgPath + "." + entry.Name
}
func (s fullSource) Len() int { return s.idx.Len() }

// SearchResult pairs a match with its symbol
type SearchResult struct {
	Symbol         *PackageSymbol
	Name           string // search name (may differ from Symbol.Name for methods)
	Score          int
	MatchedIndexes []int
}

// SearchOptions configures symbol search behavior
type SearchOptions struct {
	Limit   int          // max results (0 = no limit)
	Kinds   []SymbolKind // filter by kind (nil = all kinds)
	Exclude []string     // symbol names to exclude from results (e.g., "pkg.Symbol", "pkg.Type.Method")
}

// Search performs fuzzy search on the symbol index.
// For plain queries, searches Name only.
// For package-qualified queries (containing "."), also searches PkgPath.Name.
// Also performs reverse matching to find shorter symbols that are "inside" the query
// (e.g., query "NewServerClient" finds symbol "NewClient").
func (idx *SymbolIndex) Search(query string, opts *SearchOptions) []SearchResult {
	// Forward search: find targets where query chars appear in order (current behavior)
	nameMatches := fuzzy.FindFrom(query, nameSource{idx})

	// Search against full path only for package-qualified queries like "lsp.Client"
	var fullMatches []fuzzy.Match
	if strings.Contains(query, ".") {
		fullMatches = fuzzy.FindFrom(query, fullSource{idx})
	}

	// Reverse search: find symbols whose name appears "inside" the query.
	// This handles cases like query "NewServerClient" finding "NewClient",
	// where the symbol is shorter than the query.
	queryAsTarget := []string{query}
	var reverseMatches []fuzzy.Match
	for i := range idx.symbols {
		// Only check if symbol name is shorter than query (reverse case)
		name := idx.symbols[i].Name
		if len(name) >= len(query) {
			continue
		}
		matches := fuzzy.Find(name, queryAsTarget)
		if len(matches) > 0 {
			reverseMatches = append(reverseMatches, fuzzy.Match{
				Index:          i,
				Score:          matches[0].Score,
				MatchedIndexes: matches[0].MatchedIndexes,
			})
		}
	}

	// Scoring function: reward length similarity and case match
	scoreMatch := func(name string, fuzzyScore int) int {
		score := fuzzyScore

		// Length similarity: penalize difference in length
		// Closer length = higher score
		lenDiff := len(name) - len(query)
		if lenDiff < 0 {
			lenDiff = -lenDiff
		}
		score -= lenDiff * 10

		// Case-sensitive match bonus
		if strings.Contains(name, query) {
			score += 100
		}

		return score
	}

	// Merge results: for each symbol, take the best score
	scores := make(map[int]int)    // symbol index -> best score
	matched := make(map[int][]int) // symbol index -> matched indexes

	for _, m := range nameMatches {
		name := idx.symbols[m.Index].Name
		score := scoreMatch(name, m.Score)
		if score > scores[m.Index] {
			scores[m.Index] = score
			matched[m.Index] = m.MatchedIndexes
		}
	}

	for _, m := range fullMatches {
		// Full path matches get base fuzzy score (no length bonus since path is long)
		if m.Score > scores[m.Index] {
			scores[m.Index] = m.Score
			matched[m.Index] = m.MatchedIndexes
		}
	}

	for _, m := range reverseMatches {
		// Reverse matches: symbol name found inside query
		name := idx.symbols[m.Index].Name
		score := scoreMatch(name, m.Score)
		if score > scores[m.Index] {
			scores[m.Index] = score
			matched[m.Index] = m.MatchedIndexes
		}
	}

	// Build results sorted by score
	type scored struct {
		index int
		score int
	}
	var sorted []scored
	for i, s := range scores {
		sorted = append(sorted, scored{i, s})
	}
	slices.SortFunc(sorted, func(a, b scored) int {
		return b.score - a.score // descending
	})

	// Apply filters and limit
	results := make([]SearchResult, 0, len(sorted))
	for _, s := range sorted {
		entry := idx.symbols[s.index]

		// Filter by kind
		if opts != nil && len(opts.Kinds) > 0 && !slices.Contains(opts.Kinds, entry.Symbol.Kind) {
			continue
		}

		// Exclude specific symbols
		if opts != nil && len(opts.Exclude) > 0 {
			fullName := entry.Symbol.PackageIdentifier.Name + "." + entry.Name
			if slices.Contains(opts.Exclude, fullName) {
				continue
			}
		}

		results = append(results, SearchResult{
			Symbol:         entry.Symbol,
			Name:           entry.Name,
			Score:          s.score,
			MatchedIndexes: matched[s.index],
		})

		// Apply limit
		if opts != nil && opts.Limit > 0 && len(results) >= opts.Limit {
			break
		}
	}

	return results
}

// CollectSymbols builds a SymbolIndex from loaded packages.
// Uses precomputed Package.Symbols instead of walking AST.
func CollectSymbols(pkgs []*Package) *SymbolIndex {
	idx := &SymbolIndex{}

	for _, pkg := range pkgs {
		// Skip packages with incomplete type info (diagnostics emitted at load time)
		if pkg.Package.TypesInfo == nil {
			continue
		}
		if idx.modulePath == "" {
			idx.modulePath = pkg.Identifier.ModulePath
		}

		for _, sym := range pkg.Symbols {
			// Add the symbol itself
			idx.symbols = append(idx.symbols, indexedSymbol{
				Name:   sym.Name,
				Symbol: sym,
			})

			// For types, also add methods with qualified names
			for _, m := range sym.Methods {
				idx.symbols = append(idx.symbols, indexedSymbol{
					Name:   sym.Name + "." + m.Name, // "Type.Method"
					Symbol: m,
				})
			}
		}
	}

	return idx
}

// IsRegexPattern detects if a query contains regex metacharacters.
// Returns true if the query should be treated as a regex pattern.
func IsRegexPattern(query string) bool {
	// Common regex metacharacters that indicate intent to use regex
	// Excluding '.' since it's used for package.Symbol qualification
	metaChars := []byte{'*', '+', '?', '^', '$', '[', ']', '{', '}', '(', ')', '|', '\\'}
	for _, c := range metaChars {
		if strings.ContainsRune(query, rune(c)) {
			return true
		}
	}
	return false
}

// RegexSearch searches symbols using a compiled regex pattern against symbol names.
// Matches against the symbol Name field only (e.g., "Client", "Type.Method").
// Results are sorted by symbol name length (shorter first).
func (idx *SymbolIndex) RegexSearch(pattern *regexp.Regexp, opts *SearchOptions) []SearchResult {
	var results []SearchResult

	for _, entry := range idx.symbols {
		// Filter by kind if specified
		if opts != nil && len(opts.Kinds) > 0 && !slices.Contains(opts.Kinds, entry.Symbol.Kind) {
			continue
		}

		// Exclude specific symbols
		if opts != nil && len(opts.Exclude) > 0 {
			fullName := entry.Symbol.PackageIdentifier.Name + "." + entry.Name
			if slices.Contains(opts.Exclude, fullName) {
				continue
			}
		}

		// Match against symbol name only
		if pattern.MatchString(entry.Name) {
			results = append(results, SearchResult{
				Symbol: entry.Symbol,
				Name:   entry.Name,
				Score:  len(entry.Name), // Use length for sorting (shorter = better)
			})
		}
	}

	// Sort by name length ascending (shorter first), then alphabetically
	slices.SortFunc(results, func(a, b SearchResult) int {
		if a.Score != b.Score {
			return a.Score - b.Score // ascending (shorter first)
		}
		return strings.Compare(a.Name, b.Name)
	})

	// Apply limit
	if opts != nil && opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return results
}
