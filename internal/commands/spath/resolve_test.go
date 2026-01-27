package spath

import (
	"go/ast"
	"go/token"
	"testing"
)

// TestResolveSegments tests segment navigation with synthetic AST nodes.
func TestResolveSegments(t *testing.T) {
	tests := []struct {
		name     string
		node     ast.Node
		segments []Segment
		wantType string
		wantErr  bool
	}{
		{
			name:     "no segments",
			node:     makeStructDecl("Foo", "Name", "Age"),
			segments: nil,
			wantType: "*ast.GenDecl",
		},
		{
			name:     "field by name",
			node:     makeStructDecl("Foo", "Name", "Age"),
			segments: []Segment{{Category: "fields", Selector: "Name"}},
			wantType: "*ast.Field",
		},
		{
			name:     "field by index",
			node:     makeStructDecl("Foo", "Name", "Age"),
			segments: []Segment{{Category: "fields", Selector: "1", IsIndex: true}},
			wantType: "*ast.Field",
		},
		{
			name:     "field not found",
			node:     makeStructDecl("Foo", "Name", "Age"),
			segments: []Segment{{Category: "fields", Selector: "Missing"}},
			wantErr:  true,
		},
		{
			name:     "field index out of range",
			node:     makeStructDecl("Foo", "Name", "Age"),
			segments: []Segment{{Category: "fields", Selector: "99", IsIndex: true}},
			wantErr:  true,
		},
		{
			name:     "param by name",
			node:     makeFuncDecl("Foo", []string{"ctx", "id"}, []string{"error"}),
			segments: []Segment{{Category: "params", Selector: "ctx"}},
			wantType: "*ast.Field",
		},
		{
			name:     "param by index",
			node:     makeFuncDecl("Foo", []string{"ctx", "id"}, []string{"error"}),
			segments: []Segment{{Category: "params", Selector: "1", IsIndex: true}},
			wantType: "*ast.Field",
		},
		{
			name:     "return by index",
			node:     makeFuncDecl("Foo", []string{"ctx"}, []string{"result", "error"}),
			segments: []Segment{{Category: "returns", Selector: "0", IsIndex: true}},
			wantType: "*ast.Field",
		},
		{
			name:     "body",
			node:     makeFuncDecl("Foo", nil, nil),
			segments: []Segment{{Category: "body"}},
			wantType: "*ast.BlockStmt",
		},
		{
			name:     "chained: param then type",
			node:     makeFuncDecl("Foo", []string{"ctx"}, nil),
			segments: []Segment{
				{Category: "params", Selector: "ctx"},
				{Category: "type"},
			},
			wantType: "*ast.Ident",
		},
		{
			name:     "unknown category",
			node:     makeStructDecl("Foo", "Name"),
			segments: []Segment{{Category: "unknown"}},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := &Resolution{
				Path:       &Path{Package: "test", Symbol: "Test", Segments: tt.segments},
				Node:       tt.node,
				FieldIndex: -1,
			}

			err := ResolveSegments(res)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			gotType := nodeTypeName(res.Node)
			if gotType != tt.wantType {
				t.Errorf("node type = %s, want %s", gotType, tt.wantType)
			}
		})
	}
}

func TestResolveReceiver(t *testing.T) {
	// Method with receiver
	method := makeMethodDecl("s", "Foo", "String")
	res := &Resolution{
		Path:       &Path{Package: "test", Symbol: "Foo", Method: "String", Segments: []Segment{{Category: "receiver"}}},
		Node:       method,
		FieldIndex: -1,
	}

	if err := ResolveSegments(res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := res.Node.(*ast.Field); !ok {
		t.Errorf("expected *ast.Field, got %T", res.Node)
	}
	if res.Field == nil {
		t.Error("Field should be set")
	}
}

func TestResolveFieldIndex(t *testing.T) {
	node := makeStructDecl("Foo", "A", "B", "C")

	// By name and by index should give same FieldIndex
	tests := []struct {
		byName  string
		byIndex int
	}{
		{"A", 0},
		{"B", 1},
		{"C", 2},
	}

	for _, tt := range tests {
		resByName := &Resolution{
			Path:       &Path{Segments: []Segment{{Category: "fields", Selector: tt.byName}}},
			Node:       node,
			FieldIndex: -1,
		}
		resByIndex := &Resolution{
			Path:       &Path{Segments: []Segment{{Category: "fields", Selector: string(rune('0' + tt.byIndex)), IsIndex: true}}},
			Node:       node,
			FieldIndex: -1,
		}

		if err := ResolveSegments(resByName); err != nil {
			t.Errorf("by name %q: %v", tt.byName, err)
			continue
		}
		if err := ResolveSegments(resByIndex); err != nil {
			t.Errorf("by index %d: %v", tt.byIndex, err)
			continue
		}

		if resByName.FieldIndex != resByIndex.FieldIndex {
			t.Errorf("field %q: name gave index %d, position gave %d",
				tt.byName, resByName.FieldIndex, resByIndex.FieldIndex)
		}
		if resByName.FieldIndex != tt.byIndex {
			t.Errorf("field %q: expected index %d, got %d",
				tt.byName, tt.byIndex, resByName.FieldIndex)
		}
	}
}

// Helper functions to create synthetic AST nodes

func makeStructDecl(name string, fields ...string) *ast.GenDecl {
	var fieldList []*ast.Field
	for _, f := range fields {
		fieldList = append(fieldList, &ast.Field{
			Names: []*ast.Ident{{Name: f}},
			Type:  &ast.Ident{Name: "string"},
		})
	}
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{
				Name: &ast.Ident{Name: name},
				Type: &ast.StructType{
					Fields: &ast.FieldList{List: fieldList},
				},
			},
		},
	}
}

func makeFuncDecl(name string, params, returns []string) *ast.FuncDecl {
	var paramList []*ast.Field
	for _, p := range params {
		paramList = append(paramList, &ast.Field{
			Names: []*ast.Ident{{Name: p}},
			Type:  &ast.Ident{Name: "any"},
		})
	}

	var returnList []*ast.Field
	for _, r := range returns {
		returnList = append(returnList, &ast.Field{
			Names: []*ast.Ident{{Name: r}},
			Type:  &ast.Ident{Name: "any"},
		})
	}

	var resultsField *ast.FieldList
	if len(returnList) > 0 {
		resultsField = &ast.FieldList{List: returnList}
	}

	return &ast.FuncDecl{
		Name: &ast.Ident{Name: name},
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: paramList},
			Results: resultsField,
		},
		Body: &ast.BlockStmt{},
	}
}

func makeMethodDecl(recv, typeName, methodName string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: &ast.FieldList{
			List: []*ast.Field{
				{
					Names: []*ast.Ident{{Name: recv}},
					Type:  &ast.StarExpr{X: &ast.Ident{Name: typeName}},
				},
			},
		},
		Name: &ast.Ident{Name: methodName},
		Type: &ast.FuncType{
			Params: &ast.FieldList{},
		},
		Body: &ast.BlockStmt{},
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
