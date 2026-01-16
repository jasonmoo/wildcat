// Package output provides types and utilities for Wildcat's JSON output.
package output

// QueryInfo describes the query that was executed.
type QueryInfo struct {
	Command  string `json:"command"`
	Target   string `json:"target"`
	Resolved string `json:"resolved,omitempty"`
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

// CallersResponse is the output for the callers command.
type CallersResponse struct {
	Query             QueryInfo  `json:"query"`
	Target            TargetInfo `json:"target,omitempty"`
	Results           []Result   `json:"results,omitempty"`
	Summary           Summary    `json:"summary,omitempty"`
	OtherFuzzyMatches []string   `json:"other_fuzzy_matches,omitempty"`
	Error             string     `json:"error,omitempty"` // populated on error in multi-symbol queries
}

// CalleesResponse is the output for the callees command.
type CalleesResponse struct {
	Query             QueryInfo  `json:"query"`
	Target            TargetInfo `json:"target,omitempty"`
	Results           []Result   `json:"results,omitempty"`
	Summary           Summary    `json:"summary,omitempty"`
	OtherFuzzyMatches []string   `json:"other_fuzzy_matches,omitempty"`
	Error             string     `json:"error,omitempty"` // populated on error in multi-symbol queries
}

// RefsResponse is the output for the refs command.
type RefsResponse struct {
	Query             QueryInfo  `json:"query"`
	Target            TargetInfo `json:"target,omitempty"`
	Results           []Result   `json:"results,omitempty"`
	Summary           Summary    `json:"summary,omitempty"`
	OtherFuzzyMatches []string   `json:"other_fuzzy_matches,omitempty"`
	Error             string     `json:"error,omitempty"` // populated on error in multi-symbol queries
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

// SymbolLocation represents a location where a symbol is used.
type SymbolLocation struct {
	Symbol       string `json:"symbol,omitempty"`
	File         string `json:"file"`
	Line         int    `json:"line"`
	Reason       string `json:"reason,omitempty"`
	Snippet      string `json:"snippet,omitempty"`
	SnippetStart int    `json:"snippet_start,omitempty"`
	SnippetEnd   int    `json:"snippet_end,omitempty"`
}

// SymbolDependent represents a dependent package.
type SymbolDependent struct {
	Package    string `json:"package"`
	ImportLine int    `json:"import_line"`
	File       string `json:"file"`
}

// SymbolUsage contains all usage information for a symbol.
type SymbolUsage struct {
	Callers         []SymbolLocation  `json:"callers,omitempty"`
	References      []SymbolLocation  `json:"references,omitempty"`
	Implementations []SymbolLocation  `json:"implementations,omitempty"`
	Dependents      []SymbolDependent `json:"dependents,omitempty"`
}

// SymbolSummary provides aggregate information about symbol usage.
type SymbolSummary struct {
	TotalLocations    int `json:"total_locations"`
	Callers           int `json:"callers"`
	References        int `json:"references"`
	Implementations   int `json:"implementations"`
	DependentPackages int `json:"dependent_packages"`
	InTests           int `json:"in_tests"`
}

// SymbolResponse is the output for the symbol command.
type SymbolResponse struct {
	Query             QueryInfo     `json:"query"`
	Target            TargetInfo    `json:"target,omitempty"`
	Usage             SymbolUsage   `json:"usage,omitempty"`
	Summary           SymbolSummary `json:"summary,omitempty"`
	OtherFuzzyMatches []string      `json:"other_fuzzy_matches,omitempty"`
	Error             string        `json:"error,omitempty"` // populated on error in multi-symbol queries
}

// ImplementsResponse is the output for the implements command.
type ImplementsResponse struct {
	Query           QueryInfo  `json:"query"`
	Interface       TargetInfo `json:"interface,omitempty"`
	Implementations []Result   `json:"implementations,omitempty"`
	Summary         Summary    `json:"summary,omitempty"`
	Error           string     `json:"error,omitempty"` // populated on error in multi-symbol queries
}

// SatisfiesResponse is the output for the satisfies command.
type SatisfiesResponse struct {
	Query      QueryInfo         `json:"query"`
	Type       TargetInfo        `json:"type,omitempty"`
	Interfaces []InterfaceResult `json:"interfaces,omitempty"`
	Summary    Summary           `json:"summary,omitempty"`
	Error      string            `json:"error,omitempty"` // populated on error in multi-symbol queries
}

// InterfaceResult represents an interface that a type satisfies.
type InterfaceResult struct {
	Symbol       string   `json:"symbol"`
	File         string   `json:"file"`
	Line         int      `json:"line"`
	Methods      []string `json:"methods,omitempty"`
	Snippet      string   `json:"snippet,omitempty"`
	SnippetStart int      `json:"snippet_start,omitempty"`
	SnippetEnd   int      `json:"snippet_end,omitempty"`
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
