package spath

import (
	"context"
	"go/ast"
	"strings"
	"testing"

	"github.com/jasonmoo/wildcat/internal/golang"
)

func TestResolve(t *testing.T) {
	ctx := context.Background()

	// Load the golang package
	proj, err := golang.LoadModulePackages(ctx, ".", nil)
	if err != nil {
		t.Fatalf("failed to load packages: %v", err)
	}

	// Find the golang package
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

	tests := []struct {
		name      string
		path      string
		wantNode  string // Expected node type
		wantErr   bool
	}{
		// Top-level symbols
		{
			name:     "type Symbol",
			path:     "golang.Symbol",
			wantNode: "*ast.GenDecl",
		},
		{
			name:     "function LoadModulePackages",
			path:     "golang.LoadModulePackages",
			wantNode: "*ast.FuncDecl",
		},

		// Methods
		{
			name:     "method Symbol.String",
			path:     "golang.Symbol.Signature",
			wantNode: "*ast.FuncDecl",
		},

		// Struct fields
		{
			name:     "field Symbol.Name by name",
			path:     "golang.Symbol/fields[Name]",
			wantNode: "*ast.Field",
		},
		{
			name:     "field Symbol by index",
			path:     "golang.Symbol/fields[0]",
			wantNode: "*ast.Field",
		},

		// Function parameters
		{
			name:     "param by name",
			path:     "golang.LoadModulePackages/params[ctx]",
			wantNode: "*ast.Field",
		},
		{
			name:     "param by index",
			path:     "golang.LoadModulePackages/params[0]",
			wantNode: "*ast.Field",
		},

		// Returns
		{
			name:     "return by index",
			path:     "golang.LoadModulePackages/returns[0]",
			wantNode: "*ast.Field",
		},

		// Method receiver
		{
			name:     "receiver",
			path:     "golang.Symbol.Signature/receiver",
			wantNode: "*ast.Field",
		},

		// Body
		{
			name:     "function body",
			path:     "golang.Symbol.Signature/body",
			wantNode: "*ast.BlockStmt",
		},

		// Chained paths
		{
			name:     "param type",
			path:     "golang.LoadModulePackages/params[ctx]/type",
			wantNode: "*ast.SelectorExpr", // context.Context
		},
		{
			name:     "receiver name",
			path:     "golang.Symbol.Signature/receiver/name",
			wantNode: "*ast.Ident",
		},

		// Errors
		{
			name:    "package not found",
			path:    "nonexistent.Symbol",
			wantErr: true,
		},
		{
			name:    "symbol not found",
			path:    "golang.NonExistent",
			wantErr: true,
		},
		{
			name:    "method not found",
			path:    "golang.Symbol.NonExistent",
			wantErr: true,
		},
		{
			name:    "field not found",
			path:    "golang.Symbol/fields[NonExistent]",
			wantErr: true,
		},
		{
			name:    "param index out of range",
			path:    "golang.LoadModulePackages/params[99]",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := Parse(tt.path)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.path, err)
			}

			res, err := Resolve(proj, path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Resolve(%q) expected error, got nil", tt.path)
				}
				return
			}
			if err != nil {
				t.Errorf("Resolve(%q) unexpected error: %v", tt.path, err)
				return
			}

			// Check node type
			nodeType := nodeTypeName(res.Node)
			if nodeType != tt.wantNode {
				t.Errorf("Resolve(%q) node type = %s, want %s", tt.path, nodeType, tt.wantNode)
			}
		})
	}
}

func TestResolveFieldIndex(t *testing.T) {
	ctx := context.Background()

	proj, err := golang.LoadModulePackages(ctx, ".", nil)
	if err != nil {
		t.Fatalf("failed to load packages: %v", err)
	}

	// Test that positional and named access give same field index
	tests := []struct {
		byName  string
		byIndex string
	}{
		{"golang.Symbol/fields[Kind]", "golang.Symbol/fields[0]"},
		{"golang.Symbol/fields[Name]", "golang.Symbol/fields[1]"},
	}

	for _, tt := range tests {
		pathByName, _ := Parse(tt.byName)
		pathByIndex, _ := Parse(tt.byIndex)

		resByName, err := Resolve(proj, pathByName)
		if err != nil {
			t.Errorf("Resolve(%q) error: %v", tt.byName, err)
			continue
		}

		resByIndex, err := Resolve(proj, pathByIndex)
		if err != nil {
			t.Errorf("Resolve(%q) error: %v", tt.byIndex, err)
			continue
		}

		// Should resolve to the same field
		if resByName.Field != resByIndex.Field {
			t.Errorf("field mismatch: %q and %q resolved to different fields", tt.byName, tt.byIndex)
		}
	}
}

func nodeTypeName(node ast.Node) string {
	if node == nil {
		return "<nil>"
	}
	switch node.(type) {
	case *ast.FuncDecl:
		return "*ast.FuncDecl"
	case *ast.GenDecl:
		return "*ast.GenDecl"
	case *ast.Field:
		return "*ast.Field"
	case *ast.BlockStmt:
		return "*ast.BlockStmt"
	case *ast.CommentGroup:
		return "*ast.CommentGroup"
	case *ast.BasicLit:
		return "*ast.BasicLit"
	case *ast.Ident:
		return "*ast.Ident"
	case *ast.SelectorExpr:
		return "*ast.SelectorExpr"
	case *ast.StarExpr:
		return "*ast.StarExpr"
	case *ast.ArrayType:
		return "*ast.ArrayType"
	case *ast.MapType:
		return "*ast.MapType"
	case *ast.InterfaceType:
		return "*ast.InterfaceType"
	case *ast.StructType:
		return "*ast.StructType"
	default:
		return "<unknown>"
	}
}
