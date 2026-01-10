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
	Query   QueryInfo  `json:"query"`
	Target  TargetInfo `json:"target"`
	Results []Result   `json:"results"`
	Summary Summary    `json:"summary"`
}

// CalleesResponse is the output for the callees command.
type CalleesResponse struct {
	Query   QueryInfo  `json:"query"`
	Target  TargetInfo `json:"target"`
	Results []Result   `json:"results"`
	Summary Summary    `json:"summary"`
}

// RefsResponse is the output for the refs command.
type RefsResponse struct {
	Query   QueryInfo  `json:"query"`
	Target  TargetInfo `json:"target"`
	Results []Result   `json:"results"`
	Summary Summary    `json:"summary"`
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
	Symbol string `json:"symbol"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Reason string `json:"reason,omitempty"`
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
	Query   QueryInfo     `json:"query"`
	Target  TargetInfo    `json:"target"`
	Impact  Impact        `json:"impact"`
	Summary ImpactSummary `json:"summary"`
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
