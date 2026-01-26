package golang

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// BuiltinPackage creates a synthetic Package for Go's builtin interfaces (error, comparable).
// These exist in types.Universe but have no real package, so we create one for consistent Symbol handling.
func BuiltinPackage() *Package {
	fset := token.NewFileSet()

	// Create a minimal packages.Package for the builtin pseudo-package
	pkg := &packages.Package{
		ID:      "builtin",
		Name:    "builtin",
		PkgPath: "builtin",
		Fset:    fset,
		Types:   types.NewPackage("builtin", "builtin"),
	}

	ident := &PackageIdentifier{
		Name:    "builtin",
		PkgPath: "builtin",
		IsStd:   true,
	}

	var symbols []*Symbol

	// Create Symbol for 'error' interface
	errorObj := types.Universe.Lookup("error")
	if errorObj != nil {
		symbols = append(symbols, &Symbol{
			Kind:              SymbolKindInterface,
			Name:              "error",
			Object:            errorObj,
			Package:           pkg,
			PackageIdentifier: ident,
			IsBuiltin:         true,
		})
	}

	// Create Symbol for 'comparable' interface
	comparableObj := types.Universe.Lookup("comparable")
	if comparableObj != nil {
		symbols = append(symbols, &Symbol{
			Kind:              SymbolKindInterface,
			Name:              "comparable",
			Object:            comparableObj,
			Package:           pkg,
			PackageIdentifier: ident,
			IsBuiltin:         true,
		})
	}

	p := &Package{
		Identifier: ident,
		Package:    pkg,
		Symbols:    symbols,
	}
	p.buildSymbolIndex()

	return p
}

// BuiltinErrorSymbol returns the error Symbol from a builtin Package.
func BuiltinErrorSymbol(builtin *Package) *Symbol {
	for _, sym := range builtin.Symbols {
		if sym.Name == "error" {
			return sym
		}
	}
	return nil
}

// BuiltinComparableSymbol returns the comparable Symbol from a builtin Package.
func BuiltinComparableSymbol(builtin *Package) *Symbol {
	for _, sym := range builtin.Symbols {
		if sym.Name == "comparable" {
			return sym
		}
	}
	return nil
}
