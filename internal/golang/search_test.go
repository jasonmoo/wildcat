package golang

import (
	"context"
	"testing"
)

func TestSymbolSearch(t *testing.T) {
	ctx := context.Background()

	// Load this project's packages
	project, err := LoadModulePackages(ctx, ".", nil)
	if err != nil {
		t.Fatalf("LoadModulePackages: %v", err)
	}

	// Build symbol index
	idx := CollectSymbols(project.Packages)

	t.Logf("Collected %d symbols", idx.Len())

	// Test searches with expected top results
	tests := []struct {
		query   string
		wantMin int    // minimum expected results
		wantTop string // expected top result name (empty = any)
	}{
		{"Symbol", 1, ""},       // Multiple symbols contain "Symbol" (SymbolKind, SymbolRefs, etc.)
		{"Format", 1, ""},
		{"Package", 1, "Package"},
		{"Resolve", 1, ""},
		{"Collect", 1, ""},
	}

	for _, tc := range tests {
		results := idx.Search(tc.query, nil)
		if len(results) < tc.wantMin {
			t.Errorf("Search(%q): got %d results, want at least %d", tc.query, len(results), tc.wantMin)
			continue
		}

		// Log top 5 results
		t.Logf("Search(%q): %d results", tc.query, len(results))
		for i, r := range results {
			if i >= 5 {
				break
			}
			// Lazy render signature and location only for results we display
			t.Logf("  %d. [score=%d] %s [%s] %s // %s",
				i+1, r.Score, r.Symbol.Name, r.Symbol.Kind, r.Symbol.PackageIdentifier.PkgPath, r.Symbol.FileLocation())
			t.Logf("      sig: %s", r.Symbol.Signature())
		}

		if tc.wantTop != "" && results[0].Symbol.Name != tc.wantTop {
			t.Errorf("Search(%q): top result = %q, want %q", tc.query, results[0].Symbol.Name, tc.wantTop)
		}
	}

	// Test options: limit
	limited := idx.Search("Format", &SearchOptions{Limit: 3})
	if len(limited) != 3 {
		t.Errorf("Search with Limit=3: got %d results, want 3", len(limited))
	}

	// Test options: filter by kind
	funcsOnly := idx.Search("Format", &SearchOptions{Kinds: []SymbolKind{SymbolKindFunc}})
	for _, r := range funcsOnly {
		if r.Symbol.Kind != SymbolKindFunc {
			t.Errorf("Search with Kinds=[func]: got kind %s, want func", r.Symbol.Kind)
		}
	}
	t.Logf("Search(\"Format\", funcs only): %d results", len(funcsOnly))

	// Test options: combined
	combined := idx.Search("Package", &SearchOptions{Limit: 5, Kinds: []SymbolKind{SymbolKindMethod}})
	if len(combined) > 5 {
		t.Errorf("Search with Limit=5: got %d results", len(combined))
	}
	for _, r := range combined {
		if r.Symbol.Kind != SymbolKindMethod {
			t.Errorf("Search with Kinds=[method]: got kind %s", r.Symbol.Kind)
		}
	}
	t.Logf("Search(\"Package\", methods, limit 5): %d results", len(combined))
}
