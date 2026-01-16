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
	Symbol    string `json:"symbol"`
	Kind      string `json:"kind,omitempty"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	LineEnd   int    `json:"line_end,omitempty"`
	Signature string `json:"signature,omitempty"`
}

// Result represents a single result item (caller, callee, reference, etc.).
type Result struct {
	Symbol       string   `json:"symbol,omitempty"`
	Package      string   `json:"package,omitempty"`
	File         string   `json:"file"`
	Line         int      `json:"line,omitempty"`          // Reference line (omitted when Lines is set)
	Lines        []int    `json:"lines,omitempty"`         // All reference lines when merged
	LineEnd      int      `json:"line_end,omitempty"`
	Snippet      string   `json:"snippet,omitempty"`
	SnippetStart int      `json:"snippet_start,omitempty"` // First line of snippet
	SnippetEnd   int      `json:"snippet_end,omitempty"`   // Last line of snippet
	CallExpr     string   `json:"call_expr,omitempty"`
	Args         []string `json:"args,omitempty"`
	InTest       bool     `json:"in_test,omitempty"`
}

// Summary provides aggregate information about the results.
type Summary struct {
	Count     int      `json:"count"`
	Packages  []string `json:"packages,omitempty"`
	InTests   int      `json:"in_tests"`
	Truncated bool     `json:"truncated"`
}

// TreeSummary provides aggregate information about the tree.
type TreeSummary struct {
	PathCount       int  `json:"path_count"`
	MaxDepthReached int  `json:"max_depth_reached"`
	Truncated       bool `json:"truncated"`
}

// TreeQuery describes the tree query parameters.
type TreeQuery struct {
	Command   string `json:"command"`
	Target    string `json:"target"`
	Depth     int    `json:"depth"`
	Direction string `json:"direction"`
}

// TreeFunction contains information about a function in the call tree.
type TreeFunction struct {
	Signature string `json:"signature"`
	Location  string `json:"location"` // file:start:end
}

// TreeResponse is the output for the tree command.
type TreeResponse struct {
	Query     TreeQuery               `json:"query"`
	Paths     [][]string              `json:"paths"`
	Functions map[string]TreeFunction `json:"functions"`
	Summary   TreeSummary             `json:"summary"`
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
	ImportedBy        []string         `json:"imported_by"`
	Packages          []PackageUsage   `json:"packages"`
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
}

// SearchResult represents a single search result.
type SearchResult struct {
	Symbol   string `json:"symbol"`
	Kind     string `json:"kind"`
	Location string `json:"location"` // file:line:line_end
	Package  string `json:"package,omitempty"`
}

// SearchSummary provides aggregate information about search results.
type SearchSummary struct {
	Count     int            `json:"count"`
	ByKind    map[string]int `json:"by_kind,omitempty"`
	Truncated bool           `json:"truncated"`
}

// SearchResponse is the output for the search command.
type SearchResponse struct {
	Query   SearchQuery    `json:"query"`
	Results []SearchResult `json:"results"`
	Summary SearchSummary  `json:"summary"`
}

// PackageResponse is the output for the package command.
type PackageResponse struct {
	Query      QueryInfo          `json:"query"`
	Package    PackageInfo        `json:"package"`
	Constants  []PackageSymbol    `json:"constants,omitempty"`
	Variables  []PackageSymbol    `json:"variables,omitempty"`
	Functions  []PackageSymbol    `json:"functions,omitempty"`
	Types      []PackageType      `json:"types,omitempty"`
	Imports    []DepResult        `json:"imports"`
	ImportedBy []DepResult        `json:"imported_by"`
	Summary    PackageSummary     `json:"summary"`
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
	Constants   int            `json:"constants"`
	Variables   int            `json:"variables"`
	Functions   int            `json:"functions"`
	Types       int            `json:"types"`
	Methods     int            `json:"methods"`
	Imports     int            `json:"imports"`
	ImportedBy  int            `json:"imported_by"`
}
