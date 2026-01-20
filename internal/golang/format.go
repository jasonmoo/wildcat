package golang

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"io"
	"strings"

	"github.com/kr/pretty"
)

// func ObjKind()

func FormatDecl(v ast.Decl) ([]string, error) {
	// pretty.Println(v)
	// return "", nil
	switch val := v.(type) {
	case *ast.FuncDecl:
		sig, err := FormatFuncDecl(val)
		if err != nil {
			return nil, err
		}
		return []string{sig}, nil
	case *ast.GenDecl:
		var sigs []string
		for _, spec := range val.Specs {
			switch sp := spec.(type) {
			case *ast.TypeSpec:
				sig, err := FormatTypeSpec(val.Tok, sp)
				if err != nil {
					return nil, err
				}
				sigs = append(sigs, sig)
			case *ast.ValueSpec:
				sig, err := FormatValueSpec(val.Tok, sp)
				if err != nil {
					return nil, err
				}
				sigs = append(sigs, sig)
			}

		}
		return sigs, nil
	default:
		pretty.Println(v)
	}
	return []string{"UNKNOWN"}, nil
}

func FormatFuncDecl(v *ast.FuncDecl) (string, error) {
	var sb strings.Builder
	if err := formatFuncDecl(&sb, v); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func formatFuncDecl(w io.Writer, v *ast.FuncDecl) error {
	v.Doc = nil
	v.Body = nil
	stripFieldList(v.Recv)
	if v.Type != nil {
		stripFieldList(v.Type.Params)
		stripFieldList(v.Type.TypeParams)
		stripFieldList(v.Type.Results)
	}
	return format.Node(w, token.NewFileSet(), v)
}

func FormatTypeSpec(tok token.Token, v *ast.TypeSpec) (string, error) {
	var sb strings.Builder
	if err := formatTypeSpec(&sb, tok, v); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func formatTypeSpec(w io.Writer, tok token.Token, spec *ast.TypeSpec) error {
	spec.Doc = nil
	spec.Comment = nil
	switch t := spec.Type.(type) {
	case *ast.StructType:
		stripFields(t.Fields.List)
	case *ast.InterfaceType:
		stripFields(t.Methods.List)
	}
	return format.Node(w, token.NewFileSet(), &ast.GenDecl{
		Tok:   tok,
		Specs: []ast.Spec{spec},
	})
}

func FormatValueSpec(tok token.Token, v *ast.ValueSpec) (string, error) {
	var sb strings.Builder
	if err := formatValueSpec(&sb, tok, v); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func formatValueSpec(w io.Writer, tok token.Token, spec *ast.ValueSpec) error {
	spec.Doc = nil
	spec.Comment = nil
	for _, v := range spec.Values {
		if val, ok := v.(*ast.BasicLit); ok && val.Kind == token.STRING {
			if ct := strings.Count(val.Value, "\n"); ct > 0 {
				val.Value = fmt.Sprintf("<newline content: %d lines omitted>", ct)
			}
		}
	}
	return format.Node(w, token.NewFileSet(), &ast.GenDecl{
		Tok:   tok,
		Specs: []ast.Spec{spec},
	})
}

func stripFieldList(fs *ast.FieldList) {
	if fs != nil {
		for _, f := range fs.List {
			f.Doc = nil
			f.Comment = nil
		}
	}
}

func stripFields(fs []*ast.Field) {
	for _, f := range fs {
		f.Doc = nil
		f.Comment = nil
	}
}

// receiverTypeName extracts the type name from a method receiver.
// Handles both T and *T receivers.
func ReceiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
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

// ChannelElemType returns the element type of a channel expression, or empty string if not a channel.
func ChannelElemType(info *types.Info, expr ast.Expr) string {
	if t := info.TypeOf(expr); t != nil {
		if ch, ok := t.Underlying().(*types.Chan); ok {
			return ch.Elem().String()
		}
	}
	return ""
}

// FormatNode formats an AST node to its canonical source representation.
// Strips comments and doc strings from the node before formatting.
func FormatNode(node ast.Node) (string, error) {
	stripComments(node)
	var sb strings.Builder
	if err := format.Node(&sb, token.NewFileSet(), node); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// stripComments removes comments and doc strings from an AST node.
func stripComments(node ast.Node) {
	ast.Inspect(node, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncDecl:
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

