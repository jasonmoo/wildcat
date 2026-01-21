package golang

import (
	"context"
	"testing"
)

func TestAnalyzeDeadCode(t *testing.T) {
	project, err := LoadModulePackages(context.Background(), "../..", nil)
	if err != nil {
		t.Fatalf("LoadModulePackages: %v", err)
	}

	result, err := AnalyzeDeadCode(project, true)
	if err != nil {
		t.Fatalf("AnalyzeDeadCode: %v", err)
	}

	// Should have found many reachable functions
	if len(result.Reachable) < 1000 {
		t.Errorf("Expected >1000 reachable functions, got %d", len(result.Reachable))
	}

	// main should be reachable
	var foundMain bool
	for fn := range result.Reachable {
		if fn.Name() == "main" && fn.Pkg.Pkg.Path() == "github.com/jasonmoo/wildcat" {
			foundMain = true
			break
		}
	}
	if !foundMain {
		t.Error("main function should be reachable")
	}

	// Test IsReachable with Execute function (called from main)
	idx := CollectSymbols(project.Packages)
	sym := idx.Lookup("Execute")
	if sym == nil {
		t.Fatal("Execute symbol not found")
	}
	if !result.IsReachable(sym) {
		t.Error("Execute should be reachable from main")
	}
}

func TestAnalyzeDeadCode_UnreachableCode(t *testing.T) {
	project, err := LoadModulePackages(context.Background(), "../..", nil)
	if err != nil {
		t.Fatalf("LoadModulePackages: %v", err)
	}

	result, err := AnalyzeDeadCode(project, true)
	if err != nil {
		t.Fatalf("AnalyzeDeadCode: %v", err)
	}

	// lsp.Client should NOT be reachable (it's only used by dead code)
	idx := CollectSymbols(project.Packages)
	for _, sym := range idx.Symbols() {
		if sym.Name == "Client" && sym.Package.Identifier.Name == "lsp" {
			if result.IsReachable(&sym) {
				t.Error("lsp.Client should not be reachable (only referenced by dead code)")
			}
			return
		}
	}
	t.Error("lsp.Client symbol not found in index")
}
