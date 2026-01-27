package spath

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/jasonmoo/wildcat/internal/golang"
)

// ChildPath represents an available child path from a given resolution.
type ChildPath struct {
	Path     string // full path string
	Category string // e.g., "fields", "params", "body"
	Selector string // e.g., "Name", "0", "" for body/doc
	Kind     string // e.g., "field", "param", "body", "method"
	Type     string // type annotation (e.g., "*golang.Project", "func() string")
}

// EnumerateChildren returns all available child paths from a resolution.
// For symbols, pass the Symbol to include precomputed methods.
// For deeper resolutions (fields, params), sym can be nil.
func EnumerateChildren(res *Resolution, sym *golang.Symbol) []ChildPath {
	return EnumerateChildrenWithBase(res, sym, res.Path)
}

// EnumerateSelf returns a ChildPath describing the resolved node itself.
// This uses the same kind/type logic as child enumeration for consistency.
func EnumerateSelf(res *Resolution, sym *golang.Symbol, basePath *Path) ChildPath {
	if res == nil {
		return ChildPath{}
	}

	// Get FileSet for line counting
	var fset *token.FileSet
	if res.Package != nil && res.Package.Package != nil {
		fset = res.Package.Package.Fset
	}

	// Default to symbol info when no subpath
	if !res.Path.HasSubpath() {
		return ChildPath{
			Path: basePath.String(),
			Kind: string(sym.Kind),
			Type: symbolTypeString(sym),
		}
	}

	// Determine kind/type based on last segment category and resolved node
	lastSeg := res.Path.Segments[len(res.Path.Segments)-1]
	kind := lastSeg.Category
	typeStr := ""

	switch lastSeg.Category {
	case "receiver", "params", "returns", "fields", "embeds", "typeparams":
		if res.Field != nil {
			typeStr = golang.FormatNode(res.Field.Type)
		}
		// Adjust kind to singular form
		switch lastSeg.Category {
		case "params":
			kind = "param"
		case "returns":
			kind = "return"
		case "fields":
			kind = "field"
		case "embeds":
			kind = "embed"
		case "typeparams":
			kind = "typeparam"
		}
	case "methods":
		kind = "method"
		if res.Field != nil {
			typeStr = golang.FormatNode(res.Field.Type)
		}
	case "body":
		if block, ok := res.Node.(*ast.BlockStmt); ok {
			typeStr = bodyLoc(fset, block)
		}
	case "doc":
		if doc, ok := res.Node.(*ast.CommentGroup); ok {
			typeStr = docLines(fset, doc)
		}
	case "tag":
		// No type for tags
	}

	return ChildPath{
		Path:     basePath.String(),
		Category: lastSeg.Category,
		Selector: lastSeg.Selector,
		Kind:     kind,
		Type:     typeStr,
	}
}

// symbolTypeString returns the type annotation for a symbol.
func symbolTypeString(sym *golang.Symbol) string {
	switch sym.Kind {
	case golang.SymbolKindFunc, golang.SymbolKindMethod:
		return sym.Signature()
	case golang.SymbolKindType, golang.SymbolKindInterface:
		return sym.TypeKind()
	default:
		if sym.Object != nil {
			return sym.Object.Type().String()
		}
		return string(sym.Kind)
	}
}

// EnumerateChildrenWithBase returns child paths using a custom base path for output formatting.
// This allows using short package names in output while preserving full paths for resolution.
func EnumerateChildrenWithBase(res *Resolution, sym *golang.Symbol, basePath *Path) []ChildPath {
	if res == nil {
		return nil
	}

	// Get FileSet for line counting
	var fset *token.FileSet
	if res.Package != nil && res.Package.Package != nil {
		fset = res.Package.Package.Fset
	}

	var children []ChildPath

	switch node := res.Node.(type) {
	case *ast.FuncDecl:
		children = enumerateFunc(basePath, node, fset)
	case *ast.GenDecl:
		children = enumerateGenDecl(basePath, node, sym, fset)
	case *ast.TypeSpec:
		children = enumerateTypeSpec(basePath, node, sym, fset, nil)
	case *ast.Field:
		children = enumerateField(basePath, node, fset)
	case *ast.BlockStmt:
		// At body level - no deeper children for now
	case *ast.CommentGroup:
		// At doc level - leaf node
	case *ast.BasicLit:
		// At tag or value level - leaf node
	}

	return children
}

