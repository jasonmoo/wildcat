package golang

import (
	"context"
	"go/ast"
	"go/types"
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

func TestGenericInterfaceImplementation(t *testing.T) {
	project, err := LoadModulePackages(context.Background(), "../..", nil)
	if err != nil {
		t.Fatalf("LoadModulePackages: %v", err)
	}

	// Call ComputeInterfaceRelations to populate ImplementedBy
	ComputeInterfaceRelations(project.Packages, nil)

	// Find Command interface (a generic interface)
	var commandSym *Symbol
	for _, pkg := range project.Packages {
		for _, sym := range pkg.Symbols {
			if sym.Name == "Command" && sym.Kind == SymbolKindInterface {
				commandSym = sym
				break
			}
		}
		if commandSym != nil {
			break
		}
	}

	if commandSym == nil {
		t.Fatal("Command interface not found")
	}

	// Verify it's a generic interface
	named, ok := commandSym.Object.Type().(*types.Named)
	if !ok || named.TypeParams().Len() == 0 {
		t.Fatal("Command should be a generic interface")
	}

	// Check that implementations were found
	// There are 6 command types: Deadcode, Package, Readme, Search, Symbol, Tree
	if len(commandSym.ImplementedBy) < 6 {
		t.Errorf("Expected at least 6 implementations of Command[T], got %d", len(commandSym.ImplementedBy))
		for _, impl := range commandSym.ImplementedBy {
			t.Logf("  Found: %s", impl.PkgSymbol())
		}
	}
}
