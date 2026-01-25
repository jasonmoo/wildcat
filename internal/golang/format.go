package golang

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"strings"
)

// ReceiverTypeName extracts the type name from a method receiver.
// Returns "<unknown receiver>" if the expression type is unrecognized.
func ReceiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		// *T or *T[U] - recurse to handle generic pointer receivers
		return ReceiverTypeName(t.X)
	case *ast.ParenExpr:
		// (T) - parenthesized, recurse
		return ReceiverTypeName(t.X)
	case *ast.IndexExpr:
		// T[U] - generic type with single type param
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
		// Could be (*T)[U] or other nested expression
		return ReceiverTypeName(t.X)
	case *ast.IndexListExpr:
		// T[U, V] - generic type with multiple type params
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
		// Could be nested
		return ReceiverTypeName(t.X)
	}
	return "<unknown receiver>"
}

// constructorTypeName returns the type name if this function looks like a constructor.
// A constructor returns T or *T where T is a local exported type.
func ConstructorTypeName(ft *ast.FuncType) string {
	if ft.Results == nil || len(ft.Results.List) == 0 {
		return ""
	}
	// Check first return type
	ret := ft.Results.List[0].Type
	name := ""
	switch t := ret.(type) {
	case *ast.Ident:
		name = t.Name
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			name = ident.Name
		}
	}
	return name
}

// ChannelElemType returns the element type of a channel expression.
// Returns "<unknown type>" if type info is unavailable (e.g., code doesn't compile).
func ChannelElemType(info *types.Info, expr ast.Expr) string {
	if t := info.TypeOf(expr); t != nil {
		if ch, ok := t.Underlying().(*types.Chan); ok {
			return ch.Elem().String()
		}
	}
	return "<unknown type>"
}

// FormatNode formats an AST node to its canonical source representation.
// Strips comments and doc strings from the node before formatting.
func FormatNode(node ast.Node) string {
	stripComments(node)
	var sb strings.Builder
	if err := format.Node(&sb, token.NewFileSet(), node); err != nil {
		return fmt.Sprintf("<format error: %v>", err)
	}
	return sb.String()
}

// stripComments removes comments and doc strings from an AST node.
func stripComments(node ast.Node) {
	ast.Inspect(node, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncDecl:
			v.Body = nil
			v.Doc = nil
		case *ast.GenDecl:
			v.Doc = nil
		case *ast.TypeSpec:
			v.Doc = nil
			v.Comment = nil
		case *ast.ValueSpec:
			v.Doc = nil
			v.Comment = nil
		case *ast.Field:
			v.Doc = nil
			v.Comment = nil
		case *ast.ImportSpec:
			v.Doc = nil
			v.Comment = nil
		}
		return true
	})
}

