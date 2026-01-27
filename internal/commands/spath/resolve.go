package spath

import (
	"fmt"
	"go/ast"
	"strconv"

	"github.com/jasonmoo/wildcat/internal/golang"
)

// Resolution holds the result of resolving a path.
type Resolution struct {
	// Path is the original path that was resolved.
	Path *Path

	// Package is the resolved package.
	Package *golang.Package

	// Symbol is the resolved top-level symbol (type, func, var, const).
	// Nil if path points to something below symbol level.
	Symbol *golang.Symbol

	// Node is the AST node at the path.
	// This is the most specific node - could be the symbol's node,
	// or a Field, FuncType, BlockStmt, etc. for subpaths.
	Node ast.Node

	// Field is set when the path resolves to a struct field or parameter.
	Field *ast.Field

	// FieldIndex is the flattened index of the field (for params, returns, fields).
	// -1 if not applicable.
	FieldIndex int
}

// NewResolution creates a Resolution from a parsed path, package, and symbol.
// It navigates through any segments in the path and returns the fully resolved result.
func NewResolution(path *Path, pkg *golang.Package, sym *golang.Symbol) (*Resolution, error) {
	res := &Resolution{
		Path:       path,
		Package:    pkg,
		Symbol:     sym,
		Node:       sym.Node,
		FieldIndex: -1,
	}

	// Navigate through segments
	if err := ResolveSegments(res); err != nil {
		return nil, err
	}

	return res, nil
}

// ResolveSegments navigates through all segments in the path.
// The Resolution must have Path, Package, Symbol, and Node set.
// This function modifies res in place, updating Node, Field, and FieldIndex.
func ResolveSegments(res *Resolution) error {
	for i, seg := range res.Path.Segments {
		if err := resolveSegment(res, seg); err != nil {
			return fmt.Errorf("segment %d (%s): %w", i, seg.Category, err)
		}
	}
	return nil
}

// resolveSegment navigates one segment of the path.
func resolveSegment(res *Resolution, seg Segment) error {
	switch seg.Category {
	case "fields":
		return resolveFields(res, seg)
	case "methods":
		return resolveMethods(res, seg)
	case "embeds":
		return resolveEmbeds(res, seg)
	case "params":
		return resolveParams(res, seg)
	case "returns":
		return resolveReturns(res, seg)
	case "receiver":
		return resolveReceiver(res)
	case "typeparams":
		return resolveTypeParams(res, seg)
	case "body":
		return resolveBody(res)
	case "doc":
		return resolveDoc(res)
	case "tag":
		return resolveTag(res, seg)
	case "type":
		return resolveType(res)
	case "name":
		return resolveName(res)
	case "constraint":
		return resolveConstraint(res)
	case "value":
		return resolveValue(res)
	default:
		return fmt.Errorf("unknown category: %s", seg.Category)
	}
}

// resolveFields navigates to a struct field.
func resolveFields(res *Resolution, seg Segment) error {
	// Get struct type from current node
	st, err := getStructType(res.Node)
	if err != nil {
		return err
	}

	field, idx, err := selectField(st.Fields, seg.Selector, seg.IsIndex)
	if err != nil {
		return err
	}

	res.Node = field
	res.Field = field
	res.FieldIndex = idx
	return nil
}

// resolveMethods navigates to an interface method.
func resolveMethods(res *Resolution, seg Segment) error {
	// Get interface type from current node
	it, err := getInterfaceType(res.Node)
	if err != nil {
		return err
	}

	field, idx, err := selectField(it.Methods, seg.Selector, seg.IsIndex)
	if err != nil {
		return err
	}

	res.Node = field
	res.Field = field
	res.FieldIndex = idx
	return nil
}

