package golang

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/sahilm/fuzzy"
	"golang.org/x/tools/go/packages"
)

// SearchMode determines how the query is matched
type SearchMode string

const (
	SearchModeFuzzy SearchMode = "fuzzy" // fuzzy matching (default)
	SearchModeRegex SearchMode = "regex" // regex on symbol name
)

// SymbolKind represents the kind of symbol (matches Go declaration tokens)
type SymbolKind string

const (
	SymbolKindFunc  SymbolKind = "func"  // functions and methods
	SymbolKindType  SymbolKind = "type"  // struct, interface, type alias
	SymbolKindConst SymbolKind = "const"
	SymbolKindVar   SymbolKind = "var"
)

// Symbol represents a searchable symbol from the AST
// Stores pointers to AST nodes for lazy rendering
type Symbol struct {
	Name    string     // symbol name (for matching)
	Kind    SymbolKind // func, method, type, const, var
	PkgPath string     // full import path

	// For lazy rendering
	fset     *token.FileSet
	filename string
	pos      token.Pos
	node     ast.Node    // *ast.FuncDecl, *ast.TypeSpec, or *ast.ValueSpec
	tok      token.Token // for GenDecl (CONST, VAR, TYPE)
}

// Signature renders the symbol's signature on demand
func (s *Symbol) Signature() string {
	switch n := s.node.(type) {
	case *ast.FuncDecl:
		sig, err := FormatFuncDecl(n)
		if err != nil {
			return s.Name
		}
		return sig
	case *ast.TypeSpec:
		sig, err := FormatTypeSpec(s.tok, n)
		if err != nil {
			return s.Name
		}
		return sig
	case *ast.ValueSpec:
		sig, err := FormatValueSpec(s.tok, n)
		if err != nil {
			return s.Name
		}
		return sig
	}
	return s.Name
}

// Location renders the symbol's location on demand
func (s *Symbol) Location() string {
	p := s.fset.Position(s.pos)
	return fmt.Sprintf("%s:%d", filepath.Base(s.filename), p.Line)
}

// SymbolIndex holds symbols for fuzzy searching
type SymbolIndex struct {
	symbols []Symbol
}

// String implements fuzzy.Source
func (idx *SymbolIndex) String(i int) string {
	return idx.symbols[i].Name
}

// Len implements fuzzy.Source
func (idx *SymbolIndex) Len() int {
	return len(idx.symbols)
}

// Symbols returns all collected symbols
func (idx *SymbolIndex) Symbols() []Symbol {
	return idx.symbols
}

// SearchResult pairs a match with its symbol
type SearchResult struct {
	Symbol         *Symbol
	Score          int
	MatchedIndexes []int
}

// SearchOptions configures symbol search behavior
type SearchOptions struct {
	Mode  SearchMode   // search mode (default: fuzzy)
	Limit int          // max results (0 = no limit)
	Kinds []SymbolKind // filter by kind (nil = all kinds)
}

// Search performs fuzzy search on the symbol index
func (idx *SymbolIndex) Search(query string, opts *SearchOptions) []SearchResult {
	matches := fuzzy.FindFrom(query, idx)

	// Apply filters and limit
	results := make([]SearchResult, 0, len(matches))
	for _, m := range matches {
		sym := &idx.symbols[m.Index]

		// Filter by kind
		if opts != nil && !slices.Contains(opts.Kinds, sym.Kind) {
			continue
		}

		results = append(results, SearchResult{
			Symbol:         sym,
			Score:          m.Score,
			MatchedIndexes: m.MatchedIndexes,
		})

		// Apply limit
		if opts != nil && opts.Limit > 0 && len(results) >= opts.Limit {
			break
		}
	}

	return results
}

// CollectSymbols builds a SymbolIndex from loaded packages
func CollectSymbols(pkgs []*packages.Package) *SymbolIndex {
	idx := &SymbolIndex{}

	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}

		for _, f := range pkg.Syntax {
			filename := pkg.Fset.Position(f.Pos()).Filename

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

func (idx *SymbolIndex) addFunc(pkg *packages.Package, filename string, d *ast.FuncDecl) {
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
		PkgPath:  pkg.PkgPath,
		fset:     pkg.Fset,
		filename: filename,
		pos:      d.Pos(),
		node:     d,
	})
}

func (idx *SymbolIndex) addGenDecl(pkg *packages.Package, filename string, d *ast.GenDecl) {
	for _, spec := range d.Specs {
		switch sp := spec.(type) {
		case *ast.TypeSpec:
			idx.addTypeSpec(pkg, filename, d.Tok, sp)
		case *ast.ValueSpec:
			idx.addValueSpec(pkg, filename, d.Tok, sp)
		}
	}
}

func (idx *SymbolIndex) addTypeSpec(pkg *packages.Package, filename string, tok token.Token, sp *ast.TypeSpec) {
	name := sp.Name.Name
	kind := SymbolKindType

	if _, ok := sp.Type.(*ast.InterfaceType); ok {
		kind = SymbolKindInterface
	}

	idx.symbols = append(idx.symbols, Symbol{
		Name:     name,
		Kind:     kind,
		PkgPath:  pkg.PkgPath,
		fset:     pkg.Fset,
		filename: filename,
		pos:      sp.Pos(),
		node:     sp,
		tok:      tok,
	})
}

func (idx *SymbolIndex) addValueSpec(pkg *packages.Package, filename string, tok token.Token, sp *ast.ValueSpec) {
	kind := SymbolKindVar
	if tok == token.CONST {
		kind = SymbolKindConst
	}

	// ValueSpec can have multiple names (e.g., var a, b, c int)
	for _, ident := range sp.Names {
		idx.symbols = append(idx.symbols, Symbol{
			Name:     ident.Name,
			Kind:     kind,
			PkgPath:  pkg.PkgPath,
			fset:     pkg.Fset,
			filename: filename,
			pos:      ident.Pos(),
			node:     sp,
			tok:      tok,
		})
	}
}
