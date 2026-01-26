package golang

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"

	"golang.org/x/tools/go/packages"
)

type Symbol struct {
	Kind              SymbolKind
	Name              string
	Object            types.Object
	Package           *packages.Package
	PackageIdentifier *PackageIdentifier // package metadata for search
	File              *ast.File
	Node              ast.Node  // FuncDecl or synthetic GenDecl wrapping a single spec
	Methods           []*Symbol // for types only
	Constructors      []*Symbol // for types only (funcs returning this type)
	Parent            *Symbol   // for methods: receiver type; for constructors: constructed type

	// Interface relationships (only for types)
	Satisfies     []*Symbol // interfaces this type implements
	ImplementedBy []*Symbol // types implementing this interface (for interfaces only)
	Consumers     []*Symbol // functions/methods accepting this interface as param (for interfaces only)

	// Dependency relationships (only for struct types)
	Descendants []*Symbol // direct descendants: types only referenced by this type (would be orphaned if removed)
}

func (ps *Symbol) Signature() string {
	return FormatNode(ps.Node)
}

func (ps *Symbol) FileLocation() string {
	pos := ps.Package.Fset.Position(ps.Object.Pos())
	return fmt.Sprintf("%s:%d", filepath.Base(pos.Filename), pos.Line)
}

func (ps *Symbol) PathLocation() string {
	pos := ps.Package.Fset.Position(ps.Object.Pos())
	return fmt.Sprintf("%s:%d", pos.Filename, pos.Line)
}

func (ps *Symbol) FileDefinition() string {
	start := ps.Package.Fset.Position(ps.Node.Pos())
	end := ps.Package.Fset.Position(ps.Node.End())
	return fmt.Sprintf("%s:%d:%d", filepath.Base(start.Filename), start.Line, end.Line)
}

func (ps *Symbol) PathDefinition() string {
	start := ps.Package.Fset.Position(ps.Node.Pos())
	end := ps.Package.Fset.Position(ps.Node.End())
	return fmt.Sprintf("%s:%d:%d", start.Filename, start.Line, end.Line)
}

// ReceiverTypeName returns the receiver type name for methods, or empty string for non-methods.
// Uses type info rather than AST inspection.
func (ps *Symbol) ReceiverTypeName() string {
	if ps.Kind != SymbolKindMethod {
		return ""
	}
	fn, ok := ps.Object.(*types.Func)
	if !ok {
		return ""
	}
	sig := fn.Signature()
	if sig.Recv() == nil {
		return ""
	}
	recvType := sig.Recv().Type()
	// Strip pointer if present
	if ptr, ok := recvType.(*types.Pointer); ok {
		recvType = ptr.Elem()
	}
	// Get the type name
	if named, ok := recvType.(*types.Named); ok {
		return named.Obj().Name()
	}
	return ""
}

// ConstructedTypeName returns the type name this function constructs, or empty string.
// A constructor is a function returning a single named type (or pointer to it) from the same package.
func (ps *Symbol) ConstructedTypeName() string {
	if ps.Kind != SymbolKindFunc {
		return ""
	}
	node, ok := ps.Node.(*ast.FuncDecl)
	if !ok {
		return ""
	}
	return ConstructorTypeName(node.Type)
}

// ParentTypeName returns the associated type name for methods (receiver) or constructors (return type).
// Returns empty string for non-methods and non-constructors.
func (ps *Symbol) ParentTypeName() string {
	if name := ps.ReceiverTypeName(); name != "" {
		return name
	}
	return ps.ConstructedTypeName()
}

// PkgSymbol returns "pkgName.symbolName" (e.g., "golang.Symbol").
// For human-readable display.
func (ps *Symbol) PkgSymbol() string {
	if ps.PackageIdentifier != nil {
		return ps.PackageIdentifier.Name + "." + ps.Name
	}
	return "<pkg-name-error>." + ps.Name
}

// PkgPathSymbol returns "pkgPath.symbolName" (e.g., "github.com/.../golang.Symbol").
// For globally unique identification.
func (ps *Symbol) PkgPathSymbol() string {
	if ps.PackageIdentifier != nil {
		return ps.PackageIdentifier.PkgPath + "." + ps.Name
	}
	return "<pkg-name-error>." + ps.Name
}

