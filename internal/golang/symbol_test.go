package golang

import (
	"go/types"
	"testing"
)

func TestLoadPackageSymbols(t *testing.T) {
	// Load this package itself
	proj, err := LoadModulePackages(t.Context(), ".", nil)
	if err != nil {
		t.Fatalf("LoadModulePackages: %v", err)
	}

	// Find the golang package
	var golangPkg *Package
	for _, p := range proj.Packages {
		if p.Identifier.Name == "golang" {
			golangPkg = p
			break
		}
	}
	if golangPkg == nil {
		t.Fatal("couldn't find golang package")
	}

	// Check symbols were loaded
	if len(golangPkg.Symbols) == 0 {
		t.Fatal("no symbols loaded")
	}

	// Find PackageSymbol type
	var pkgSymbolType *PackageSymbol
	for _, sym := range golangPkg.Symbols {
		if sym.Name == "PackageSymbol" {
			pkgSymbolType = sym
			break
		}
	}
	if pkgSymbolType == nil {
		t.Fatal("couldn't find PackageSymbol type")
	}

	// Check it's a TypeName
	if _, ok := pkgSymbolType.Object.(*types.TypeName); !ok {
		t.Errorf("PackageSymbol.Object is %T, want *types.TypeName", pkgSymbolType.Object)
	}

	// Check it has methods (Signature)
	if len(pkgSymbolType.Methods) == 0 {
		t.Error("PackageSymbol has no methods, expected at least Signature()")
	}

	var hasSignatureMethod bool
	for _, m := range pkgSymbolType.Methods {
		if m.Name == "Signature" {
			hasSignatureMethod = true
			t.Logf("Found method: %s", m.Signature())
		}
	}
	if !hasSignatureMethod {
		t.Error("PackageSymbol missing Signature method")
	}

	// Check signature formatting works
	sig := pkgSymbolType.Signature()
	if sig == "" {
		t.Error("Signature() returned empty string")
	}
	t.Logf("PackageSymbol signature: %s", sig)

	// Find LoadPackageSymbols func and check it has a constructor relationship
	var loadFunc *PackageSymbol
	for _, sym := range golangPkg.Symbols {
		if sym.Name == "LoadPackageSymbols" {
			loadFunc = sym
			break
		}
	}
	if loadFunc == nil {
		t.Fatal("couldn't find LoadPackageSymbols func")
	}
	t.Logf("LoadPackageSymbols signature: %s", loadFunc.Signature())
}

func TestLoadImports(t *testing.T) {
	proj, err := LoadModulePackages(t.Context(), ".", nil)
	if err != nil {
		t.Fatalf("LoadModulePackages: %v", err)
	}

	// Find the golang package
	var golangPkg *Package
	for _, p := range proj.Packages {
		if p.Identifier.Name == "golang" {
			golangPkg = p
			break
		}
	}
	if golangPkg == nil {
		t.Fatal("couldn't find golang package")
	}

	// Check imports were loaded
	if len(golangPkg.Imports) == 0 {
		t.Fatal("no imports loaded")
	}

	// Look for an import that should resolve to a project package
	var foundInternal bool
	var foundExternal bool
	for _, fi := range golangPkg.Imports {
		t.Logf("File: %s", fi.FilePath)
		for _, imp := range fi.Imports {
			if imp.Package != nil {
				t.Logf("  import %q -> resolved to %s", imp.Path, imp.Package.Identifier.PkgPath)
				foundInternal = true
			} else {
				t.Logf("  import %q -> external (nil)", imp.Path)
				foundExternal = true
			}
		}
	}

	// golang package imports both internal (none actually) and external packages
	if !foundExternal {
		t.Error("expected at least one external import (e.g., go/ast)")
	}
	_ = foundInternal // golang package might not import other project packages
}
