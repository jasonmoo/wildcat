package golang

import (
	"context"
	"fmt"
	"go/ast"
	"go/types"
	"sort"
	"strings"
	"testing"
)

// TestEnumeratePaths is an exploratory test to understand the path space.
// Run with: go test -v -run TestEnumeratePaths ./internal/golang/
func TestEnumeratePaths(t *testing.T) {
	ctx := context.Background()

	// Load the golang package itself
	proj, err := LoadModulePackages(ctx, ".", nil)
	if err != nil {
		t.Fatalf("failed to load packages: %v", err)
	}

	var golangPkg *Package
	for _, pkg := range proj.Packages {
		if strings.HasSuffix(pkg.Identifier.PkgPath, "internal/golang") {
			golangPkg = pkg
			break
		}
	}
	if golangPkg == nil {
		t.Fatal("could not find internal/golang package")
	}

	var paths []string
	pkgName := "golang" // short name for paths

	// Walk all declarations
	for _, file := range golangPkg.Package.Syntax {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				paths = append(paths, enumFuncPaths(pkgName, d, golangPkg.Package.TypesInfo)...)
			case *ast.GenDecl:
				paths = append(paths, enumGenDeclPaths(pkgName, d, golangPkg.Package.TypesInfo)...)
			}
		}
	}

	sort.Strings(paths)

	// Print all paths
	fmt.Println("\n=== PATH ENUMERATION ===")
	fmt.Printf("Package: %s (%d paths)\n\n", golangPkg.Identifier.PkgPath, len(paths))
	for _, p := range paths {
		fmt.Println(p)
	}
	fmt.Println("\n=== END ===")
}

// enumFuncPaths generates paths for a function or method declaration.
func enumFuncPaths(pkg string, fn *ast.FuncDecl, info *types.Info) []string {
	var paths []string

	// Build the base path
	var basePath string
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		// Method: pkg.Type.Method
		recvType := ReceiverTypeName(fn.Recv.List[0].Type)
		basePath = fmt.Sprintf("%s.%s.%s", pkg, recvType, fn.Name.Name)
	} else {
		// Function: pkg.Func
		basePath = fmt.Sprintf("%s.%s", pkg, fn.Name.Name)
	}
	paths = append(paths, basePath)

	// Parameters: basePath/params[N] or basePath/params[name]
	if fn.Type.Params != nil {
		paramIdx := 0
		for _, field := range fn.Type.Params.List {
			if len(field.Names) == 0 {
				// Anonymous parameter - only positional access
				paths = append(paths, fmt.Sprintf("%s/params[%d]", basePath, paramIdx))
				paramIdx++
			} else {
				for _, name := range field.Names {
					paths = append(paths, fmt.Sprintf("%s/params[%d]", basePath, paramIdx))
					paths = append(paths, fmt.Sprintf("%s/params[%s]", basePath, name.Name))
					paramIdx++
				}
			}
		}
	}

	// Returns: basePath/returns[N] or basePath/returns[name]
	if fn.Type.Results != nil {
		resultIdx := 0
		for _, field := range fn.Type.Results.List {
			if len(field.Names) == 0 {
				// Anonymous return - only positional access
				paths = append(paths, fmt.Sprintf("%s/returns[%d]", basePath, resultIdx))
				resultIdx++
			} else {
				for _, name := range field.Names {
					paths = append(paths, fmt.Sprintf("%s/returns[%d]", basePath, resultIdx))
					paths = append(paths, fmt.Sprintf("%s/returns[%s]", basePath, name.Name))
					resultIdx++
				}
			}
		}
	}

	// Body: basePath/body
	if fn.Body != nil {
		paths = append(paths, fmt.Sprintf("%s/body", basePath))
	}

	return paths
}

// enumGenDeclPaths generates paths for type, var, const declarations.
func enumGenDeclPaths(pkg string, decl *ast.GenDecl, info *types.Info) []string {
	var paths []string

	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			paths = append(paths, enumTypePaths(pkg, s, info)...)
		case *ast.ValueSpec:
			// var or const
			for _, name := range s.Names {
				paths = append(paths, fmt.Sprintf("%s.%s", pkg, name.Name))
			}
		}
	}

	return paths
}

// enumTypePaths generates paths for a type declaration.
func enumTypePaths(pkg string, spec *ast.TypeSpec, info *types.Info) []string {
	var paths []string

	basePath := fmt.Sprintf("%s.%s", pkg, spec.Name.Name)
	paths = append(paths, basePath)

	switch t := spec.Type.(type) {
	case *ast.StructType:
		// Fields: basePath/fields[N] or basePath/fields[Name]
		if t.Fields != nil {
			fieldIdx := 0
			for _, field := range t.Fields.List {
				if len(field.Names) == 0 {
					// Embedded field - use type name
					typeName := typeExprName(field.Type)
					paths = append(paths, fmt.Sprintf("%s/fields[%d]", basePath, fieldIdx))
					paths = append(paths, fmt.Sprintf("%s/fields[%s]", basePath, typeName))
					fieldIdx++
				} else {
					for _, name := range field.Names {
						paths = append(paths, fmt.Sprintf("%s/fields[%d]", basePath, fieldIdx))
						paths = append(paths, fmt.Sprintf("%s/fields[%s]", basePath, name.Name))
						fieldIdx++
					}
				}
			}
		}

	case *ast.InterfaceType:
		// Methods: basePath/methods[N] or basePath/methods[Name]
		if t.Methods != nil {
			methodIdx := 0
			for _, method := range t.Methods.List {
				if len(method.Names) > 0 {
					for _, name := range method.Names {
						paths = append(paths, fmt.Sprintf("%s/methods[%d]", basePath, methodIdx))
						paths = append(paths, fmt.Sprintf("%s/methods[%s]", basePath, name.Name))
						methodIdx++
					}
				} else {
					// Embedded interface
					typeName := typeExprName(method.Type)
					paths = append(paths, fmt.Sprintf("%s/embeds[%s]", basePath, typeName))
				}
			}
		}
	}

	return paths
}

// typeExprName extracts a readable name from a type expression.
func typeExprName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", typeExprName(t.X), t.Sel.Name)
	case *ast.StarExpr:
		return typeExprName(t.X)
	default:
		return "<unknown>"
	}
}
