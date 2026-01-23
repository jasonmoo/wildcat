package golang

import (
	"fmt"
	"go/ast"
	"go/token"
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
)

// ParseKind parses a flexible kind string into SymbolKind.
// Accepts variations like "func", "function", "fn" -> SymbolKindFunc.
// Returns empty string if not recognized.
func ParseKind(s string) SymbolKind {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "func", "function", "fn":
		return SymbolKindFunc
	case "method", "meth":
		return SymbolKindMethod
	case "type", "struct":
		return SymbolKindType
	case "interface", "iface":
		return SymbolKindInterface
	case "const", "constant":
		return SymbolKindConst
	case "var", "variable":
		return SymbolKindVar
	default:
		return ""
	}
}

// ParseKinds parses a comma-separated list of kind strings.
// Unknown kinds are silently ignored.
func ParseKinds(s string) []SymbolKind {
	if s == "" {
		return nil
	}
	var kinds []SymbolKind
	for _, part := range strings.Split(s, ",") {
		if k := ParseKind(part); k != "" {
			kinds = append(kinds, k)
		}
	}
	return kinds
}

// Symbol represents a searchable symbol from the AST
// Stores pointers to AST nodes for lazy rendering
type Symbol struct {
	Name    string     // symbol name (for matching)
	Kind    SymbolKind // func, method, type, const, var
	Package *Package

	// For lazy rendering
	filename string
	pos      token.Pos
	node     ast.Node    // *ast.FuncDecl, *ast.TypeSpec, or *ast.ValueSpec
	tok      token.Token // for GenDecl (CONST, VAR, TYPE)
}

// Signature renders the symbol's signature on demand
func (s *Symbol) Signature() string {
	switch n := s.node.(type) {
	case *ast.FuncDecl:
		return FormatFuncDecl(n)
	case *ast.TypeSpec:
		return FormatTypeSpec(s.tok, n)
	case *ast.ValueSpec:
		return FormatValueSpec(s.tok, n)
	}
	return s.Name
}

// Location renders the symbol's line range (start:end)
func (s *Symbol) Location() string {
	start := s.Package.Package.Fset.Position(s.pos)
	end := s.Package.Package.Fset.Position(s.node.End())
	return fmt.Sprintf("%d:%d", start.Line, end.Line)
}

// Filename returns the absolute path to the file containing this symbol
func (s *Symbol) Filename() string {
	return s.filename
}

// Pos returns the token.Pos of the symbol
func (s *Symbol) Pos() token.Pos {
	return s.pos
}

// Node returns the underlying AST node (*ast.FuncDecl, *ast.TypeSpec, or *ast.ValueSpec)
func (s *Symbol) Node() ast.Node {
	return s.node
}

// SearchName returns the fully qualified searchable name (PkgPath.Name)
// This allows fuzzy matching like "lsp.Client" or "wildcatlspclient"
func (s *Symbol) SearchName() string {
	return s.Package.Identifier.PkgPath + "." + s.Name
}

// SymbolIndex holds symbols for fuzzy searching
type SymbolIndex struct {
	symbols    []Symbol
	modulePath string
}

