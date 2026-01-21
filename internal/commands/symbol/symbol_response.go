package symbol_cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/output"
)

var _ commands.Result = (*SymbolCommandResponse)(nil)

type SymbolCommandResponse struct {
	Query             output.QueryInfo        `json:"query"`
	Target            output.TargetInfo       `json:"target"`
	ImportedBy        []output.DepResult      `json:"imported_by"`
	References        []output.PackageUsage   `json:"references"`
	Implementations   []output.SymbolLocation `json:"implementations,omitempty"`
	Satisfies         []output.SymbolLocation `json:"satisfies,omitempty"`
	QuerySummary      output.SymbolSummary    `json:"query_summary"`
	PackageSummary    output.SymbolSummary    `json:"package_summary"`
	ProjectSummary    output.SymbolSummary    `json:"project_summary"`
	OtherFuzzyMatches []string                `json:"other_fuzzy_matches"`
}

func (r *SymbolCommandResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Query             output.QueryInfo        `json:"query"`
		Target            output.TargetInfo       `json:"target"`
		QuerySummary      output.SymbolSummary    `json:"query_summary"`
		PackageSummary    output.SymbolSummary    `json:"package_summary"`
		ProjectSummary    output.SymbolSummary    `json:"project_summary"`
		ImportedBy        []output.DepResult      `json:"imported_by"`
		References        []output.PackageUsage   `json:"references"`
		Implementations   []output.SymbolLocation `json:"implementations,omitempty"`
		Satisfies         []output.SymbolLocation `json:"satisfies,omitempty"`
		OtherFuzzyMatches []string                `json:"other_fuzzy_matches"`
	}{
		Query:             r.Query,
		Target:            r.Target,
		QuerySummary:      r.QuerySummary,
		PackageSummary:    r.PackageSummary,
		ProjectSummary:    r.ProjectSummary,
		ImportedBy:        r.ImportedBy,
		References:        r.References,
		Implementations:   r.Implementations,
		Satisfies:         r.Satisfies,
		OtherFuzzyMatches: r.OtherFuzzyMatches,
	})
}

func (r *SymbolCommandResponse) MarshalMarkdown() ([]byte, error) {
	var sb strings.Builder

	// Header
	fmt.Fprintf(&sb, "# Symbol: %s\n\n", r.Query.Target)

	// Target section
	sb.WriteString("## Target\n\n")
	fmt.Fprintf(&sb, "- **Symbol:** %s\n", r.Target.Symbol)
	fmt.Fprintf(&sb, "- **Signature:** `%s`\n", r.Target.Signature)
	fmt.Fprintf(&sb, "- **Definition:** %s\n\n", r.Target.Definition)

	// Summary section
	sb.WriteString("## Summary\n\n")
	fmt.Fprintf(&sb, "| Scope | Callers | References |\n")
	fmt.Fprintf(&sb, "|-------|---------|------------|\n")
	fmt.Fprintf(&sb, "| Query | %d | %d |\n", r.QuerySummary.Callers, r.QuerySummary.References)
	fmt.Fprintf(&sb, "| Package | %d | %d |\n", r.PackageSummary.Callers, r.PackageSummary.References)
	fmt.Fprintf(&sb, "| Project | %d | %d |\n\n", r.ProjectSummary.Callers, r.ProjectSummary.References)

	// Imported by
	if len(r.ImportedBy) > 0 {
		sb.WriteString("## Imported By\n\n")
		for _, dep := range r.ImportedBy {
			fmt.Fprintf(&sb, "- %s\n", dep.Package)
			if dep.Location != "" {
				fmt.Fprintf(&sb, "  %s\n", dep.Location)
			}
		}
		sb.WriteString("\n")
	}

	// References by package
	if len(r.References) > 0 {
		sb.WriteString("## References\n\n")
		for _, pkg := range r.References {
			fmt.Fprintf(&sb, "### %s\n\n", pkg.Package)
			fmt.Fprintf(&sb, "**Dir:** `%s`\n\n", pkg.Dir)

			if len(pkg.Callers) > 0 {
				sb.WriteString("#### Callers\n\n")
				for _, caller := range pkg.Callers {
					fmt.Fprintf(&sb, "- %s `%s`\n", caller.Location, caller.Symbol)
					if caller.Snippet.Source != "" {
						sb.WriteString("  ```go\n")
						for _, line := range strings.Split(caller.Snippet.Source, "\n") {
							sb.WriteString("  ")
							sb.WriteString(line)
							sb.WriteString("\n")
						}
						sb.WriteString("  ```\n")
					}
				}
				sb.WriteString("\n")
			}

			if len(pkg.References) > 0 {
				sb.WriteString("#### References\n\n")
				for _, ref := range pkg.References {
					if ref.Symbol != "" {
						fmt.Fprintf(&sb, "- %s `%s`\n", ref.Location, ref.Symbol)
					} else {
						fmt.Fprintf(&sb, "- %s\n", ref.Location)
					}
					if ref.Snippet.Source != "" {
						sb.WriteString("  ```go\n")
						for _, line := range strings.Split(ref.Snippet.Source, "\n") {
							sb.WriteString("  ")
							sb.WriteString(line)
							sb.WriteString("\n")
						}
						sb.WriteString("  ```\n")
					}
				}
				sb.WriteString("\n")
			}
		}
	}

	// Implementations
	if len(r.Implementations) > 0 {
		sb.WriteString("## Implementations\n\n")
		sb.WriteString("| Type | Signature | Location |\n")
		sb.WriteString("|------|-----------|----------|\n")
		for _, impl := range r.Implementations {
			sig := strings.ReplaceAll(impl.Signature, "|", "\\|")
			fmt.Fprintf(&sb, "| %s | `%s` | %s |\n", impl.Symbol, sig, impl.Location)
		}
		sb.WriteString("\n")
	}

	// Satisfies
	if len(r.Satisfies) > 0 {
		sb.WriteString("## Satisfies\n\n")
		sb.WriteString("| Interface | Signature | Location |\n")
		sb.WriteString("|-----------|-----------|----------|\n")
		for _, sat := range r.Satisfies {
			sig := strings.ReplaceAll(sat.Signature, "|", "\\|")
			fmt.Fprintf(&sb, "| %s | `%s` | %s |\n", sat.Symbol, sig, sat.Location)
		}
		sb.WriteString("\n")
	}

	// Fuzzy matches
	if len(r.OtherFuzzyMatches) > 0 {
		sb.WriteString("## Similar Symbols\n\n")
		for _, match := range r.OtherFuzzyMatches {
			fmt.Fprintf(&sb, "- %s\n", match)
		}
		sb.WriteString("\n")
	}

	return []byte(sb.String()), nil
}
