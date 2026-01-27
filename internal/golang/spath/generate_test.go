package spath

import (
	"context"
	"strings"
	"testing"

	"github.com/jasonmoo/wildcat/internal/golang"
)

func TestGenerate(t *testing.T) {
	ctx := context.Background()

	proj, err := golang.LoadModulePackages(ctx, ".", nil)
	if err != nil {
		t.Fatalf("failed to load packages: %v", err)
	}

	var golangPkg *golang.Package
	for _, pkg := range proj.Packages {
		if strings.HasSuffix(pkg.Identifier.PkgPath, "internal/golang") &&
			!strings.Contains(pkg.Identifier.PkgPath, "spath") {
			golangPkg = pkg
			break
		}
	}
	if golangPkg == nil {
		t.Fatal("could not find internal/golang package")
	}

	// Find Symbol type
	var symbolSym *golang.Symbol
	for _, sym := range golangPkg.Symbols {
		if sym.Name == "Symbol" {
			symbolSym = sym
			break
		}
	}
	if symbolSym == nil {
		t.Fatal("could not find Symbol type")
	}

	// Test Generate for type
	got := Generate(symbolSym)
	if !strings.HasSuffix(got, "internal/golang.Symbol") {
		t.Errorf("Generate(Symbol) = %q, want suffix 'internal/golang.Symbol'", got)
	}

	// Test Generate for method
	if len(symbolSym.Methods) > 0 {
		method := symbolSym.Methods[0]
		got := Generate(method)
		// Should be like "github.com/.../internal/golang.Symbol.MethodName"
		if !strings.Contains(got, "internal/golang.Symbol.") {
			t.Errorf("Generate(method) = %q, want to contain 'internal/golang.Symbol.'", got)
		}
	}

	// Test GenerateField
	got = GenerateField(symbolSym, "Name")
	if !strings.HasSuffix(got, "internal/golang.Symbol/fields[Name]") {
		t.Errorf("GenerateField = %q, want suffix 'internal/golang.Symbol/fields[Name]'", got)
	}

	// Find a function for param/return tests
	var funcSym *golang.Symbol
	for _, sym := range golangPkg.Symbols {
		if sym.Name == "LoadModulePackages" {
			funcSym = sym
			break
		}
	}
	if funcSym == nil {
		t.Fatal("could not find LoadModulePackages function")
	}

	// Test GenerateParam
	got = GenerateParam(funcSym, "ctx")
	if !strings.HasSuffix(got, "internal/golang.LoadModulePackages/params[ctx]") {
		t.Errorf("GenerateParam = %q, want suffix ending in '/params[ctx]'", got)
	}

	// Test GenerateReturn
	got = GenerateReturn(funcSym, 0)
	if !strings.HasSuffix(got, "internal/golang.LoadModulePackages/returns[0]") {
		t.Errorf("GenerateReturn = %q, want suffix ending in '/returns[0]'", got)
	}

	// Test GenerateBody
	got = GenerateBody(funcSym)
	if !strings.HasSuffix(got, "internal/golang.LoadModulePackages/body") {
		t.Errorf("GenerateBody = %q, want suffix ending in '/body'", got)
	}

	// Test GenerateDoc
	got = GenerateDoc(funcSym)
	if !strings.HasSuffix(got, "internal/golang.LoadModulePackages/doc") {
		t.Errorf("GenerateDoc = %q, want suffix ending in '/doc'", got)
	}
}

func TestGenerateRoundtrip(t *testing.T) {
	ctx := context.Background()

	proj, err := golang.LoadModulePackages(ctx, ".", nil)
	if err != nil {
		t.Fatalf("failed to load packages: %v", err)
	}

	var golangPkg *golang.Package
	for _, pkg := range proj.Packages {
		if strings.HasSuffix(pkg.Identifier.PkgPath, "internal/golang") &&
			!strings.Contains(pkg.Identifier.PkgPath, "spath") {
			golangPkg = pkg
			break
		}
	}
	if golangPkg == nil {
		t.Fatal("could not find internal/golang package")
	}

	// Test round-trip: Generate -> Parse -> Resolve -> same symbol
	for _, sym := range golangPkg.Symbols {
		pathStr := Generate(sym)

		path, err := Parse(pathStr)
		if err != nil {
			t.Errorf("Parse(Generate(%s)) error: %v", sym.Name, err)
			continue
		}

		res, err := Resolve(proj, path)
		if err != nil {
			t.Errorf("Resolve(Parse(Generate(%s))) error: %v", sym.Name, err)
			continue
		}

		// Should resolve to the same symbol
		if res.Symbol.Name != sym.Name {
			t.Errorf("Round-trip failed for %s: got %s", sym.Name, res.Symbol.Name)
		}

		// Test methods too
		for _, method := range sym.Methods {
			pathStr := Generate(method)

			path, err := Parse(pathStr)
			if err != nil {
				t.Errorf("Parse(Generate(%s.%s)) error: %v", sym.Name, method.Name, err)
				continue
			}

			res, err := Resolve(proj, path)
			if err != nil {
				t.Errorf("Resolve(Parse(Generate(%s.%s))) error: %v", sym.Name, method.Name, err)
				continue
			}

			if res.Symbol.Name != method.Name {
				t.Errorf("Round-trip failed for %s.%s: got %s", sym.Name, method.Name, res.Symbol.Name)
			}
		}
	}
}