// resolveEmbeds navigates to an embedded type.
func resolveEmbeds(res *Resolution, seg Segment) error {
	// Can be struct or interface
	switch node := res.Node.(type) {
	case *ast.GenDecl:
		if len(node.Specs) > 0 {
			if ts, ok := node.Specs[0].(*ast.TypeSpec); ok {
				return resolveEmbedsFromType(res, ts.Type, seg)
			}
		}
	case *ast.TypeSpec:
		return resolveEmbedsFromType(res, node.Type, seg)
	}
	return fmt.Errorf("embeds requires a type declaration")
}

func resolveEmbedsFromType(res *Resolution, typeExpr ast.Expr, seg Segment) error {
	switch t := typeExpr.(type) {
	case *ast.StructType:
		return resolveStructEmbeds(res, t, seg)
	case *ast.InterfaceType:
		return resolveInterfaceEmbeds(res, t, seg)
	}
	return fmt.Errorf("embeds requires a struct or interface type")
}

func resolveStructEmbeds(res *Resolution, st *ast.StructType, seg Segment) error {
	if st.Fields == nil {
		return fmt.Errorf("struct has no fields")
	}

	// Embedded fields have no names
	idx := 0
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			// This is an embedded field
			typeName := typeExprName(field.Type)
			if matchSelector(typeName, idx, seg) {
				res.Node = field
				res.Field = field
				res.FieldIndex = idx
				return nil
			}
			idx++
		}
	}
	return fmt.Errorf("embedded type not found: %s", seg.Selector)
}

func resolveInterfaceEmbeds(res *Resolution, it *ast.InterfaceType, seg Segment) error {
	if it.Methods == nil {
		return fmt.Errorf("interface has no methods")
	}

	// Embedded interfaces have no names in the Methods list
	idx := 0
	for _, field := range it.Methods.List {
		if len(field.Names) == 0 {
			// This is an embedded interface (not a method signature)
			if _, ok := field.Type.(*ast.FuncType); !ok {
				typeName := typeExprName(field.Type)
				if matchSelector(typeName, idx, seg) {
					res.Node = field
					res.Field = field
					res.FieldIndex = idx
					return nil
				}
				idx++
			}
		}
	}
	return fmt.Errorf("embedded interface not found: %s", seg.Selector)
}

// resolveParams navigates to a function parameter.
func resolveParams(res *Resolution, seg Segment) error {
	ft, err := getFuncType(res.Node)
	if err != nil {
		return err
	}

	if ft.Params == nil {
		return fmt.Errorf("function has no parameters")
	}

	field, idx, err := selectField(ft.Params, seg.Selector, seg.IsIndex)
	if err != nil {
		return err
	}

	res.Node = field
	res.Field = field
	res.FieldIndex = idx
	return nil
}

// resolveReturns navigates to a function return value.
func resolveReturns(res *Resolution, seg Segment) error {
	ft, err := getFuncType(res.Node)
	if err != nil {
		return err
	}

	if ft.Results == nil {
		return fmt.Errorf("function has no return values")
	}

	field, idx, err := selectField(ft.Results, seg.Selector, seg.IsIndex)
	if err != nil {
		return err
	}

	res.Node = field
	res.Field = field
	res.FieldIndex = idx
	return nil
}

// resolveReceiver navigates to a method receiver.
func resolveReceiver(res *Resolution) error {
	fd, ok := res.Node.(*ast.FuncDecl)
	if !ok {
		return fmt.Errorf("receiver requires a method declaration")
	}

	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fmt.Errorf("not a method (no receiver)")
	}

	res.Node = fd.Recv.List[0]
	res.Field = fd.Recv.List[0]
	res.FieldIndex = 0
	return nil
}

