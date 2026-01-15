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

// ImpactCategory represents a category of impact.
type ImpactCategory struct {
	Symbol       string `json:"symbol,omitempty"`
	File         string `json:"file"`
	Line         int    `json:"line"`
	Reason       string `json:"reason,omitempty"`
	Snippet      string `json:"snippet,omitempty"`
	SnippetStart int    `json:"snippet_start,omitempty"`
	SnippetEnd   int    `json:"snippet_end,omitempty"`
}

// ImpactDependent represents a dependent package.
type ImpactDependent struct {
	Package    string `json:"package"`
	ImportLine int    `json:"import_line"`
	File       string `json:"file"`
}

// Impact contains all impact information.
type Impact struct {
	Callers         []ImpactCategory  `json:"callers,omitempty"`
	References      []ImpactCategory  `json:"references,omitempty"`
	Implementations []ImpactCategory  `json:"implementations,omitempty"`
	Dependents      []ImpactDependent `json:"dependents,omitempty"`
}

// ImpactSummary provides aggregate information about impact.
type ImpactSummary struct {
	TotalLocations    int `json:"total_locations"`
	Callers           int `json:"callers"`
	References        int `json:"references"`
	Implementations   int `json:"implementations"`
	DependentPackages int `json:"dependent_packages"`
	InTests           int `json:"in_tests"`
}

// ImpactResponse is the output for the impact command.
type ImpactResponse struct {
	Query             QueryInfo     `json:"query"`
	Target            TargetInfo    `json:"target,omitempty"`
	Impact            Impact        `json:"impact,omitempty"`
	Summary           ImpactSummary `json:"summary,omitempty"`
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

// DepsResponse is the output for the deps command.
type DepsResponse struct {
	Query      QueryInfo   `json:"query"`
	Package    string      `json:"package"`
	Imports    []DepResult `json:"imports"`
	ImportedBy []DepResult `json:"imported_by"`
	Summary    DepsSummary `json:"summary"`
}

// DepsSummary provides counts for the deps command.
type DepsSummary struct {
	ImportsCount    int `json:"imports_count"`
	ImportedByCount int `json:"imported_by_count"`
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

// SymbolsQuery describes a symbols search query.
type SymbolsQuery struct {
	Command string `json:"command"`
	Pattern string `json:"pattern"`
}

// SymbolResult represents a single symbol search result.
type SymbolResult struct {
	Symbol   string `json:"symbol"`
	Kind     string `json:"kind"`
	Location string `json:"location"` // file:line:line_end
	Package  string `json:"package,omitempty"`
}

// SymbolsSummary provides aggregate information about symbol search results.
type SymbolsSummary struct {
	Count     int            `json:"count"`
	ByKind    map[string]int `json:"by_kind,omitempty"`
	Truncated bool           `json:"truncated"`
}

// SymbolsResponse is the output for the symbols command.
type SymbolsResponse struct {
	Query   SymbolsQuery   `json:"query"`
	Results []SymbolResult `json:"results"`
	Summary SymbolsSummary `json:"summary"`
}

// PackageResponse is the output for the package command.
type PackageResponse struct {
	Query      QueryInfo          `json:"query"`
	Package    PackageInfo        `json:"package"`
	Constants  []PackageSymbol    `json:"constants,omitempty"`
	Variables  []PackageSymbol    `json:"variables,omitempty"`
	Functions  []PackageSymbol    `json:"functions,omitempty"`
	Types      []PackageType      `json:"types,omitempty"`
	Imports    []string           `json:"imports"`
	ImportedBy []string           `json:"imported_by"`
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
