package golang

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestFormatNode_MultilineStringTruncation(t *testing.T) {
	src := `package test
const query = ` + "`" + `
SELECT *
FROM users
` + "`" + `
var template = ` + "`" + `hello
world` + "`" + `
const singleLine = ` + "`" + `no newlines here` + "`" + `
const quoted = "also\nno truncation"
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			name := vs.Names[0].Name
			formatted := FormatNode(gd)

			switch name {
			case "query", "template":
				// Multiline backtick strings should be truncated
				if !strings.Contains(formatted, "`...") {
					t.Errorf("%s: expected truncated backtick string, got: %s", name, formatted)
				}
				if strings.Contains(formatted, "SELECT") || strings.Contains(formatted, "hello") {
					t.Errorf("%s: multiline content should be truncated, got: %s", name, formatted)
				}
			case "singleLine":
				// Single-line backtick strings should NOT be truncated
				if strings.Contains(formatted, "`...") {
					t.Errorf("%s: single-line backtick should not be truncated, got: %s", name, formatted)
				}
			case "quoted":
				// Regular quoted strings should NOT be truncated
				if strings.Contains(formatted, "...") {
					t.Errorf("%s: quoted string should not be truncated, got: %s", name, formatted)
				}
			}
		}
	}
}