// enumerateFunc lists children of a function declaration.
func enumerateFunc(basePath *Path, fn *ast.FuncDecl, fset *token.FileSet) []ChildPath {
	var children []ChildPath

	// Receiver (if method)
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv := fn.Recv.List[0]
		children = append(children, ChildPath{
			Path:     basePath.WithSegment("receiver", "", false).String(),
			Category: "receiver",
			Kind:     "receiver",
			Type:     golang.FormatNode(recv.Type),
		})
	}

	// Type parameters (if generic)
	if fn.Type.TypeParams != nil {
		children = append(children, enumerateFieldList(basePath, "typeparams", "typeparam", fn.Type.TypeParams)...)
	}

	// Parameters
	if fn.Type.Params != nil && len(fn.Type.Params.List) > 0 {
		children = append(children, enumerateFieldList(basePath, "params", "param", fn.Type.Params)...)
	}

	// Returns
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		children = append(children, enumerateFieldList(basePath, "returns", "return", fn.Type.Results)...)
	}

	// Body
	if fn.Body != nil {
		children = append(children, ChildPath{
			Path:     basePath.WithSegment("body", "", false).String(),
			Category: "body",
			Kind:     "body",
			Type:     bodyLoc(fset, fn.Body),
		})
	}

	// Doc comment
	if fn.Doc != nil {
		children = append(children, ChildPath{
			Path:     basePath.WithSegment("doc", "", false).String(),
			Category: "doc",
			Kind:     "doc",
			Type:     docLines(fset, fn.Doc),
		})
	}

	return children
}

// docLines returns a "N lines" string for a comment group.
func docLines(fset *token.FileSet, doc *ast.CommentGroup) string {
	if fset == nil || doc == nil || len(doc.List) == 0 {
		return ""
	}
	startLine := fset.Position(doc.List[0].Pos()).Line
	endLine := fset.Position(doc.List[len(doc.List)-1].End()).Line
	lines := endLine - startLine + 1
	if lines == 1 {
		return "1 line"
	}
	return fmt.Sprintf("%d lines", lines)
}

// bodyLoc returns a "N loc" string for a block statement.
func bodyLoc(fset *token.FileSet, block *ast.BlockStmt) string {
	if fset == nil || block == nil {
		return ""
	}
	startLine := fset.Position(block.Lbrace).Line
	endLine := fset.Position(block.Rbrace).Line
	return fmt.Sprintf("%d loc", endLine-startLine+1)
}

// enumerateGenDecl lists children of a general declaration.
func enumerateGenDecl(basePath *Path, gd *ast.GenDecl, sym *golang.Symbol, fset *token.FileSet) []ChildPath {
	if len(gd.Specs) == 0 {
		return nil
	}

	switch spec := gd.Specs[0].(type) {
	case *ast.TypeSpec:
		return enumerateTypeSpec(basePath, spec, sym, fset, gd.Doc)
	case *ast.ValueSpec:
		return enumerateValueSpec(basePath, spec, gd.Doc, fset)
	}

	return nil
}

// enumerateTypeSpec lists children of a type specification.
func enumerateTypeSpec(basePath *Path, ts *ast.TypeSpec, sym *golang.Symbol, fset *token.FileSet, parentDoc *ast.CommentGroup) []ChildPath {
	var children []ChildPath

	// Type parameters (if generic)
	if ts.TypeParams != nil {
		children = append(children, enumerateFieldList(basePath, "typeparams", "typeparam", ts.TypeParams)...)
	}

	// Doc comment (prefer TypeSpec doc, fall back to parent GenDecl doc)
	doc := ts.Doc
	if doc == nil {
		doc = parentDoc
	}
	if doc != nil {
		children = append(children, ChildPath{
			Path:     basePath.WithSegment("doc", "", false).String(),
			Category: "doc",
			Kind:     "doc",
			Type:     docLines(fset, doc),
		})
	}

	switch t := ts.Type.(type) {
	case *ast.StructType:
		children = append(children, enumerateStructType(basePath, t)...)
	case *ast.InterfaceType:
		children = append(children, enumerateInterfaceType(basePath, t)...)
	}

	// Add concrete methods from symbol (precomputed)
	// Use basePath to maintain consistent path format (not full import path)
	if sym != nil {
		for _, method := range sym.Methods {
			// Build method path using basePath's package format
			methodPath := &Path{
				Package: basePath.Package,
				Symbol:  basePath.Symbol,
				Method:  method.Name,
			}
			children = append(children, ChildPath{
				Path:     methodPath.String(),
				Category: "methods",
				Selector: method.Name,
				Kind:     "method",
				Type:     method.Signature(),
			})
		}
	}

	return children
}

