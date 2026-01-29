package golang

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"sort"
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
// Use this for compact display (signatures, type expressions).
func FormatNode(node ast.Node) string {
	restore := stripComments(node)
	defer restore()
	var sb strings.Builder
	if err := format.Node(&sb, token.NewFileSet(), node); err != nil {
		return fmt.Sprintf("<format error: %v>", err)
	}
	return sb.String()
}

// RenderSource renders an AST node to source code, preserving comments.
// Use this for full source display where comments matter.
// The fset parameter should be the original FileSet from parsing.
func RenderSource(node ast.Node, fset *token.FileSet) (string, error) {
	if node == nil {
		return "", fmt.Errorf("no AST node")
	}

	// Handle node types that format.Node doesn't support
	switch n := node.(type) {
	case *ast.Field:
		return RenderField(n, fset)
	case *ast.CommentGroup:
		return RenderCommentGroup(n), nil
	case *ast.BasicLit:
		return n.Value, nil
	}

	var buf strings.Builder
	if err := format.Node(&buf, fset, node); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// RenderField renders a field (struct field, param, etc.) to source.
func RenderField(field *ast.Field, fset *token.FileSet) (string, error) {
	var buf strings.Builder

	// Names (if any)
	for i, name := range field.Names {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(name.Name)
	}

	// Type
	if field.Type != nil {
		if len(field.Names) > 0 {
			buf.WriteString(" ")
		}
		if err := format.Node(&buf, fset, field.Type); err != nil {
			return "", err
		}
	}

	// Tag
	if field.Tag != nil {
		buf.WriteString(" ")
		buf.WriteString(field.Tag.Value)
	}

	return buf.String(), nil
}

// RenderCommentGroup renders a comment group to source.
func RenderCommentGroup(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	var lines []string
	for _, c := range cg.List {
		lines = append(lines, c.Text)
	}
	return strings.Join(lines, "\n")
}

// stripComments removes comments, doc strings, function bodies, and truncates
// multiline string literals from an AST node for compact display.
// Returns a restore function that puts back all the original values.
func stripComments(node ast.Node) func() {
	var restoreFuncs []func()

	ast.Inspect(node, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncDecl:
			body, doc := v.Body, v.Doc
			v.Body, v.Doc = nil, nil
			restoreFuncs = append(restoreFuncs, func() { v.Body, v.Doc = body, doc })
		case *ast.GenDecl:
			doc := v.Doc
			v.Doc = nil
			restoreFuncs = append(restoreFuncs, func() { v.Doc = doc })
		case *ast.TypeSpec:
			doc, comment := v.Doc, v.Comment
			v.Doc, v.Comment = nil, nil
			restoreFuncs = append(restoreFuncs, func() { v.Doc, v.Comment = doc, comment })
		case *ast.ValueSpec:
			doc, comment := v.Doc, v.Comment
			v.Doc, v.Comment = nil, nil
			restoreFuncs = append(restoreFuncs, func() { v.Doc, v.Comment = doc, comment })
		case *ast.Field:
			doc, comment := v.Doc, v.Comment
			v.Doc, v.Comment = nil, nil
			restoreFuncs = append(restoreFuncs, func() { v.Doc, v.Comment = doc, comment })
		case *ast.ImportSpec:
			doc, comment := v.Doc, v.Comment
			v.Doc, v.Comment = nil, nil
			restoreFuncs = append(restoreFuncs, func() { v.Doc, v.Comment = doc, comment })
		case *ast.BasicLit:
			// Truncate multiline raw string literals (backtick strings)
			if strings.HasPrefix(v.Value, "`") && strings.Contains(v.Value, "\n") {
				orig := v.Value
				v.Value = "`..."
				restoreFuncs = append(restoreFuncs, func() { v.Value = orig })
			}
		}
		return true
	})

	return func() {
		for _, f := range restoreFuncs {
			f()
		}
	}
}

