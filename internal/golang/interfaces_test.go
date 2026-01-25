package golang

import (
	"context"
	"go/ast"
	"testing"
)

func TestIsInterfaceMethod(t *testing.T) {
	project, err := LoadModulePackages(context.Background(), "../..", nil)
	if err != nil {
		t.Fatalf("LoadModulePackages: %v", err)
	}

	idx := CollectSymbols(project.Packages)

	// Find README methods and check if they're interface methods
	// In the new structure, method names are just "README" (not "Type.README")
	var found bool
	for _, sym := range idx.Symbols() {
		if sym.Name == "README" && sym.Kind == SymbolKindMethod {
			isIface := IsInterfaceMethod(sym, project, nil)

			// Get receiver type name from AST node
			var typeName string
			if fd, ok := sym.Node.(*ast.FuncDecl); ok && fd.Recv != nil && len(fd.Recv.List) > 0 {
				typeName = ReceiverTypeName(fd.Recv.List[0].Type)
			}

			t.Logf("%s.README (pkg=%s): IsInterfaceMethod=%v", typeName, sym.PackageIdentifier.Name, isIface)
			found = true

			// All README methods should be interface methods (implement Command[T])
			if !isIface {
				t.Errorf("%s.README should be an interface method", typeName)
			}

			// Also test type lookup
			if typeName != "" {
				// Try different lookup formats
				lookups := []string{
					sym.PackageIdentifier.Name + "." + typeName,
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
	}

	if !found {
		t.Error("No README methods found")
	}
}
