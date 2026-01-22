package symbol_cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/output"
)

var _ commands.Result = (*SymbolCommandResponse)(nil)

// SymbolRefs contains reference counts for a symbol.
type SymbolRefs struct {
	Internal int `json:"internal"` // references from same package
	External int `json:"external"` // references from other packages
	Packages int `json:"packages"` // number of external packages
}

// FunctionInfo describes a method or constructor.
type FunctionInfo struct {
	Symbol     string     `json:"symbol"` // qualified: pkg.Type.Method or pkg.Func
	Signature  string     `json:"signature"`
	Definition string     `json:"definition"` // file:start:end
	Refs       SymbolRefs `json:"refs"`
}

// DescendantInfo describes a type that would be orphaned if the target is removed.
type DescendantInfo struct {
	Symbol     string     `json:"symbol"` // qualified: pkg.Type
	Signature  string     `json:"signature"`
	Definition string     `json:"definition"` // file:line
	Reason     string     `json:"reason"`     // why it's a descendant
	Refs       SymbolRefs `json:"refs"`
}

// TypeInfo describes a type (for implementations/satisfies).
type TypeInfo struct {
	Symbol     string     `json:"symbol"` // qualified: pkg.Type
	Signature  string     `json:"signature"`
	Definition string     `json:"definition"` // file:line
	Refs       SymbolRefs `json:"refs"`
}

// SuggestionInfo describes a fuzzy match suggestion.
type SuggestionInfo struct {
	Symbol string `json:"symbol"` // qualified: pkg.Name or pkg.Type.Method
	Kind   string `json:"kind"`   // func, method, type, interface, const, var
}

type SymbolCommandResponse struct {
	Query             output.QueryInfo      `json:"query"`
	Package           output.PackageInfo    `json:"package"`
	Target            output.TargetInfo     `json:"target"`
	Methods           []FunctionInfo        `json:"methods,omitempty"`
	Constructors      []FunctionInfo        `json:"constructors,omitempty"`
	Descendants       []DescendantInfo      `json:"descendants,omitempty"` // types orphaned if target removed
	ImportedBy        []output.DepResult    `json:"imported_by"`
	References        []output.PackageUsage `json:"references"`
	Implementations   []TypeInfo            `json:"implementations,omitempty"`
	Satisfies         []TypeInfo            `json:"satisfies,omitempty"`
	QuerySummary      output.SymbolSummary  `json:"query_summary"`
	PackageSummary    output.SymbolSummary  `json:"package_summary"`
	ProjectSummary    output.SymbolSummary  `json:"project_summary"`
	OtherFuzzyMatches []SuggestionInfo      `json:"other_fuzzy_matches"`
}

func (r *SymbolCommandResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Query             output.QueryInfo      `json:"query"`
		Package           output.PackageInfo    `json:"package"`
		Target            output.TargetInfo     `json:"target"`
		QuerySummary      output.SymbolSummary  `json:"query_summary"`
		PackageSummary    output.SymbolSummary  `json:"package_summary"`
		ProjectSummary    output.SymbolSummary  `json:"project_summary"`
		Methods           []FunctionInfo        `json:"methods,omitempty"`
		Constructors      []FunctionInfo        `json:"constructors,omitempty"`
		Descendants       []DescendantInfo      `json:"descendants,omitempty"`
		ImportedBy        []output.DepResult    `json:"imported_by"`
		References        []output.PackageUsage `json:"references"`
		Implementations   []TypeInfo            `json:"implementations,omitempty"`
		Satisfies         []TypeInfo            `json:"satisfies,omitempty"`
		OtherFuzzyMatches []SuggestionInfo      `json:"other_fuzzy_matches"`
	}{
		Query:             r.Query,
		Package:           r.Package,
		Target:            r.Target,
		QuerySummary:      r.QuerySummary,
		PackageSummary:    r.PackageSummary,
		ProjectSummary:    r.ProjectSummary,
		Methods:           r.Methods,
		Constructors:      r.Constructors,
		Descendants:       r.Descendants,
		ImportedBy:        r.ImportedBy,
		References:        r.References,
		Implementations:   r.Implementations,
		Satisfies:         r.Satisfies,
		OtherFuzzyMatches: r.OtherFuzzyMatches,
	})
}

