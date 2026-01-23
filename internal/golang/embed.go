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
	VarType  string         // type expression (e.g., "embed.FS", "string", "[]byte"), or error message if formatting failed
	Position token.Position // file:line
}

// FindEmbedDirectives finds all //go:embed directives in a package.
// Returns directives in source order.
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
				directives = append(directives, EmbedDirective{
					Patterns: embedPatterns,
					VarName:  varName,
					VarType:  FormatNode(vs.Type),
					Position: pkg.Package.Fset.Position(gd.Pos()),
				})
			}
		}
	}

	return directives
}
