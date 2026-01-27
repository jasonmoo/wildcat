// Package spath provides semantic paths for addressing Go code elements.
//
// A semantic path identifies code by meaning rather than location:
//
//	encoding/json.Marshal/params[v]     // the v parameter of json.Marshal
//	github.com/user/repo.User/fields[ID] // the ID field of User struct
//
// Paths have three components:
//   - Package: full import path (e.g., "encoding/json")
//   - Symbol: top-level declaration, optionally with method (e.g., "Marshal", "User.GetID")
//   - Subpath: structural navigation (e.g., "/params[v]", "/fields[ID]/tag")
//
// See docs/path-syntax-grammar.ebnf for the formal grammar.
package spath

import "strings"

// Path represents a parsed semantic path.
type Path struct {
	// Package is the full import path (canonical form).
	// Examples: "encoding/json", "github.com/user/repo/internal/pkg"
	Package string

	// Symbol is the top-level declaration name.
	// Examples: "Marshal", "User", "ErrNotFound"
	Symbol string

	// Method is the method name, if this path refers to a method.
	// Empty for non-method symbols.
	// Example: for "pkg.User.GetID", Symbol="User" and Method="GetID"
	Method string

	// Segments are the structural navigation components after the symbol.
	// Example: for "pkg.Func/params[ctx]/type", Segments contains
	// {Category:"params", Selector:"ctx"} and {Category:"type", Selector:""}
	Segments []Segment
}

// Segment represents one structural navigation component (e.g., "/params[ctx]").
type Segment struct {
	// Category is the structural component type.
	// Valid values: fields, methods, embeds, params, returns, receiver,
	// typeparams, body, doc, tag, type, name, constraint, value
	Category string

	// Selector identifies which element within the category.
	// Can be a name (e.g., "ctx") or position (e.g., "0").
	// Empty for categories that don't need selection (e.g., "body", "doc").
	Selector string

	// IsIndex is true if Selector is a numeric index (positional access).
	IsIndex bool
}

// String returns the canonical string representation of the path.
func (p *Path) String() string {
	var b strings.Builder

	// Package.Symbol or Package.Symbol.Method
	b.WriteString(p.Package)
	b.WriteByte('.')
	b.WriteString(p.Symbol)
	if p.Method != "" {
		b.WriteByte('.')
		b.WriteString(p.Method)
	}

	// Subpath segments
	for _, seg := range p.Segments {
		b.WriteByte('/')
		b.WriteString(seg.Category)
		if seg.Selector != "" {
			b.WriteByte('[')
			b.WriteString(seg.Selector)
			b.WriteByte(']')
		}
	}

	return b.String()
}

// IsMethod returns true if this path refers to a method.
func (p *Path) IsMethod() bool {
	return p.Method != ""
}

// HasSubpath returns true if this path has structural navigation.
func (p *Path) HasSubpath() bool {
	return len(p.Segments) > 0
}

// Base returns a new Path with only Package, Symbol, and Method (no segments).
func (p *Path) Base() *Path {
	return &Path{
		Package: p.Package,
		Symbol:  p.Symbol,
		Method:  p.Method,
	}
}

// WithSegment returns a new Path with an additional segment appended.
func (p *Path) WithSegment(category, selector string, isIndex bool) *Path {
	newPath := &Path{
		Package:  p.Package,
		Symbol:   p.Symbol,
		Method:   p.Method,
		Segments: make([]Segment, len(p.Segments)+1),
	}
	copy(newPath.Segments, p.Segments)
	newPath.Segments[len(p.Segments)] = Segment{
		Category: category,
		Selector: selector,
		IsIndex:  isIndex,
	}
	return newPath
}

// SymbolQuery returns the symbol query string for Index.Lookup.
// Format: "Package.Symbol" or "Package.Symbol.Method"
func (p *Path) SymbolQuery() string {
	if p.Method != "" {
		return p.Package + "." + p.Symbol + "." + p.Method
	}
	return p.Package + "." + p.Symbol
}

// Valid category names for segments.
var ValidCategories = map[string]bool{
	"fields":     true,
	"methods":    true,
	"embeds":     true,
	"params":     true,
	"returns":    true,
	"receiver":   true,
	"typeparams": true,
	"body":       true,
	"doc":        true,
	"tag":        true,
	"type":       true,
	"name":       true,
	"constraint": true,
	"value":      true,
}
