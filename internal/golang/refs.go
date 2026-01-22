package golang

import (
	"go/ast"
	"go/types"
)

// Reference represents a single reference to a symbol.
type Reference struct {
	Package    *Package  // package containing the reference
	File       string    // file path
	Line       int       // line number
	Ident      *ast.Ident // the identifier node
	Containing string    // containing symbol (e.g., "pkg.Func" or "pkg.Type.Method")
}

// IsInternal returns true if this reference is from the same package as the target.
func (r *Reference) IsInternal(targetPkgPath string) bool {
	return r.Package.Identifier.PkgPath == targetPkgPath
}

// RefVisitor is called for each reference found. Return false to stop walking.
type RefVisitor func(ref Reference) bool

// WalkReferences walks all references to a symbol in the given packages.
// Pass project.Packages for all packages, or a subset for filtering.
func WalkReferences(pkgs []*Package, sym *Symbol, visitor RefVisitor) {
	targetObj := GetTypesObject(sym)
	if targetObj == nil {
		return
	}

	for _, pkg := range pkgs {
		if !walkPackageRefs(pkg, targetObj, visitor) {
			return
		}
	}
}

// walkPackageRefs walks references in a single package. Returns false if visitor wants to stop.
func walkPackageRefs(pkg *Package, targetObj types.Object, visitor RefVisitor) bool {
	fset := pkg.Package.Fset

	for _, file := range pkg.Package.Syntax {
		filename := fset.Position(file.Pos()).Filename

		for _, decl := range file.Decls {
			containing := ""

			switch d := decl.(type) {
			case *ast.FuncDecl:
				containing = pkg.Identifier.Name + "."
				if d.Recv != nil && len(d.Recv.List) > 0 {
					containing += ReceiverTypeName(d.Recv.List[0].Type) + "."
				}
				containing += d.Name.Name

				if !walkNodeRefs(d, pkg, filename, containing, targetObj, visitor) {
					return false
				}

			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						containing = pkg.Identifier.Name + "." + s.Name.Name
						if !walkNodeRefs(s.Type, pkg, filename, containing, targetObj, visitor) {
							return false
						}
					case *ast.ValueSpec:
						if s.Type != nil && len(s.Names) > 0 {
							containing = pkg.Identifier.Name + "." + s.Names[0].Name
							if !walkNodeRefs(s.Type, pkg, filename, containing, targetObj, visitor) {
								return false
							}
						}
						for i, name := range s.Names {
							if i < len(s.Values) {
								containing = pkg.Identifier.Name + "." + name.Name
								if !walkNodeRefs(s.Values[i], pkg, filename, containing, targetObj, visitor) {
									return false
								}
							}
						}
					}
				}
			}
		}
	}
	return true
}

// walkNodeRefs walks an AST node looking for references. Returns false if visitor wants to stop.
func walkNodeRefs(node ast.Node, pkg *Package, filename, containing string, targetObj types.Object, visitor RefVisitor) bool {
	continueWalk := true

	ast.Inspect(node, func(n ast.Node) bool {
		if !continueWalk {
			return false
		}

		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}

		obj := pkg.Package.TypesInfo.Uses[ident]
		if obj == nil {
			return true
		}

		if SameObject(obj, targetObj) {
			pos := pkg.Package.Fset.Position(ident.Pos())
			ref := Reference{
				Package:    pkg,
				File:       filename,
				Line:       pos.Line,
				Ident:      ident,
				Containing: containing,
			}
			if !visitor(ref) {
				continueWalk = false
				return false
			}
		}
		return true
	})

	return continueWalk
}

// RefCounts holds reference statistics for a symbol.
type RefCounts struct {
	Internal int      // references from same package
	External int      // references from other packages
	Packages []string // unique external package paths that reference this symbol
}

// Total returns the total number of references.
func (r *RefCounts) Total() int {
	return r.Internal + r.External
}

// PackageCount returns the number of unique external packages referencing this symbol.
func (r *RefCounts) PackageCount() int {
	return len(r.Packages)
}

// CountReferences counts all references to a symbol in the given packages.
// Pass project.Packages for all packages, or a subset for filtering.
func CountReferences(pkgs []*Package, sym *Symbol) *RefCounts {
	counts := &RefCounts{}
	pkgSet := make(map[string]bool)
	targetPkgPath := sym.Package.Identifier.PkgPath

	WalkReferences(pkgs, sym, func(ref Reference) bool {
		if ref.IsInternal(targetPkgPath) {
			counts.Internal++
		} else {
			counts.External++
			pkgPath := ref.Package.Identifier.PkgPath
			if !pkgSet[pkgPath] {
				pkgSet[pkgPath] = true
				counts.Packages = append(counts.Packages, pkgPath)
			}
		}
		return true
	})

	return counts
}

// GetTypesObject returns the types.Object for a symbol.
func GetTypesObject(sym *Symbol) types.Object {
	node := sym.Node()

	switch n := node.(type) {
	case *ast.FuncDecl:
		return sym.Package.Package.TypesInfo.Defs[n.Name]
	case *ast.TypeSpec:
		return sym.Package.Package.TypesInfo.Defs[n.Name]
	case *ast.ValueSpec:
		for _, name := range n.Names {
			if name.Name == sym.Name {
				return sym.Package.Package.TypesInfo.Defs[name]
			}
		}
	case *ast.Field:
		for _, name := range n.Names {
			if name.Name == sym.Name {
				return sym.Package.Package.TypesInfo.Defs[name]
			}
		}
	}
	return nil
}

// SameObject checks if two types.Object refer to the same symbol.
func SameObject(obj, target types.Object) bool {
	if obj == target {
		return true
	}
	if obj.Pkg() == nil || target.Pkg() == nil {
		return false
	}
	return obj.Pkg().Path() == target.Pkg().Path() &&
		obj.Name() == target.Name() &&
		obj.Pos() == target.Pos()
}
