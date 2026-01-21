package golang

import (
	"fmt"
	"go/ast"
	"go/token"
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
func (s *Symbol) Signature() (string, error) {
	switch n := s.node.(type) {
	case *ast.FuncDecl:
		return FormatFuncDecl(n)
	case *ast.TypeSpec:
		return FormatTypeSpec(s.tok, n)
	case *ast.ValueSpec:
		return FormatValueSpec(s.tok, n)
	}
	return s.Name, nil
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
	symbols []Symbol
}

// Symbols returns all collected symbols
func (idx *SymbolIndex) Symbols() []Symbol {
	return idx.symbols
}

func (idx *SymbolIndex) Len() int {
	return len(idx.symbols)
}

// Lookup finds a symbol by exact match. Query formats:
//   - "FuncName" - matches any symbol with that name
//   - "pkg.FuncName" - matches by short package name + symbol
//   - "Type.Method" - matches method on type
//   - "pkg.Type.Method" - matches by short package + type + method
//   - "github.com/user/repo/pkg.Symbol" - matches by full import path
//
// Returns nil if not found or if multiple matches exist for ambiguous queries.
func (idx *SymbolIndex) Lookup(query string) *Symbol {
	// Check if query contains a full import path (has multiple slashes)
	if strings.Count(query, "/") > 0 {
		// Full import path: github.com/user/repo/pkg.Symbol
		lastDot := strings.LastIndex(query, ".")
		if lastDot == -1 {
			return nil
		}
		pkgPath := query[:lastDot]
		symbolName := query[lastDot+1:]

		for i := range idx.symbols {
			if idx.symbols[i].Package.Identifier.PkgPath == pkgPath && idx.symbols[i].Name == symbolName {
				return &idx.symbols[i]
			}
		}
		return nil
	}

	// Short form: might be "Name", "pkg.Name", "Type.Method", or "pkg.Type.Method"
	parts := strings.Split(query, ".")

	switch len(parts) {
	case 1:
		// Just "Name" - find unique match
		var match *Symbol
		for i := range idx.symbols {
			if idx.symbols[i].Name == parts[0] {
				if match != nil {
					return nil // ambiguous
				}
				match = &idx.symbols[i]
			}
		}
		return match

	case 2:
		// Could be "pkg.Name" or "Type.Method"
		// Try pkg.Name first
		var match *Symbol
		for i := range idx.symbols {
			shortPkg := idx.symbols[i].Package.Identifier.PkgPath
			if lastSlash := strings.LastIndex(shortPkg, "/"); lastSlash >= 0 {
				shortPkg = shortPkg[lastSlash+1:]
			}
			if shortPkg == parts[0] && idx.symbols[i].Name == parts[1] {
				if match != nil {
					return nil // ambiguous
				}
				match = &idx.symbols[i]
			}
		}
		if match != nil {
			return match
		}
		// Try Type.Method (symbol name includes receiver)
		methodName := parts[0] + "." + parts[1]
		for i := range idx.symbols {
			if idx.symbols[i].Name == methodName {
				if match != nil {
					return nil // ambiguous
				}
				match = &idx.symbols[i]
			}
		}
		return match

	case 3:
		// "pkg.Type.Method"
		methodName := parts[1] + "." + parts[2]
		var match *Symbol
		for i := range idx.symbols {
			shortPkg := idx.symbols[i].Package.Identifier.PkgPath
			if lastSlash := strings.LastIndex(shortPkg, "/"); lastSlash >= 0 {
				shortPkg = shortPkg[lastSlash+1:]
			}
			if shortPkg == parts[0] && idx.symbols[i].Name == methodName {
				if match != nil {
					return nil // ambiguous
				}
				match = &idx.symbols[i]
			}
		}
		return match
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
	Exclude string       // symbol name to exclude from results (e.g., "pkg.Symbol")
}

// Search performs fuzzy search on the symbol index.
// For plain queries, searches Name only.
// For package-qualified queries (containing "."), also searches PkgPath.Name.
func (idx *SymbolIndex) Search(query string, opts *SearchOptions) []SearchResult {
	// Search against symbol names (always)
	nameMatches := fuzzy.FindFrom(query, nameSource{idx})

	// Search against full path only for package-qualified queries like "lsp.Client"
	var fullMatches []fuzzy.Match
	if strings.Contains(query, ".") {
		fullMatches = fuzzy.FindFrom(query, fullSource{idx})
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

		// Exclude specific symbol
		if opts != nil && opts.Exclude != "" {
			fullName := sym.Package.Identifier.Name + "." + sym.Name
			if fullName == opts.Exclude {
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

		if pkg.Package.TypesInfo == nil {
			continue
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
