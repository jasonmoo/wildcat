// Package output provides types and utilities for Wildcat's JSON output.
package output

// QueryInfo describes the query that was executed.
type QueryInfo struct {
	Command       string         `json:"command"`
	Target        string         `json:"target"`
	Resolved      string         `json:"resolved,omitempty"`
	Scope         string         `json:"scope,omitempty"`
	ScopeResolved *ScopeResolved `json:"scope_resolved,omitempty"`
}

// ScopeResolved contains the packages that were actually examined after
// resolving scope patterns. This provides transparency about what was
// included/excluded, so there's no ambiguity about what was searched.
type ScopeResolved struct {
	Includes []string `json:"includes,omitempty"` // packages included in scope
	Excludes []string `json:"excludes,omitempty"` // packages explicitly excluded
}

// TargetInfo describes the target symbol.
// TargetRefs contains reference counts for a target symbol.
type TargetRefs struct {
	Internal int `json:"internal"` // references from same package
	External int `json:"external"` // references from other packages
	Packages int `json:"packages"` // number of external packages
}

type TargetInfo struct {
	Symbol     string     `json:"symbol"`
	Kind       string     `json:"-"` // func, method, type, interface, const, var (for markdown logic)
	Signature  string     `json:"signature,omitempty"`
	Definition string     `json:"definition"` // path:start:end
	Refs       TargetRefs `json:"refs"`
}

// TreeSummary provides aggregate information about the tree.
type TreeSummary struct {
	Callers       int  `json:"callers"`                  // total caller edges
	Callees       int  `json:"callees"`                  // total callee edges
	MaxUpDepth    int  `json:"max_up_depth,omitempty"`   // deepest caller level reached
	MaxDownDepth  int  `json:"max_down_depth,omitempty"` // deepest callee level reached
	UpTruncated   bool `json:"up_truncated"`             // hit up depth limit
	DownTruncated bool `json:"down_truncated"`           // hit down depth limit
}

// TreeQuery describes the tree query parameters.
type TreeQuery struct {
	Command string `json:"command"`
	Target  string `json:"target"`
	Up      int    `json:"up"`    // caller depth requested
	Down    int    `json:"down"`  // callee depth requested
	Scope   string `json:"scope"` // all, project, or package
}

// CallNode represents a node in a call chain.
type CallNode struct {
	Symbol   string      `json:"symbol"`             // qualified: pkg.Name
	Callsite string      `json:"callsite,omitempty"` // where called: /full/path/file.go:line (empty for entry points)
	Calls    []*CallNode `json:"calls,omitempty"`    // next in chain (toward target for callers, away from target for callees)
	Error    string      `json:"error,omitempty"`    // if set, analysis couldn't continue past this node
}

// TreeFunction contains information about a function in the call tree.
type TreeFunction struct {
	Symbol     string `json:"symbol"` // qualified: pkg.Name or pkg.Type.Method
	Signature  string `json:"signature"`
	Definition string `json:"definition"` // file:start:end (full function range)
}

// TreePackage groups functions by package in tree output.
type TreePackage struct {
	Package string         `json:"package"`
	Dir     string         `json:"dir"`
	Symbols []TreeFunction `json:"symbols"`
}

// TreeTargetInfo describes the target for tree command output.
type TreeTargetInfo struct {
	Symbol     string `json:"symbol"`
	Signature  string `json:"signature"`
	Definition string `json:"definition"` // path:start:end
}

// Snippet represents a code snippet with its location.
type Snippet struct {
	Location string `json:"location"` // "file.go:start:end"
	Source   string `json:"source"`
}

// Location represents a reference location within a package.
type Location struct {
	Location string      `json:"location"` // "file.go:line" or "file.go:line1,line2" when merged
	Symbol   string      `json:"symbol"`
	Snippet  Snippet     `json:"snippet"`
	RefCount int         `json:"ref_count,omitempty"` // number of refs merged into this location (0 or 1 = single ref)
	Refs     *TargetRefs `json:"refs,omitempty"`      // reference counts for the referencing symbol
}

// PackageUsage contains callers and references within a single package.
type PackageUsage struct {
	Package    string     `json:"package"`
	Dir        string     `json:"dir"`
	Callers    []Location `json:"callers"`
	References []Location `json:"references"`
}

// SymbolSummary provides aggregate information about symbol usage.
type SymbolSummary struct {
	Callers         int `json:"callers"`
	References      int `json:"references"`
	Implementations int `json:"implementations"`
	Satisfies       int `json:"satisfies"`
	InTests         int `json:"in_tests"`
}

// DepResult represents a package dependency.
type DepResult struct {
	Package  string `json:"package"`
	Location string `json:"location,omitempty"` // file:line where import occurs
}

// SearchQuery describes a search query.
type SearchQuery struct {
	Command       string         `json:"command"`
	Pattern       string         `json:"pattern"`
	Mode          string         `json:"mode,omitempty"` // "fuzzy" or "regex"
	Scope         string         `json:"scope,omitempty"`
	ScopeResolved *ScopeResolved `json:"scope_resolved,omitempty"`
	Kind          string         `json:"kind,omitempty"`
}

// SearchSummary provides aggregate information about search results.
type SearchSummary struct {
	Count     int            `json:"count"`
	ByKind    map[string]int `json:"by_kind,omitempty"`
	Truncated bool           `json:"truncated"`
}

// FileInfo describes a source file in a package.
type FileInfo struct {
	Name       string      `json:"name"`                 // base filename
	LineCount  int         `json:"line_count"`           // total lines in file
	Exported   int         `json:"exported,omitempty"`   // exported symbols defined in file
	Unexported int         `json:"unexported,omitempty"` // unexported symbols defined in file
	Refs       *TargetRefs `json:"refs,omitempty"`       // aggregate refs to all symbols in file
}

// PackageInfo describes the package.
type PackageInfo struct {
	ImportPath string `json:"import_path"`
	Name       string `json:"name"`
	Dir        string `json:"dir"`
}

// PackageSymbol represents a symbol in a package.
type PackageSymbol struct {
	Signature string      `json:"signature"`
	Location  string      `json:"location,omitempty"` // file:line[:line_end] - omitted for fields
	Refs      *TargetRefs `json:"refs,omitempty"`
}

// PackageType represents a type with its functions and methods.
type PackageType struct {
	Signature     string          `json:"signature"`
	Location      string          `json:"location"`
	Refs          *TargetRefs     `json:"refs,omitempty"`
	Functions     []PackageSymbol `json:"functions,omitempty"`
	Methods       []PackageSymbol `json:"methods,omitempty"`
	Satisfies     []string        `json:"satisfies,omitempty"`      // interfaces this type implements
	ImplementedBy []string        `json:"implemented_by,omitempty"` // types implementing this interface
}

// PackageSummary provides counts for the package command.
type PackageSummary struct {
	Constants  int `json:"constants"`
	Variables  int `json:"variables"`
	Functions  int `json:"functions"`
	Types      int `json:"types"`
	Methods    int `json:"methods"`
	Imports    int `json:"imports"`
	ImportedBy int `json:"imported_by"`
}