// RenderPackageSource renders a package as unified source code.
// Combines all files into a single output with:
// - One package declaration
// - Consolidated imports
// - Constants, then variables
// - Types with their constructors and methods grouped together
// - Standalone functions at the end
func RenderPackageSource(pkg *Package) (string, error) {
	if pkg == nil || pkg.Package == nil || len(pkg.Package.Syntax) == 0 {
		return "", fmt.Errorf("no source files")
	}

	var buf strings.Builder

	// Package declaration
	buf.WriteString("package ")
	buf.WriteString(pkg.Identifier.Name)
	buf.WriteString("\n")

	// Collect and consolidate imports
	imports := collectImports(pkg.Package.Syntax)
	if len(imports) > 0 {
		buf.WriteString("\nimport (\n")
		for _, imp := range imports {
			buf.WriteString("\t")
			buf.WriteString(imp)
			buf.WriteString("\n")
		}
		buf.WriteString(")\n")
	}

	// Collect declarations
	decls := collectPackageDeclarations(pkg)

	// Render constants
	if len(decls.consts) > 0 {
		for _, d := range decls.consts {
			buf.WriteString("\n")
			buf.WriteString(d.source)
			buf.WriteString("\n")
		}
	}

	// Render variables
	if len(decls.vars) > 0 {
		for _, d := range decls.vars {
			buf.WriteString("\n")
			buf.WriteString(d.source)
			buf.WriteString("\n")
		}
	}

	// Render types with their constructors and methods
	for _, t := range decls.types {
		buf.WriteString("\n")
		buf.WriteString(t.source)
		buf.WriteString("\n")
		// Constructors
		for _, c := range t.constructors {
			buf.WriteString("\n")
			buf.WriteString(c.source)
			buf.WriteString("\n")
		}
		// Methods
		for _, m := range t.methods {
			buf.WriteString("\n")
			buf.WriteString(m.source)
			buf.WriteString("\n")
		}
	}

	// Render standalone functions
	for _, d := range decls.funcs {
		buf.WriteString("\n")
		buf.WriteString(d.source)
		buf.WriteString("\n")
	}

	return buf.String(), nil
}

// declaration holds a rendered declaration.
type declaration struct {
	name   string
	source string
}

// typeDeclaration holds a type with its associated constructors and methods.
type typeDeclaration struct {
	name         string
	source       string
	constructors []declaration
	methods      []declaration
}

// packageDeclarations holds all declarations organized for rendering.
type packageDeclarations struct {
	consts []declaration
	vars   []declaration
	types  []typeDeclaration
	funcs  []declaration
}