// enumerateStructType lists fields and embeds from a struct.
func enumerateStructType(basePath *Path, st *ast.StructType) []ChildPath {
	var children []ChildPath

	if st.Fields == nil {
		return children
	}

	for _, field := range st.Fields.List {
		typeStr := golang.FormatNode(field.Type)
		if len(field.Names) == 0 {
			// Embedded type
			name := typeExprName(field.Type)
			if name != "" {
				children = append(children, ChildPath{
					Path:     basePath.WithSegment("embeds", name, false).String(),
					Category: "embeds",
					Selector: name,
					Kind:     "embed",
					Type:     typeStr,
				})
			}
		} else {
			// Named field(s)
			for _, ident := range field.Names {
				children = append(children, ChildPath{
					Path:     basePath.WithSegment("fields", ident.Name, false).String(),
					Category: "fields",
					Selector: ident.Name,
					Kind:     "field",
					Type:     typeStr,
				})
			}
		}
	}

	return children
}

// enumerateInterfaceType lists methods and embeds from an interface.
func enumerateInterfaceType(basePath *Path, it *ast.InterfaceType) []ChildPath {
	var children []ChildPath

	if it.Methods == nil {
		return children
	}

	for _, field := range it.Methods.List {
		if len(field.Names) == 0 {
			// Could be embedded interface or type constraint
			if _, ok := field.Type.(*ast.FuncType); !ok {
				name := typeExprName(field.Type)
				if name != "" {
					children = append(children, ChildPath{
						Path:     basePath.WithSegment("embeds", name, false).String(),
						Category: "embeds",
						Selector: name,
						Kind:     "embed",
						Type:     golang.FormatNode(field.Type),
					})
				}
			}
		} else {
			// Method signature
			for _, ident := range field.Names {
				children = append(children, ChildPath{
					Path:     basePath.WithSegment("methods", ident.Name, false).String(),
					Category: "methods",
					Selector: ident.Name,
					Kind:     "method",
					Type:     golang.FormatNode(field.Type),
				})
			}
		}
	}

	return children
}

// enumerateValueSpec lists children of a var/const declaration.
func enumerateValueSpec(basePath *Path, vs *ast.ValueSpec, genDeclDoc *ast.CommentGroup, fset *token.FileSet) []ChildPath {
	var children []ChildPath

	// Type (if explicit)
	if vs.Type != nil {
		children = append(children, ChildPath{
			Path:     basePath.WithSegment("type", "", false).String(),
			Category: "type",
			Kind:     "type",
		})
	}

	// Value (if present)
	if len(vs.Values) > 0 {
		children = append(children, ChildPath{
			Path:     basePath.WithSegment("value", "", false).String(),
			Category: "value",
			Kind:     "value",
		})
	}

	// Doc comment (check both spec and parent GenDecl)
	doc := vs.Doc
	if doc == nil {
		doc = genDeclDoc
	}
	if doc != nil {
		children = append(children, ChildPath{
			Path:     basePath.WithSegment("doc", "", false).String(),
			Category: "doc",
			Kind:     "doc",
			Type:     docLines(fset, doc),
		})
	}

	return children
}

// enumerateField lists children of a field (struct field, param, etc.)
func enumerateField(basePath *Path, field *ast.Field, fset *token.FileSet) []ChildPath {
	var children []ChildPath

	// Tag (struct fields only)
	if field.Tag != nil {
		children = append(children, ChildPath{
			Path:     basePath.WithSegment("tag", "", false).String(),
			Category: "tag",
			Kind:     "tag",
		})
	}

	// Doc comment
	if field.Doc != nil {
		children = append(children, ChildPath{
			Path:     basePath.WithSegment("doc", "", false).String(),
			Category: "doc",
			Kind:     "doc",
			Type:     docLines(fset, field.Doc),
		})
	}

	return children
}

// enumerateFieldList creates child paths for a field list (params, returns, etc.)
func enumerateFieldList(basePath *Path, category, kind string, fl *ast.FieldList) []ChildPath {
	var children []ChildPath

	idx := 0
	for _, field := range fl.List {
		typeStr := golang.FormatNode(field.Type)
		if len(field.Names) == 0 {
			// Unnamed - use positional index
			children = append(children, ChildPath{
				Path:     basePath.WithSegment(category, itoa(idx), true).String(),
				Category: category,
				Selector: itoa(idx),
				Kind:     kind,
				Type:     typeStr,
			})
			idx++
		} else {
			// Named
			for _, name := range field.Names {
				children = append(children, ChildPath{
					Path:     basePath.WithSegment(category, name.Name, false).String(),
					Category: category,
					Selector: name.Name,
					Kind:     kind,
					Type:     typeStr,
				})
				idx++
			}
		}
	}

	return children
}
