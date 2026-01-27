package spath

import (
	"github.com/jasonmoo/wildcat/internal/golang"
)

// Generate creates a canonical path string for a symbol.
//
// This is the reverse of Parse+Resolve: given a symbol, produce the
// canonical path that would resolve back to it.
//
// The generated path uses the full import path (PkgPath) for the package,
// ensuring the path is unambiguous and portable.
func Generate(sym *golang.Symbol) string {
	path := &Path{
		Package: sym.PackageIdentifier.PkgPath,
		Symbol:  sym.Name,
	}

	// For methods, add the receiver type
	if sym.Kind == golang.SymbolKindMethod && sym.Parent != nil {
		path.Symbol = sym.Parent.Name
		path.Method = sym.Name
	}

	return path.String()
}

// GeneratePath creates a Path struct for a symbol.
//
// Unlike Generate which returns a string, this returns the structured
// Path that can be extended with additional segments.
func GeneratePath(sym *golang.Symbol) *Path {
	path := &Path{
		Package: sym.PackageIdentifier.PkgPath,
		Symbol:  sym.Name,
	}

	if sym.Kind == golang.SymbolKindMethod && sym.Parent != nil {
		path.Symbol = sym.Parent.Name
		path.Method = sym.Name
	}

	return path
}

// GenerateField creates a path for a struct field.
func GenerateField(sym *golang.Symbol, fieldName string) string {
	path := GeneratePath(sym)
	return path.WithSegment("fields", fieldName, false).String()
}

// GenerateParam creates a path for a function parameter.
func GenerateParam(sym *golang.Symbol, paramName string) string {
	path := GeneratePath(sym)
	return path.WithSegment("params", paramName, false).String()
}

// GenerateReturn creates a path for a function return value.
// Uses positional index since return values are often unnamed.
func GenerateReturn(sym *golang.Symbol, index int) string {
	path := GeneratePath(sym)
	return path.WithSegment("returns", itoa(index), true).String()
}

// GenerateReceiver creates a path for a method receiver.
func GenerateReceiver(sym *golang.Symbol) string {
	path := GeneratePath(sym)
	return path.WithSegment("receiver", "", false).String()
}

// GenerateBody creates a path for a function body.
func GenerateBody(sym *golang.Symbol) string {
	path := GeneratePath(sym)
	return path.WithSegment("body", "", false).String()
}

// GenerateDoc creates a path for a symbol's doc comment.
func GenerateDoc(sym *golang.Symbol) string {
	path := GeneratePath(sym)
	return path.WithSegment("doc", "", false).String()
}

// itoa converts int to string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