// resolveTypeParams navigates to a type parameter.
func resolveTypeParams(res *Resolution, seg Segment) error {
	var typeParams *ast.FieldList

	switch node := res.Node.(type) {
	case *ast.FuncDecl:
		if node.Type.TypeParams != nil {
			typeParams = node.Type.TypeParams
		}
	case *ast.GenDecl:
		if len(node.Specs) > 0 {
			if ts, ok := node.Specs[0].(*ast.TypeSpec); ok && ts.TypeParams != nil {
				typeParams = ts.TypeParams
			}
		}
	case *ast.TypeSpec:
		if node.TypeParams != nil {
			typeParams = node.TypeParams
		}
	}

	if typeParams == nil {
		return fmt.Errorf("no type parameters")
	}

	field, idx, err := selectField(typeParams, seg.Selector, seg.IsIndex)
	if err != nil {
		return err
	}

	res.Node = field
	res.Field = field
	res.FieldIndex = idx
	return nil
}

// resolveBody navigates to a function body.
func resolveBody(res *Resolution) error {
	fd, ok := res.Node.(*ast.FuncDecl)
	if !ok {
		return fmt.Errorf("body requires a function declaration")
	}

	if fd.Body == nil {
		return fmt.Errorf("function has no body")
	}

	res.Node = fd.Body
	res.Field = nil
	res.FieldIndex = -1
	return nil
}

// resolveDoc navigates to a doc comment.
func resolveDoc(res *Resolution) error {
	var doc *ast.CommentGroup

	switch node := res.Node.(type) {
	case *ast.FuncDecl:
		doc = node.Doc
	case *ast.GenDecl:
		doc = node.Doc
	case *ast.TypeSpec:
		doc = node.Doc
	case *ast.Field:
		doc = node.Doc
	case *ast.ValueSpec:
		doc = node.Doc
	}

	if doc == nil {
		return fmt.Errorf("no doc comment")
	}

	res.Node = doc
	res.Field = nil
	res.FieldIndex = -1
	return nil
}

// resolveTag navigates to a struct field tag.
func resolveTag(res *Resolution, seg Segment) error {
	field, ok := res.Node.(*ast.Field)
	if !ok {
		return fmt.Errorf("tag requires a struct field")
	}

	if field.Tag == nil {
		return fmt.Errorf("field has no tag")
	}

	// If selector is provided, we're accessing a specific tag key
	// For now, we just return the whole tag literal
	res.Node = field.Tag
	res.Field = nil
	res.FieldIndex = -1
	return nil
}

// resolveType navigates to the type of a field/param/var.
func resolveType(res *Resolution) error {
	switch node := res.Node.(type) {
	case *ast.Field:
		if node.Type != nil {
			res.Node = node.Type
			res.Field = nil
			res.FieldIndex = -1
			return nil
		}
	case *ast.ValueSpec:
		if node.Type != nil {
			res.Node = node.Type
			res.Field = nil
			res.FieldIndex = -1
			return nil
		}
	}
	return fmt.Errorf("no type information")
}

// resolveName navigates to the name of a field/param/receiver.
func resolveName(res *Resolution) error {
	field, ok := res.Node.(*ast.Field)
	if !ok {
		return fmt.Errorf("name requires a field")
	}

	if len(field.Names) == 0 {
		return fmt.Errorf("field has no name (anonymous)")
	}

	// Return the first name (for grouped fields, we've already selected the right one)
	res.Node = field.Names[0]
	res.Field = nil
	res.FieldIndex = -1
	return nil
}

// resolveConstraint navigates to a type parameter constraint.
func resolveConstraint(res *Resolution) error {
	field, ok := res.Node.(*ast.Field)
	if !ok {
		return fmt.Errorf("constraint requires a type parameter")
	}

	if field.Type == nil {
		return fmt.Errorf("type parameter has no constraint")
	}

	res.Node = field.Type
	res.Field = nil
	res.FieldIndex = -1
	return nil
}

// resolveValue navigates to a const/var initializer.
func resolveValue(res *Resolution) error {
	switch node := res.Node.(type) {
	case *ast.GenDecl:
		if len(node.Specs) > 0 {
			if vs, ok := node.Specs[0].(*ast.ValueSpec); ok {
				if len(vs.Values) > 0 {
					res.Node = vs.Values[0]
					res.Field = nil
					res.FieldIndex = -1
					return nil
				}
			}
		}
	case *ast.ValueSpec:
		if len(node.Values) > 0 {
			res.Node = node.Values[0]
			res.Field = nil
			res.FieldIndex = -1
			return nil
		}
	}
	return fmt.Errorf("no initializer value")
}

