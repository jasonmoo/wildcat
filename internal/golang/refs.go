package golang

import (
	"go/ast"
	"go/token"
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
	case *ast.GenDecl:
		// Symbol wraps TypeSpec/ValueSpec in synthetic GenDecl for formatting
		if len(n.Specs) > 0 {
			switch spec := n.Specs[0].(type) {
			case *ast.TypeSpec:
				return sym.Package.Package.TypesInfo.Defs[spec.Name]
			case *ast.ValueSpec:
				for _, name := range spec.Names {
					if name.Name == sym.Name {
						return sym.Package.Package.TypesInfo.Defs[name]
					}
				}
			}
		}
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

// GetInterfaceType extracts the types.Interface from an interface symbol.
// Returns nil if the symbol is not an interface.
func GetInterfaceType(sym *Symbol) *types.Interface {
	if sym.Kind != SymbolKindInterface {
		return nil
	}

	// Handle both direct TypeSpec and GenDecl wrapper
	node := sym.Node()
	var typeSpec *ast.TypeSpec
	if ts, ok := node.(*ast.TypeSpec); ok {
		typeSpec = ts
	} else if gd, ok := node.(*ast.GenDecl); ok && len(gd.Specs) > 0 {
		typeSpec, _ = gd.Specs[0].(*ast.TypeSpec)
	}
	if typeSpec == nil {
		return nil
	}

	// Verify it's an interface type in the AST
	if _, ok := typeSpec.Type.(*ast.InterfaceType); !ok {
		return nil
	}

	// Get the types.Interface from type info
	obj := sym.Package.Package.TypesInfo.Defs[typeSpec.Name]
	if obj == nil {
		return nil
	}
	named, ok := obj.Type().(*types.Named)
	if !ok {
		return nil
	}
	iface, ok := named.Underlying().(*types.Interface)
	if !ok {
		return nil
	}
	return iface
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

// WalkNonCallReferences walks references to a symbol that are NOT direct calls.
// These are "escaping" references where the function value is passed, assigned,
// or stored - indicating it could be called from external code.
//
// Examples of non-call references:
//   - Passed as argument: http.HandleFunc("/", handler)
//   - Assigned to variable: fn := myFunc
//   - Struct field: &Command{Run: handler}
//   - Stored in map/slice: handlers["x"] = myFunc
//   - Returned: return myFunc
//
// Examples of call references (excluded):
//   - Direct call: foo()
//   - Method call: x.Method()
//   - Qualified call: pkg.Func()
func WalkNonCallReferences(pkgs []*Package, sym *Symbol, visitor RefVisitor) {
	targetObj := GetTypesObject(sym)
	if targetObj == nil {
		return
	}

	for _, pkg := range pkgs {
		if !walkPackageNonCallRefs(pkg, targetObj, visitor) {
			return
		}
	}
}

// walkPackageNonCallRefs walks non-call references in a single package.
func walkPackageNonCallRefs(pkg *Package, targetObj types.Object, visitor RefVisitor) bool {
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

				if !walkNodeNonCallRefs(d, pkg, filename, containing, targetObj, visitor) {
					return false
				}

			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						containing = pkg.Identifier.Name + "." + s.Name.Name
						if !walkNodeNonCallRefs(s.Type, pkg, filename, containing, targetObj, visitor) {
							return false
						}
					case *ast.ValueSpec:
						if s.Type != nil && len(s.Names) > 0 {
							containing = pkg.Identifier.Name + "." + s.Names[0].Name
							if !walkNodeNonCallRefs(s.Type, pkg, filename, containing, targetObj, visitor) {
								return false
							}
						}
						for i, name := range s.Names {
							if i < len(s.Values) {
								containing = pkg.Identifier.Name + "." + name.Name
								if !walkNodeNonCallRefs(s.Values[i], pkg, filename, containing, targetObj, visitor) {
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

// walkNodeNonCallRefs walks an AST node looking for non-call references.
// It first collects all positions where identifiers are used as the function
// in a call expression, then walks and skips those positions.
func walkNodeNonCallRefs(node ast.Node, pkg *Package, filename, containing string, targetObj types.Object, visitor RefVisitor) bool {
	// First pass: collect positions of identifiers in call position
	callPositions := collectCallPositions(node)

	// Second pass: find references that are not in call position
	continueWalk := true

	ast.Inspect(node, func(n ast.Node) bool {
		if !continueWalk {
			return false
		}

		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}

		// Skip if this identifier is in call position
		if callPositions[ident.Pos()] {
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

// collectCallPositions returns the positions of all identifiers that are
// used as the function being called in a CallExpr.
func collectCallPositions(node ast.Node) map[token.Pos]bool {
	positions := make(map[token.Pos]bool)

	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Get the position of the identifier being called
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			// Direct call: foo()
			positions[fn.Pos()] = true
		case *ast.SelectorExpr:
			// Method/qualified call: x.Method() or pkg.Func()
			positions[fn.Sel.Pos()] = true
		case *ast.IndexExpr:
			// Generic call: foo[T]()
			if ident, ok := fn.X.(*ast.Ident); ok {
				positions[ident.Pos()] = true
			}
		case *ast.IndexListExpr:
			// Generic call with multiple params: foo[T, U]()
			if ident, ok := fn.X.(*ast.Ident); ok {
				positions[ident.Pos()] = true
			}
		}
		return true
	})

	return positions
}

// CountNonCallReferences counts references to a symbol that are not direct calls.
// This is useful for dead code analysis: a function with non-call references
// may be called from external code (e.g., passed to cobra, http handlers).
func CountNonCallReferences(pkgs []*Package, sym *Symbol) int {
	count := 0
	WalkNonCallReferences(pkgs, sym, func(ref Reference) bool {
		count++
		return true
	})
	return count
}

// ComputeDescendants populates Descendants on all struct PackageSymbols.
// A direct descendant is a type that is only referenced by the parent type (and its methods).
// If the parent type were removed, the descendant would be orphaned.
// Note: This only computes direct descendants, not transitive ones.
// This should be called after loading packages.
func ComputeDescendants(project []*Package) {
	// Build a map of types by their Object for quick lookup
	typeByObj := make(map[types.Object]*PackageSymbol)
	for _, pkg := range project {
		for _, sym := range pkg.Symbols {
			if _, ok := sym.Object.(*types.TypeName); ok {
				typeByObj[sym.Object] = sym
			}
		}
	}

	// Build referrers map: for each type, which symbols reference it
	// key: type's Object, value: set of referrer symbol names (pkg.Name or pkg.Type.Method)
	referrers := make(map[types.Object]map[string]bool)

	for _, pkg := range project {
		for _, file := range pkg.Package.Syntax {
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					symbolName := pkg.Identifier.Name + "."
					if d.Recv != nil && len(d.Recv.List) > 0 {
						symbolName += ReceiverTypeName(d.Recv.List[0].Type) + "."
					}
					symbolName += d.Name.Name
					collectTypeReferrers(pkg, d, symbolName, referrers)

				case *ast.GenDecl:
					for _, spec := range d.Specs {
						switch s := spec.(type) {
						case *ast.TypeSpec:
							symbolName := pkg.Identifier.Name + "." + s.Name.Name
							collectTypeReferrers(pkg, s.Type, symbolName, referrers)
						case *ast.ValueSpec:
							for _, name := range s.Names {
								symbolName := pkg.Identifier.Name + "." + name.Name
								collectTypeReferrers(pkg, s, symbolName, referrers)
							}
						}
					}
				}
			}
		}
	}

	// For each struct type, find descendants
	for _, pkg := range project {
		for _, sym := range pkg.Symbols {
			tn, ok := sym.Object.(*types.TypeName)
			if !ok {
				continue
			}
			// Only struct types can have descendants (not interfaces, aliases, etc.)
			if _, isStruct := tn.Type().Underlying().(*types.Struct); !isStruct {
				continue
			}

			// Build the scope: this type + its methods
			scope := make(map[string]bool)
			scope[pkg.Identifier.Name+"."+sym.Name] = true
			for _, m := range sym.Methods {
				scope[pkg.Identifier.Name+"."+sym.Name+"."+m.Name] = true
			}

			// Find types referenced in this type's definition
			referencedTypes := findReferencedTypesInNode(pkg, sym.Node, typeByObj)

			// Check each referenced type
			for _, refType := range referencedTypes {
				refs := referrers[refType.Object]
				if len(refs) == 0 {
					continue
				}

				// Check if all referrers are within our scope
				allInScope := true
				for referrer := range refs {
					if !scope[referrer] {
						allInScope = false
						break
					}
				}

				if allInScope {
					sym.Descendants = append(sym.Descendants, refType)
				}
			}
		}
	}
}

// findReferencedTypesInNode extracts all project types referenced in an AST node.
func findReferencedTypesInNode(pkg *Package, node ast.Node, typeByObj map[types.Object]*PackageSymbol) []*PackageSymbol {
	var result []*PackageSymbol
	seen := make(map[types.Object]bool)

	ast.Inspect(node, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}

		obj := pkg.Package.TypesInfo.Uses[ident]
		if obj == nil {
			return true
		}

		// Only interested in type names
		if _, ok := obj.(*types.TypeName); !ok {
			return true
		}

		// Skip if already seen or not in project
		if seen[obj] {
			return true
		}
		seen[obj] = true

		if typeSym := typeByObj[obj]; typeSym != nil {
			result = append(result, typeSym)
		}

		return true
	})

	return result
}

// collectTypeReferrers adds type references from a node to the referrers map.
func collectTypeReferrers(pkg *Package, node ast.Node, symbolName string, referrers map[types.Object]map[string]bool) {
	ast.Inspect(node, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}

		obj := pkg.Package.TypesInfo.Uses[ident]
		if obj == nil {
			return true
		}

		// Only interested in type names
		if _, ok := obj.(*types.TypeName); !ok {
			return true
		}

		// Skip builtin types
		if obj.Pkg() == nil {
			return true
		}

		if referrers[obj] == nil {
			referrers[obj] = make(map[string]bool)
		}
		referrers[obj][symbolName] = true

		return true
	})
}
