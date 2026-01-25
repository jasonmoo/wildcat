package golang

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"

	"golang.org/x/tools/go/packages"
)

type PackageSymbol struct {
	Name         string
	Object       types.Object
	Package      *packages.Package
	File         *ast.File
	Node         ast.Node         // FuncDecl or synthetic GenDecl wrapping a single spec
	Methods      []*PackageSymbol // for types only
	Constructors []*PackageSymbol // for types only (funcs returning this type)

	// Interface relationships (only for types)
	Satisfies     []*PackageSymbol // interfaces this type implements
	ImplementedBy []*PackageSymbol // types implementing this interface (for interfaces only)
	Consumers     []*PackageSymbol // functions/methods accepting this interface as param (for interfaces only)
}

func (ps *PackageSymbol) Signature() string {
	return FormatNode(ps.Node)
}

func (ps *PackageSymbol) FileLocation() string {
	pos := ps.Package.Fset.Position(ps.Object.Pos())
	return fmt.Sprintf("%s:%d", filepath.Base(pos.Filename), pos.Line)
}

func (ps *PackageSymbol) PathLocation() string {
	pos := ps.Package.Fset.Position(ps.Object.Pos())
	return fmt.Sprintf("%s:%d", pos.Filename, pos.Line)
}

func (ps *PackageSymbol) FileDefinition() string {
	start := ps.Package.Fset.Position(ps.Node.Pos())
	end := ps.Package.Fset.Position(ps.Node.End())
	return fmt.Sprintf("%s:%d:%d", filepath.Base(start.Filename), start.Line, end.Line)
}

func (ps *PackageSymbol) PathDefinition() string {
	start := ps.Package.Fset.Position(ps.Node.Pos())
	end := ps.Package.Fset.Position(ps.Node.End())
	return fmt.Sprintf("%s:%d:%d", start.Filename, start.Line, end.Line)
}

func loadPackageSymbols(pkg *packages.Package) []*PackageSymbol {

	ss := make(map[string]*PackageSymbol)

	// First pass: create all symbols, collect methods for types
	for _, name := range pkg.Types.Scope().Names() {
		if name == "_" {
			continue // skip blank identifier
		}
		obj := pkg.Types.Scope().Lookup(name)
		file, node := findNode(pkg, obj.Pos())
		sym := &PackageSymbol{
			Name:    name,
			Object:  obj,
			Package: pkg,
			File:    file,
			Node:    node,
		}
		if tn, ok := sym.Object.(*types.TypeName); ok {
			if named, ok := tn.Type().(*types.Named); ok {
				for m := range named.Methods() {
					mFile, mNode := findNode(pkg, m.Pos())
					sym.Methods = append(sym.Methods, &PackageSymbol{
						Name:    m.Name(),
						Object:  m,
						Package: pkg,
						File:    mFile,
						Node:    mNode,
					})
				}
			}
		}
		ss[name] = sym
	}

	for _, sym := range ss {
		if fd, ok := sym.Node.(*ast.FuncDecl); ok {
			if typeName := ConstructorTypeName(fd.Type); typeName != "" {
				if typeSym := ss[typeName]; typeSym != nil {
					typeSym.Constructors = append(typeSym.Constructors, sym)
				}
			}
		}
	}

	var ret []*PackageSymbol

	for _, name := range pkg.Types.Scope().Names() {
		if name == "_" {
			continue
		}
		ret = append(ret, ss[name])
	}

	return ret
}

// findNode locates the AST node for a given position.
// For specs (TypeSpec, ValueSpec), returns a synthetic GenDecl wrapping just that spec
// so the node is directly formattable with FormatNode.
func findNode(pkg *packages.Package, pos token.Pos) (*ast.File, ast.Node) {
	for _, f := range pkg.Syntax {
		if pkg.Fset.File(f.Pos()).Name() != pkg.Fset.File(pos).Name() {
			continue
		}
		for _, decl := range f.Decls {
			switch v := decl.(type) {
			case *ast.FuncDecl:
				if v.Name.Pos() == pos {
					return f, v
				}
			case *ast.GenDecl:
				for _, spec := range v.Specs {
					switch vv := spec.(type) {
					case *ast.TypeSpec:
						if vv.Name.Pos() == pos {
							// Preserve position from the spec for FileDefinition
							return f, &ast.GenDecl{TokPos: vv.Pos(), Tok: v.Tok, Specs: []ast.Spec{vv}}
						}
					case *ast.ValueSpec:
						for _, ident := range vv.Names {
							if ident.Pos() == pos {
								return f, &ast.GenDecl{TokPos: vv.Pos(), Tok: v.Tok, Specs: []ast.Spec{vv}}
							}
						}
					}
				}
			}
		}
	}
	return nil, nil
}
