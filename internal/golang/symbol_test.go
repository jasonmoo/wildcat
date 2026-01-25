package golang

import (
	"go/types"
	"testing"
)

func TestLoadSymbols(t *testing.T) {
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

	// Find Symbol type
	var symbolType *Symbol
	for _, sym := range golangPkg.Symbols {
		if sym.Name == "Symbol" {
			symbolType = sym
			break
		}
	}
	if symbolType == nil {
		t.Fatal("couldn't find Symbol type")
	}

	// Check it's a TypeName
	if _, ok := symbolType.Object.(*types.TypeName); !ok {
		t.Errorf("Symbol.Object is %T, want *types.TypeName", symbolType.Object)
	}

	// Check it has methods (Signature)
	if len(symbolType.Methods) == 0 {
		t.Error("Symbol has no methods, expected at least Signature()")
	}

	var hasSignatureMethod bool
	for _, m := range symbolType.Methods {
		if m.Name == "Signature" {
			hasSignatureMethod = true
			t.Logf("Found method: %s", m.Signature())
		}
	}
	if !hasSignatureMethod {
		t.Error("Symbol missing Signature method")
	}

	// Check signature formatting works
	sig := symbolType.Signature()
	if sig == "" {
		t.Error("Signature() returned empty string")
	}
	t.Logf("Symbol signature: %s", sig)

	// Find LoadModulePackages func (an exported function)
	var loadFunc *Symbol
	for _, sym := range golangPkg.Symbols {
		if sym.Name == "LoadModulePackages" {
			loadFunc = sym
			break
		}
	}
	if loadFunc == nil {
		t.Fatal("couldn't find LoadModulePackages func")
	}
	t.Logf("LoadModulePackages signature: %s", loadFunc.Signature())
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
