package golang

import (
	"context"
	"strings"
	"testing"
)

func TestIsInterfaceMethod(t *testing.T) {
	project, err := LoadModulePackages(context.Background(), "../..", nil)
	if err != nil {
		t.Fatalf("LoadModulePackages: %v", err)
	}

	idx := CollectSymbols(project.Packages)

	// Find README methods and check if they're interface methods
	// Note: method names include receiver type, e.g. "PackageCommand.README"
	var found bool
	for _, sym := range idx.Symbols() {
		if strings.HasSuffix(sym.Name, ".README") && sym.Kind == SymbolKindMethod {
			isIface := IsInterfaceMethod(&sym, project, nil)
			t.Logf("%s (pkg=%s): IsInterfaceMethod=%v", sym.Name, sym.Package.Identifier.Name, isIface)
			found = true

			// All README methods should be interface methods (implement Command[T])
			if !isIface {
				t.Errorf("%s.README should be an interface method", sym.Package.Identifier.Name)
			}

			// Also test type lookup
			typeName := strings.TrimSuffix(sym.Name, ".README")

			// Try different lookup formats
			lookups := []string{
				sym.Package.Identifier.Name + "." + typeName,
				typeName,
			}
			var typeSym *Symbol
			var foundKey string
			for _, key := range lookups {
				matches := idx.Lookup(key)
				if len(matches) == 1 {
					typeSym = matches[0]
					foundKey = key
					break
				}
			}

			if typeSym == nil {
				t.Errorf("Type symbol not found for %s, tried: %v", typeName, lookups)
			} else {
				refs := CountReferences(project.Packages, typeSym).Total()
				t.Logf("  Type %s (found via %q) has %d refs", typeName, foundKey, refs)
			}
		}
	}

	if !found {
		t.Error("No README methods found")
	}
}