// PkgTypeSymbol returns the name with receiver type for methods.
// For methods: "pkgName.ReceiverType.methodName" (e.g., "golang.Symbol.Signature")
// For non-methods: same as PkgSymbol() (e.g., "golang.LoadSymbols")
func (ps *Symbol) PkgTypeSymbol() string {
	if ps.PackageIdentifier != nil {
		if recv := ps.ReceiverTypeName(); recv != "" {
			return ps.PackageIdentifier.Name + "." + recv + "." + ps.Name
		}
		return ps.PackageIdentifier.Name + "." + ps.Name
	}
	if recv := ps.ReceiverTypeName(); recv != "" {
		return "<pkg-name-error>." + recv + "." + ps.Name
	}
	return "<pkg-name-error>." + ps.Name
}

// PkgPathTypeSymbol returns full path with receiver type for methods.
// For methods: "pkgPath.ReceiverType.methodName"
// For non-methods: same as PkgPathSymbol()
func (ps *Symbol) PkgPathTypeSymbol() string {
	if ps.PackageIdentifier != nil {
		if recv := ps.ReceiverTypeName(); recv != "" {
			return ps.PackageIdentifier.PkgPath + "." + recv + "." + ps.Name
		}
		return ps.PackageIdentifier.PkgPath + "." + ps.Name
	}
	if recv := ps.ReceiverTypeName(); recv != "" {
		return "<pkg-name-error>." + recv + "." + ps.Name
	}
	return "<pkg-name-error>." + ps.Name
}

// SearchName returns the fully qualified name for search (PkgPath.Name).
// Deprecated: Use PkgPathSymbol() instead.
func (ps *Symbol) SearchName() string {
	return ps.PkgPathSymbol()
}

func loadSymbols(pkg *packages.Package) []*Symbol {

	ss := make(map[string]*Symbol)

	// First pass: create all symbols, collect methods for types
	for _, name := range pkg.Types.Scope().Names() {
		if name == "_" {
			continue // skip blank identifier
		}
		obj := pkg.Types.Scope().Lookup(name)
		file, node := findNode(pkg, obj.Pos())
		sym := &Symbol{
			Kind:    kindFromObject(obj),
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
					sym.Methods = append(sym.Methods, &Symbol{
						Kind:    SymbolKindMethod,
						Name:    m.Name(),
						Object:  m,
						Package: pkg,
						File:    mFile,
						Node:    mNode,
						Parent:  sym,
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
					sym.Parent = typeSym
				}
			}
		}
	}

	var ret []*Symbol

	for _, name := range pkg.Types.Scope().Names() {
		if name == "_" {
			continue
		}
		ret = append(ret, ss[name])
	}

	return ret
}

// setSymbolIdentifiers sets the PackageIdentifier on all symbols and their nested methods.
func setSymbolIdentifiers(symbols []*Symbol, ident *PackageIdentifier) {
	for _, sym := range symbols {
		sym.PackageIdentifier = ident
		for _, m := range sym.Methods {
			m.PackageIdentifier = ident
		}
	}
}

// kindFromObject determines the SymbolKind for a types.Object.
func kindFromObject(obj types.Object) SymbolKind {
	switch o := obj.(type) {
	case *types.Func:
		if o.Signature().Recv() != nil {
			return SymbolKindMethod
		}
		return SymbolKindFunc
	case *types.TypeName:
		if _, ok := o.Type().Underlying().(*types.Interface); ok {
			return SymbolKindInterface
		}
		return SymbolKindType
	case *types.Const:
		return SymbolKindConst
	case *types.Var:
		return SymbolKindVar
	case *types.PkgName:
		return SymbolKindPkgName
	case *types.Label:
		return SymbolKindLabel
	case *types.Builtin:
		return SymbolKindBuiltin
	case *types.Nil:
		return SymbolKindNil
	default:
		return SymbolKindUnknown
	}
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