// collectPackageDeclarations gathers all declarations from a package.
// Uses Symbol metadata to associate methods and constructors with types.
func collectPackageDeclarations(pkg *Package) packageDeclarations {
	fset := pkg.Package.Fset
	var decls packageDeclarations

	// Build maps from Symbol data for methods and constructors
	// Key: type name, Value: list of method/constructor symbols
	methodsByType := make(map[string][]*Symbol)
	constructorsByType := make(map[string][]*Symbol)
	methodNames := make(map[string]bool)      // track which funcs are methods
	constructorNames := make(map[string]bool) // track which funcs are constructors

	for _, sym := range pkg.Symbols {
		switch sym.Kind {
		case SymbolKindType, SymbolKindInterface:
			for _, m := range sym.Methods {
				methodsByType[sym.Name] = append(methodsByType[sym.Name], m)
				methodNames[m.Name] = true
			}
			for _, c := range sym.Constructors {
				constructorsByType[sym.Name] = append(constructorsByType[sym.Name], c)
				constructorNames[c.Name] = true
			}
		}
	}

	// Collect declarations from AST
	for _, file := range pkg.Package.Syntax {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				switch d.Tok {
				case token.CONST:
					if src, err := RenderSource(d, fset); err == nil {
						decls.consts = append(decls.consts, declaration{source: src})
					}
				case token.VAR:
					if src, err := RenderSource(d, fset); err == nil {
						decls.vars = append(decls.vars, declaration{source: src})
					}
				case token.TYPE:
					// Each type spec becomes a typeDeclaration
					for _, spec := range d.Specs {
						ts, ok := spec.(*ast.TypeSpec)
						if !ok {
							continue
						}
						// Create a single-spec GenDecl for rendering
						singleDecl := &ast.GenDecl{
							Doc:    d.Doc,
							TokPos: d.TokPos,
							Tok:    d.Tok,
							Specs:  []ast.Spec{ts},
						}
						if src, err := RenderSource(singleDecl, fset); err == nil {
							td := typeDeclaration{
								name:   ts.Name.Name,
								source: src,
							}
							// Add constructors (sorted by name)
							if ctors := constructorsByType[ts.Name.Name]; len(ctors) > 0 {
								sort.Slice(ctors, func(i, j int) bool {
									return ctors[i].Name < ctors[j].Name
								})
								for _, c := range ctors {
									if csrc, err := RenderSource(c.Node, fset); err == nil {
										td.constructors = append(td.constructors, declaration{
											name:   c.Name,
											source: csrc,
										})
									}
								}
							}
							// Add methods (sorted by name)
							if methods := methodsByType[ts.Name.Name]; len(methods) > 0 {
								sort.Slice(methods, func(i, j int) bool {
									return methods[i].Name < methods[j].Name
								})
								for _, m := range methods {
									if msrc, err := RenderSource(m.Node, fset); err == nil {
										td.methods = append(td.methods, declaration{
											name:   m.Name,
											source: msrc,
										})
									}
								}
							}
							decls.types = append(decls.types, td)
						}
					}
				}
			case *ast.FuncDecl:
				// Skip methods and constructors - they're rendered with their types
				if d.Recv != nil {
					continue // method
				}
				if constructorNames[d.Name.Name] {
					continue // constructor
				}
				if src, err := RenderSource(d, fset); err == nil {
					decls.funcs = append(decls.funcs, declaration{
						name:   d.Name.Name,
						source: src,
					})
				}
			}
		}
	}

	// Sort types and standalone funcs by name
	sort.Slice(decls.types, func(i, j int) bool {
		return decls.types[i].name < decls.types[j].name
	})
	sort.Slice(decls.funcs, func(i, j int) bool {
		return decls.funcs[i].name < decls.funcs[j].name
	})

	return decls
}

// collectImports gathers all unique imports from files.
func collectImports(files []*ast.File) []string {
	seen := make(map[string]bool)
	var imports []string

	for _, file := range files {
		for _, imp := range file.Imports {
			// Build import string
			var impStr string
			if imp.Name != nil && imp.Name.Name != "_" && imp.Name.Name != "." {
				impStr = imp.Name.Name + " " + imp.Path.Value
			} else if imp.Name != nil && imp.Name.Name == "_" {
				impStr = "_ " + imp.Path.Value
			} else if imp.Name != nil && imp.Name.Name == "." {
				impStr = ". " + imp.Path.Value
			} else {
				impStr = imp.Path.Value
			}

			if !seen[impStr] {
				seen[impStr] = true
				imports = append(imports, impStr)
			}
		}
	}

	// Sort imports: stdlib first, then third-party
	sort.Slice(imports, func(i, j int) bool {
		iPath := strings.Trim(imports[i], `"`)
		jPath := strings.Trim(imports[j], `"`)
		// Remove alias prefix for sorting
		if idx := strings.Index(iPath, " "); idx != -1 {
			iPath = strings.Trim(iPath[idx+1:], `"`)
		}
		if idx := strings.Index(jPath, " "); idx != -1 {
			jPath = strings.Trim(jPath[idx+1:], `"`)
		}
		iStd := !strings.Contains(iPath, ".")
		jStd := !strings.Contains(jPath, ".")
		if iStd != jStd {
			return iStd // stdlib first
		}
		return imports[i] < imports[j]
	})

	return imports
}