// Symbols returns all collected symbols
func (idx *SymbolIndex) Symbols() []Symbol {
	return idx.symbols
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
func (idx *SymbolIndex) Lookup(query string) []*Symbol {
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
		for i := range idx.symbols {
			if idx.symbols[i].Package.Identifier.PkgPath == pkgPath && idx.symbols[i].Name == symbolName {
				return []*Symbol{&idx.symbols[i]}
			}
		}

		// Try as relative path within module (e.g., "internal/commands/package")
		if idx.modulePath != "" {
			fullPath := path.Join(idx.modulePath, pkgPath)
			for i := range idx.symbols {
				if idx.symbols[i].Package.Identifier.PkgPath == fullPath && idx.symbols[i].Name == symbolName {
					return []*Symbol{&idx.symbols[i]}
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
		var matches []*Symbol
		for i := range idx.symbols {
			if idx.symbols[i].Name == parts[0] {
				matches = append(matches, &idx.symbols[i])
			}
		}
		return matches

	case 2:
		// Could be "pkg.Name" or "Type.Method"
		// Try pkg.Name first (using actual Go package name, not directory)
		var matches []*Symbol
		for i := range idx.symbols {
			pkgName := idx.symbols[i].Package.Identifier.Name
			if pkgName == parts[0] && idx.symbols[i].Name == parts[1] {
				matches = append(matches, &idx.symbols[i])
			}
		}
		if len(matches) > 0 {
			return matches
		}
		// Try Type.Method (symbol name includes receiver)
		methodName := parts[0] + "." + parts[1]
		for i := range idx.symbols {
			if idx.symbols[i].Name == methodName {
				matches = append(matches, &idx.symbols[i])
			}
		}
		return matches

	case 3:
		// "pkg.Type.Method" (using actual Go package name, not directory)
		methodName := parts[1] + "." + parts[2]
		var matches []*Symbol
		for i := range idx.symbols {
			pkgName := idx.symbols[i].Package.Identifier.Name
			if pkgName == parts[0] && idx.symbols[i].Name == methodName {
				matches = append(matches, &idx.symbols[i])
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

func (s fullSource) String(i int) string { return s.idx.symbols[i].SearchName() }
func (s fullSource) Len() int            { return s.idx.Len() }

// SearchResult pairs a match with its symbol
type SearchResult struct {
	Symbol         *Symbol
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
		sym := &idx.symbols[s.index]

		// Filter by kind
		if opts != nil && len(opts.Kinds) > 0 && !slices.Contains(opts.Kinds, sym.Kind) {
			continue
		}

		// Exclude specific symbols
		if opts != nil && len(opts.Exclude) > 0 {
			fullName := sym.Package.Identifier.Name + "." + sym.Name
			if slices.Contains(opts.Exclude, fullName) {
				continue
			}
		}

		results = append(results, SearchResult{
			Symbol:         sym,
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

// CollectSymbols builds a SymbolIndex from loaded packages
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

		for _, f := range pkg.Package.Syntax {
			filename := pkg.Package.Fset.Position(f.Pos()).Filename

			for _, decl := range f.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					idx.addFunc(pkg, filename, d)
				case *ast.GenDecl:
					idx.addGenDecl(pkg, filename, d)
				}
			}
		}
	}

	return idx
}

func (idx *SymbolIndex) addFunc(pkg *Package, filename string, d *ast.FuncDecl) {
	name := d.Name.Name
	kind := SymbolKindFunc

	// Method
	if d.Recv != nil && len(d.Recv.List) > 0 {
		kind = SymbolKindMethod
		recvType := ReceiverTypeName(d.Recv.List[0].Type)
		if recvType != "" {
			name = recvType + "." + name
		}
	}

	idx.symbols = append(idx.symbols, Symbol{
		Name:     name,
		Kind:     kind,
		Package:  pkg,
		filename: filename,
		pos:      d.Pos(),
		node:     d,
	})
}

func (idx *SymbolIndex) addGenDecl(pkg *Package, filename string, d *ast.GenDecl) {
	for _, spec := range d.Specs {
		switch sp := spec.(type) {
		case *ast.TypeSpec:
			idx.addTypeSpec(pkg, filename, d.Tok, sp)
		case *ast.ValueSpec:
			idx.addValueSpec(pkg, filename, d.Tok, sp)
		}
	}
}

func (idx *SymbolIndex) addTypeSpec(pkg *Package, filename string, tok token.Token, sp *ast.TypeSpec) {
	name := sp.Name.Name
	kind := SymbolKindType

	if _, ok := sp.Type.(*ast.InterfaceType); ok {
		kind = SymbolKindInterface
	}

	idx.symbols = append(idx.symbols, Symbol{
		Name:     name,
		Kind:     kind,
		Package:  pkg,
		filename: filename,
		pos:      sp.Pos(),
		node:     sp,
		tok:      tok,
	})
}

func (idx *SymbolIndex) addValueSpec(pkg *Package, filename string, tok token.Token, sp *ast.ValueSpec) {
	kind := SymbolKindVar
	if tok == token.CONST {
		kind = SymbolKindConst
	}

	// ValueSpec can have multiple names (e.g., var a, b, c int)
	for _, ident := range sp.Names {
		idx.symbols = append(idx.symbols, Symbol{
			Name:     ident.Name,
			Kind:     kind,
			Package:  pkg,
			filename: filename,
			pos:      ident.Pos(),
			node:     sp,
			tok:      tok,
		})
	}
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

	for i := range idx.symbols {
		sym := &idx.symbols[i]

		// Filter by kind if specified
		if opts != nil && len(opts.Kinds) > 0 && !slices.Contains(opts.Kinds, sym.Kind) {
			continue
		}

		// Exclude specific symbols
		if opts != nil && len(opts.Exclude) > 0 {
			fullName := sym.Package.Identifier.Name + "." + sym.Name
			if slices.Contains(opts.Exclude, fullName) {
				continue
			}
		}

		// Match against symbol name only
		if pattern.MatchString(sym.Name) {
			results = append(results, SearchResult{
				Symbol: sym,
				Score:  len(sym.Name), // Use length for sorting (shorter = better)
			})
		}
	}

	// Sort by name length ascending (shorter first), then alphabetically
	slices.SortFunc(results, func(a, b SearchResult) int {
		if a.Score != b.Score {
			return a.Score - b.Score // ascending (shorter first)
		}
		return strings.Compare(a.Symbol.Name, b.Symbol.Name)
	})

	// Apply limit
	if opts != nil && opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return results
}
