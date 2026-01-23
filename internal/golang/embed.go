package golang

import (
	"go/ast"
	"go/token"
	"strings"
)

// EmbedDirective represents a //go:embed directive found in source.
type EmbedDirective struct {
	Patterns []string       // embed patterns from directive
	VarName  string         // variable name
	VarType  string         // type expression (e.g., "embed.FS", "string", "[]byte")
	Position token.Position // file:line
	Error    string         // error message if directive couldn't be fully processed
}

// FindEmbedDirectives finds all //go:embed directives in a package.
// Returns directives in source order. If a directive can't be fully processed,
// it's still included with an Error field explaining the issue.
func FindEmbedDirectives(pkg *Package) []EmbedDirective {
	var directives []EmbedDirective

	for _, f := range pkg.Package.Syntax {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR || gd.Doc == nil {
				continue
			}

			// Look for //go:embed in doc comments
			var embedPatterns []string
			for _, comment := range gd.Doc.List {
				text := comment.Text
				if strings.HasPrefix(text, "//go:embed ") {
					patternsStr := strings.TrimPrefix(text, "//go:embed ")
					patterns := strings.Fields(patternsStr)
					embedPatterns = append(embedPatterns, patterns...)
				}
			}

			if len(embedPatterns) == 0 {
				continue
			}

			// Get variable info from the spec
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || len(vs.Names) == 0 {
					continue
				}

				varName := vs.Names[0].Name
				varType, err := FormatNode(vs.Type)

				ed := EmbedDirective{
					Patterns: embedPatterns,
					VarName:  varName,
					VarType:  varType,
					Position: pkg.Package.Fset.Position(gd.Pos()),
				}
				if err != nil {
					ed.Error = err.Error()
				}

				directives = append(directives, ed)
			}
		}
	}

	return directives
}
