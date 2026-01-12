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
	Symbol   string   `json:"symbol"`
	Package  string   `json:"package,omitempty"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	LineEnd  int      `json:"line_end,omitempty"`
	Snippet  string   `json:"snippet,omitempty"`
	CallExpr string   `json:"call_expr,omitempty"`
	Args     []string `json:"args,omitempty"`
	InTest   bool     `json:"in_test"`
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
	Query          QueryInfo  `json:"query"`
	Target         TargetInfo `json:"target"`
	Results        []Result   `json:"results"`
	Summary        Summary    `json:"summary"`
	SimilarSymbols []string   `json:"similar_symbols,omitempty"`
}

// CalleesResponse is the output for the callees command.
type CalleesResponse struct {
	Query          QueryInfo  `json:"query"`
	Target         TargetInfo `json:"target"`
	Results        []Result   `json:"results"`
	Summary        Summary    `json:"summary"`
	SimilarSymbols []string   `json:"similar_symbols,omitempty"`
}

// RefsResponse is the output for the refs command.
type RefsResponse struct {
	Query          QueryInfo  `json:"query"`
	Target         TargetInfo `json:"target"`
	Results        []Result   `json:"results"`
	Summary        Summary    `json:"summary"`
	SimilarSymbols []string   `json:"similar_symbols,omitempty"`
}

// TreeNode represents a node in the call tree.
type TreeNode struct {
	File      string   `json:"file"`
	Line      int      `json:"line"`
	Signature string   `json:"signature,omitempty"`
	Calls     []string `json:"calls,omitempty"`
	CalledBy  []string `json:"called_by,omitempty"`
}

// TreeEdge represents an edge in the call tree.
type TreeEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	File string `json:"file"`
	Line int    `json:"line"`
}

// TreeSummary provides aggregate information about the tree.
type TreeSummary struct {
	NodeCount       int  `json:"node_count"`
	EdgeCount       int  `json:"edge_count"`
	MaxDepthReached int  `json:"max_depth_reached"`
	Truncated       bool `json:"truncated"`
}

// TreeQuery describes the tree query parameters.
type TreeQuery struct {
	Command   string `json:"command"`
	Root      string `json:"root"`
	Depth     int    `json:"depth"`
	Direction string `json:"direction"`
}

// TreeResponse is the output for the tree command.
type TreeResponse struct {
	Query   TreeQuery           `json:"query"`
	Nodes   map[string]TreeNode `json:"nodes"`
	Edges   []TreeEdge          `json:"edges"`
	Summary TreeSummary         `json:"summary"`
}

// ImpactCategory represents a category of impact.
type ImpactCategory struct {
	Symbol  string `json:"symbol"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Reason  string `json:"reason,omitempty"`
	Snippet string `json:"snippet,omitempty"`
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
	Query          QueryInfo     `json:"query"`
	Target         TargetInfo    `json:"target"`
	Impact         Impact        `json:"impact"`
	Summary        ImpactSummary `json:"summary"`
	SimilarSymbols []string      `json:"similar_symbols,omitempty"`
}

// ImplementsResponse is the output for the implements command.
type ImplementsResponse struct {
	Query           QueryInfo  `json:"query"`
	Interface       TargetInfo `json:"interface"`
	Implementations []Result   `json:"implementations"`
	Summary         Summary    `json:"summary"`
}

// SatisfiesResponse is the output for the satisfies command.
type SatisfiesResponse struct {
	Query      QueryInfo         `json:"query"`
	Type       TargetInfo        `json:"type"`
	Interfaces []InterfaceResult `json:"interfaces"`
	Summary    Summary           `json:"summary"`
}

// InterfaceResult represents an interface that a type satisfies.
type InterfaceResult struct {
	Symbol  string   `json:"symbol"`
	File    string   `json:"file"`
	Line    int      `json:"line"`
	Methods []string `json:"methods,omitempty"`
}

// DepsResponse is the output for the deps command.
type DepsResponse struct {
	Query        QueryInfo    `json:"query"`
	Package      string       `json:"package"`
	Direction    string       `json:"direction"`
	Dependencies []DepResult  `json:"dependencies"`
	Summary      Summary      `json:"summary"`
}

// DepResult represents a package dependency.
type DepResult struct {
	Package    string `json:"package"`
	ImportFile string `json:"import_file"`
	ImportLine int    `json:"import_line"`
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