// Helper functions

func getStructType(node ast.Node) (*ast.StructType, error) {
	switch n := node.(type) {
	case *ast.GenDecl:
		if len(n.Specs) > 0 {
			if ts, ok := n.Specs[0].(*ast.TypeSpec); ok {
				if st, ok := ts.Type.(*ast.StructType); ok {
					return st, nil
				}
			}
		}
	case *ast.TypeSpec:
		if st, ok := n.Type.(*ast.StructType); ok {
			return st, nil
		}
	case *ast.StructType:
		return n, nil
	}
	return nil, fmt.Errorf("not a struct type")
}

func getInterfaceType(node ast.Node) (*ast.InterfaceType, error) {
	switch n := node.(type) {
	case *ast.GenDecl:
		if len(n.Specs) > 0 {
			if ts, ok := n.Specs[0].(*ast.TypeSpec); ok {
				if it, ok := ts.Type.(*ast.InterfaceType); ok {
					return it, nil
				}
			}
		}
	case *ast.TypeSpec:
		if it, ok := n.Type.(*ast.InterfaceType); ok {
			return it, nil
		}
	case *ast.InterfaceType:
		return n, nil
	}
	return nil, fmt.Errorf("not an interface type")
}

func getFuncType(node ast.Node) (*ast.FuncType, error) {
	switch n := node.(type) {
	case *ast.FuncDecl:
		return n.Type, nil
	case *ast.FuncType:
		return n, nil
	case *ast.Field:
		// Interface method signature
		if ft, ok := n.Type.(*ast.FuncType); ok {
			return ft, nil
		}
	}
	return nil, fmt.Errorf("not a function")
}

// selectField selects a field from a field list by name or index.
// Returns the field, the flattened index, and any error.
func selectField(fl *ast.FieldList, selector string, isIndex bool) (*ast.Field, int, error) {
	if fl == nil {
		return nil, -1, fmt.Errorf("no fields")
	}

	if isIndex {
		idx, _ := strconv.Atoi(selector)
		return selectFieldByIndex(fl, idx)
	}
	return selectFieldByName(fl, selector)
}

func selectFieldByIndex(fl *ast.FieldList, targetIdx int) (*ast.Field, int, error) {
	idx := 0
	for _, field := range fl.List {
		if len(field.Names) == 0 {
			// Anonymous field/param
			if idx == targetIdx {
				return field, idx, nil
			}
			idx++
		} else {
			for range field.Names {
				if idx == targetIdx {
					return field, idx, nil
				}
				idx++
			}
		}
	}
	return nil, -1, fmt.Errorf("index %d out of range (have %d)", targetIdx, idx)
}

func selectFieldByName(fl *ast.FieldList, name string) (*ast.Field, int, error) {
	idx := 0
	for _, field := range fl.List {
		if len(field.Names) == 0 {
			// Anonymous - check type name
			typeName := typeExprName(field.Type)
			if typeName == name {
				return field, idx, nil
			}
			idx++
		} else {
			for _, ident := range field.Names {
				if ident.Name == name {
					return field, idx, nil
				}
				idx++
			}
		}
	}
	return nil, -1, fmt.Errorf("name not found: %s", name)
}

func matchSelector(name string, idx int, seg Segment) bool {
	if seg.IsIndex {
		targetIdx, _ := strconv.Atoi(seg.Selector)
		return idx == targetIdx
	}
	return name == seg.Selector
}

// typeExprName extracts a readable name from a type expression.
func typeExprName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.StarExpr:
		return typeExprName(t.X)
	case *ast.IndexExpr:
		return typeExprName(t.X)
	case *ast.IndexListExpr:
		return typeExprName(t.X)
	default:
		return ""
	}
}