func (r *SymbolCommandResponse) MarshalMarkdown() ([]byte, error) {
	var sb strings.Builder

	isType := r.Target.Kind == "type"
	isInterface := r.Target.Kind == "interface"

	// Header with target symbol and package context
	fmt.Fprintf(&sb, "# %s\n\n", r.Target.Symbol)
	fmt.Fprintf(&sb, "%s // %s\n\n", r.Package.ImportPath, r.Package.Dir)
	fmt.Fprintf(&sb, "%s // %s, refs(%d pkg, %d proj, imported %d)\n\n", r.Target.Signature, r.Target.Definition, r.Target.Refs.Internal, r.Target.Refs.External, r.Target.Refs.Packages)

	// Summary section
	sb.WriteString("## Summary\n\n")
	fmt.Fprintf(&sb, "Package // %d callers, %d references\n", r.PackageSummary.Callers, r.PackageSummary.References)
	fmt.Fprintf(&sb, "Project // %d callers, %d references\n\n", r.ProjectSummary.Callers, r.ProjectSummary.References)

	// Methods (show for types, even if empty)
	if isType || len(r.Methods) > 0 {
		fmt.Fprintf(&sb, "## Methods (%d)\n\n", len(r.Methods))
		for _, m := range r.Methods {
			fmt.Fprintf(&sb, "%s // %s, refs(%d pkg, %d proj, imported %d)\n", m.Signature, m.Definition, m.Refs.Internal, m.Refs.External, m.Refs.Packages)
		}
		if len(r.Methods) > 0 {
			sb.WriteString("\n")
		}
	}

	// Constructors (show for types, even if empty)
	if isType || len(r.Constructors) > 0 {
		fmt.Fprintf(&sb, "## Constructors (%d)\n\n", len(r.Constructors))
		for _, c := range r.Constructors {
			fmt.Fprintf(&sb, "%s // %s, refs(%d pkg, %d proj, imported %d)\n", c.Signature, c.Definition, c.Refs.Internal, c.Refs.External, c.Refs.Packages)
		}
		if len(r.Constructors) > 0 {
			sb.WriteString("\n")
		}
	}

	// Descendants (show for types, even if empty)
	if isType || len(r.Descendants) > 0 {
		fmt.Fprintf(&sb, "## Descendants (%d)\n\n", len(r.Descendants))
		for _, d := range r.Descendants {
			fmt.Fprintf(&sb, "%s // %s, refs(%d pkg, %d proj, imported %d)\n", d.Signature, d.Definition, d.Refs.Internal, d.Refs.External, d.Refs.Packages)
			if d.Reason != "" {
				fmt.Fprintf(&sb, "  %s\n", d.Reason)
			}
		}
		if len(r.Descendants) > 0 {
			sb.WriteString("\n")
		}
	}

	// Imported by (always show)
	fmt.Fprintf(&sb, "## Imported By (%d)\n\n", len(r.ImportedBy))
	for _, dep := range r.ImportedBy {
		sb.WriteString(dep.Package)
		if dep.Location != "" {
			sb.WriteString(" // ")
			sb.WriteString(dep.Location)
		}
		sb.WriteString("\n")
	}
	if len(r.ImportedBy) > 0 {
		sb.WriteString("\n")
	}

	// References by package (always show)
	fmt.Fprintf(&sb, "## References By Package (%d)\n\n", len(r.References))
	for _, pkg := range r.References {
		fmt.Fprintf(&sb, "### %s // %s\n\n", pkg.Package, pkg.Dir)

		if len(pkg.Callers) > 0 {
			sb.WriteString("#### Callers\n\n")
			for _, caller := range pkg.Callers {
				fmt.Fprintf(&sb, "##### %s // %s\n", caller.Symbol, caller.Snippet.Location)
				if caller.Snippet.Source != "" {
					sb.WriteString("```")
					sb.WriteString(caller.Snippet.Source)
					sb.WriteString("```\n\n")
				}
			}
		}

		if len(pkg.References) > 0 {
			// Count total refs (RefCount of 0 or 1 means single ref)
			totalRefs := 0
			for _, ref := range pkg.References {
				if ref.RefCount > 1 {
					totalRefs += ref.RefCount
				} else {
					totalRefs++
				}
			}
			fmt.Fprintf(&sb, "#### References (%d)\n\n", totalRefs)
			for _, ref := range pkg.References {
				// Build annotation with ref count if merged
				refAnnotation := ""
				if ref.RefCount > 1 {
					refAnnotation = fmt.Sprintf(", %d refs", ref.RefCount)
				}
				if ref.Symbol != "" {
					fmt.Fprintf(&sb, "##### %s // %s%s\n", ref.Symbol, ref.Snippet.Location, refAnnotation)
				} else {
					fmt.Fprintf(&sb, "##### %s%s\n", ref.Snippet.Location, refAnnotation)
				}
				if ref.Snippet.Source != "" {
					sb.WriteString("```")
					sb.WriteString(ref.Snippet.Source)
					sb.WriteString("```\n\n")
				}
			}
		}
	}

	// Implementations (show for interfaces, even if empty)
	if isInterface || len(r.Implementations) > 0 {
		fmt.Fprintf(&sb, "## Implementations (%d)\n\n", len(r.Implementations))
		for _, impl := range r.Implementations {
			fmt.Fprintf(&sb, "%s // %s, refs(%d pkg, %d proj, imported %d)\n", impl.Signature, impl.Definition, impl.Refs.Internal, impl.Refs.External, impl.Refs.Packages)
		}
		if len(r.Implementations) > 0 {
			sb.WriteString("\n")
		}
	}

	// Satisfies (show for types, even if empty)
	if isType || len(r.Satisfies) > 0 {
		fmt.Fprintf(&sb, "## Satisfies (%d)\n\n", len(r.Satisfies))
		for _, sat := range r.Satisfies {
			fmt.Fprintf(&sb, "%s // %s, refs(%d pkg, %d proj, imported %d)\n", sat.Signature, sat.Definition, sat.Refs.Internal, sat.Refs.External, sat.Refs.Packages)
		}
		if len(r.Satisfies) > 0 {
			sb.WriteString("\n")
		}
	}

	// Fuzzy matches (only show if > 0)
	if len(r.OtherFuzzyMatches) > 0 {
		fmt.Fprintf(&sb, "## Similar Symbols (%d)\n\n", len(r.OtherFuzzyMatches))
		for _, match := range r.OtherFuzzyMatches {
			fmt.Fprintf(&sb, "- %s [%s]\n", match.Symbol, match.Kind)
		}
		sb.WriteString("\n")
	}

	return []byte(sb.String()), nil
}
