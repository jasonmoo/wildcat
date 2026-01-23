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

	result, err := AnalyzeDeadCode(project)
	if err != nil {
		t.Fatalf("AnalyzeDeadCode: %v", err)
	}

	// Should have entry points (wildcat has main)
	if !result.HasEntryPoints {
		t.Error("Expected HasEntryPoints=true for wildcat (has main)")
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

	// Test IsReachable with NewSymbolCommand function (called from main)
	idx := CollectSymbols(project.Packages)
	sym := idx.Lookup("NewSymbolCommand")
	if sym == nil {
		t.Fatal("NewSymbolCommand symbol not found")
	}
	reachable, analyzed := result.IsReachable(sym)
	if !analyzed {
		t.Error("NewSymbolCommand should be analyzable")
	}
	if !reachable {
		t.Error("NewSymbolCommand should be reachable from main")
	}
}

func TestAnalyzeDeadCode_UnreachableCode(t *testing.T) {
	// TODO(wc-4967): SSA analysis has false positives for cobra patterns,
	// flag bindings, and interface types. Skip until deadcode detection
	// is more accurate.
	t.Skip("deadcode detection needs improvement - see wc-4967")
}
