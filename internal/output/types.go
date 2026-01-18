// Package output provides types and utilities for Wildcat's JSON output.
package output

// QueryInfo describes the query that was executed.
type QueryInfo struct {
	Command  string `json:"command"`
	Target   string `json:"target"`
	Resolved string `json:"resolved,omitempty"`
	Scope    string `json:"scope,omitempty"`
}

// TargetInfo describes the target symbol.
type TargetInfo struct {
	Symbol     string `json:"symbol"`
	Signature  string `json:"signature,omitempty"`
	Definition string `json:"definition"` // path:start:end
}

// TreeSummary provides aggregate information about the tree.
type TreeSummary struct {
	Callers       int  `json:"callers"`                  // total caller edges
	Callees       int  `json:"callees"`                  // total callee edges
	MaxUpDepth    int  `json:"max_up_depth,omitempty"`   // deepest caller level reached
	MaxDownDepth  int  `json:"max_down_depth,omitempty"` // deepest callee level reached
	UpTruncated   bool `json:"up_truncated,omitempty"`   // hit up depth limit
	DownTruncated bool `json:"down_truncated,omitempty"` // hit down depth limit
}

// TreeQuery describes the tree query parameters.
type TreeQuery struct {
	Command string `json:"command"`
	Target  string `json:"target"`
	Up      int    `json:"up"`   // caller depth requested
	Down    int    `json:"down"` // callee depth requested
}

// CallNode represents a node in a call chain.
type CallNode struct {
	Symbol   string      `json:"symbol"`             // qualified: pkg.Name
	Callsite string      `json:"callsite,omitempty"` // where called: /full/path/file.go:line (empty for entry points)
	Calls    []*CallNode `json:"calls,omitempty"`    // next in chain (toward target for callers, away from target for callees)
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

// TreeResponse is the output for the tree command.
type TreeResponse struct {
	Query       TreeQuery      `json:"query"`
	Target      TreeTargetInfo `json:"target"`
	Summary     TreeSummary    `json:"summary"`
	Callers     []*CallNode    `json:"callers"`
	Calls       []*CallNode    `json:"calls"`
	Definitions []TreePackage  `json:"definitions"`
}

// Snippet represents a code snippet with its location.
type Snippet struct {
	Location string `json:"location"` // "file.go:start:end"
	Source   string `json:"source"`
}

// Location represents a reference location within a package.
type Location struct {
	Location string  `json:"location"` // "file.go:line"
	Symbol   string  `json:"symbol"`
	Snippet  Snippet `json:"snippet"`
}

// PackageUsage contains callers and references within a single package.
type PackageUsage struct {
	Package    string     `json:"package"`
	Dir        string     `json:"dir"`
	Callers    []Location `json:"callers,omitempty"`
	References []Location `json:"references,omitempty"`
}

// SymbolLocation represents a location for cross-package type relationships.
// Used for implementations and satisfies which need full paths.
type SymbolLocation struct {
	Location string  `json:"location"` // full path: "/home/.../file.go:line"
	Symbol   string  `json:"symbol"`
	Snippet  Snippet `json:"snippet"`
}

// SymbolSummary provides aggregate information about symbol usage.
type SymbolSummary struct {
	Callers         int `json:"callers"`
	References      int `json:"references"`
	Implementations int `json:"implementations"`
	Satisfies       int `json:"satisfies"`
	InTests         int `json:"in_tests"`
}

// SymbolResponse is the output for the symbol command.
type SymbolResponse struct {
	Query             QueryInfo        `json:"query"`
	Target            TargetInfo       `json:"target"`
	ImportedBy        []DepResult      `json:"imported_by"`
	References        []PackageUsage   `json:"references"`
	Implementations   []SymbolLocation `json:"implementations,omitempty"`
	Satisfies         []SymbolLocation `json:"satisfies,omitempty"`
	QuerySummary      SymbolSummary    `json:"query_summary"`
	PackageSummary    SymbolSummary    `json:"package_summary"`
	ProjectSummary    SymbolSummary    `json:"project_summary"`
	OtherFuzzyMatches []string         `json:"other_fuzzy_matches"`
	Error             string           `json:"error,omitempty"`
}

// DepResult represents a package dependency.
type DepResult struct {
	Package  string `json:"package"`
	Location string `json:"location,omitempty"` // file:line where import occurs
}

// ErrorResponse is the output when an error occurs.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error information.
type ErrorDetail struct {
	Code        string         `json:"code"`
	Message     string         `json:"message"`
	Suggestions []string       `json:"suggestions,omitempty"`
	Context     map[string]any `json:"context,omitempty"`
}

// SearchQuery describes a search query.
type SearchQuery struct {
	Command string `json:"command"`
	Pattern string `json:"pattern"`
	Scope   string `json:"scope,omitempty"`
}

// SearchMatch represents a single symbol match within a package.
type SearchMatch struct {
	Location string   `json:"location"` // "file.go:line"
	Symbol   string   `json:"symbol"`   // short name: "Type.Method"
	Kind     string   `json:"kind"`
	Snippet  *Snippet `json:"snippet,omitempty"`
}

// SearchPackage groups search matches by package.
type SearchPackage struct {
	Package string        `json:"package"`
	Dir     string        `json:"dir"`
	Matches []SearchMatch `json:"matches"`
}

// SearchSummary provides aggregate information about search results.
type SearchSummary struct {
	Count     int            `json:"count"`
	ByKind    map[string]int `json:"by_kind,omitempty"`
	Truncated bool           `json:"truncated"`
}

// SearchResponse is the output for the search command.
type SearchResponse struct {
	Query    SearchQuery     `json:"query"`
	Packages []SearchPackage `json:"packages"`
	Summary  SearchSummary   `json:"summary"`
}

// PackageResponse is the output for the package command.
type PackageResponse struct {
	Query      QueryInfo       `json:"query"`
	Package    PackageInfo     `json:"package"`
	Summary    PackageSummary  `json:"summary"`
	Constants  []PackageSymbol `json:"constants,omitempty"`
	Variables  []PackageSymbol `json:"variables,omitempty"`
	Functions  []PackageSymbol `json:"functions,omitempty"`
	Types      []PackageType   `json:"types,omitempty"`
	Imports    []DepResult     `json:"imports"`
	ImportedBy []DepResult     `json:"imported_by"`
}

// PackageInfo describes the package.
type PackageInfo struct {
	ImportPath string `json:"import_path"`
	Name       string `json:"name"`
	Dir        string `json:"dir"`
}

// PackageSymbol represents a symbol in a package.
type PackageSymbol struct {
	Signature string `json:"signature"`
	Location  string `json:"location,omitempty"` // file:line[:line_end] - omitted for fields
}

// PackageType represents a type with its functions and methods.
type PackageType struct {
	Signature     string          `json:"signature"`
	Location      string          `json:"location"`
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
