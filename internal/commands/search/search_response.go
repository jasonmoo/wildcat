package search_cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/output"
)

// SearchMatch is a single result with package info
type SearchMatch struct {
	Symbol     string `json:"symbol"`
	Kind       string `json:"kind"`
	Package    string `json:"package"`
	Definition string `json:"definition"`
	Signature  string `json:"signature,omitempty"`
}

type SearchCommandResponse struct {
	Query       output.SearchQuery      `json:"query"`
	Summary     output.SearchSummary    `json:"summary"`
	Results     []SearchMatch           `json:"results"`
	Diagnostics []commands.Diagnostics  `json:"diagnostics,omitempty"`
}

var _ commands.Result = (*SearchCommandResponse)(nil)

func (r *SearchCommandResponse) SetDiagnostics(ds []commands.Diagnostics) {
	r.Diagnostics = ds
}

func (r *SearchCommandResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Diagnostics []commands.Diagnostics `json:"diagnostics,omitempty"`
		Query       output.SearchQuery     `json:"query"`
		Summary     output.SearchSummary   `json:"summary"`
		Results     []SearchMatch          `json:"results"`
	}{
		Diagnostics: r.Diagnostics,
		Query:       r.Query,
		Summary:     r.Summary,
		Results:     r.Results,
	})
}

func (r *SearchCommandResponse) MarshalMarkdown() ([]byte, error) {
	var sb strings.Builder

	// Header
	sb.WriteString("# search ")
	sb.WriteString(r.Query.Pattern)
	sb.WriteString("\n")

	commands.FormatDiagnosticsMarkdown(&sb, r.Diagnostics)

	// Summary
	fmt.Fprintf(&sb, "\n## Summary (%d results)\n", r.Summary.Count)
	fmt.Fprintf(&sb, "mode: %s, scope: %s\n", r.Query.Mode, r.Query.Scope)
	if r.Query.ScopeResolved != nil && len(r.Query.ScopeResolved.Excludes) > 0 {
		fmt.Fprintf(&sb, "excluded: %s\n", strings.Join(r.Query.ScopeResolved.Excludes, ", "))
	}
	if r.Query.Kind != "" {
		fmt.Fprintf(&sb, "kind: %s\n", r.Query.Kind)
	}
	if len(r.Summary.ByKind) > 0 {
		sb.WriteString("by kind:")
		for k, v := range r.Summary.ByKind {
			fmt.Fprintf(&sb, " %s=%d", k, v)
		}
		sb.WriteString("\n")
	}

	// Flat results list by score
	fmt.Fprintf(&sb, "\n## Results (%d)\n\n", r.Summary.Count)
	for _, m := range r.Results {
		fmt.Fprintf(&sb, "- %s [%s]\n", m.Symbol, m.Kind)
		fmt.Fprintf(&sb, "  %s\n", m.Package)
		fmt.Fprintf(&sb, "  %s\n", m.Definition)
		if m.Signature != "" && m.Signature != m.Symbol {
			sb.WriteString("  ")
			sb.WriteString(strings.ReplaceAll(m.Signature, "\n", "\n  "))
			sb.WriteString("\n")
		}
	}

	return []byte(sb.String()), nil
}
