package golang

import (
	"go/ast"
	"go/format"
	"go/token"
	"io"
	"strings"
)

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
	stripFields(v.Recv.List)
	stripFields(v.Type.Params.List)
	stripFields(v.Type.TypeParams.List)
	stripFields(v.Type.Results.List)
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
	// TODO: truncate multi line values
	// for _, v := range spec.Values {
	// 	switch vv := v.(type) {
	// 	}
	// }
	return format.Node(w, token.NewFileSet(), &ast.GenDecl{
		Tok:   tok,
		Specs: []ast.Spec{spec},
	})
}

func stripFields(fs []*ast.Field) {
	for _, f := range fs {
		f.Doc = nil
		f.Comment = nil
	}
}
