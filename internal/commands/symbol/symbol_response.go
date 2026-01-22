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
	Definition string     `json:"definition"` // file:line (short form when in PackageTypes context)
	Refs       SymbolRefs `json:"refs,omitempty"`
	Impls      ImplCounts `json:"impls,omitempty"` // for interfaces: how many types implement it
}

// ImplCounts tracks how many types implement an interface.
type ImplCounts struct {
	Package int `json:"package"` // implementations in the target symbol's package
	Project int `json:"project"` // implementations across the project
}

// PackageTypes groups types by package for implementations/satisfies sections.
type PackageTypes struct {
	Package string     `json:"package"` // import path
	Dir     string     `json:"dir"`     // directory path
	Types   []TypeInfo `json:"types"`
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
	Implementations   []PackageTypes        `json:"implementations,omitempty"`
	Satisfies         []PackageTypes        `json:"satisfies,omitempty"`
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
		Implementations   []PackageTypes        `json:"implementations,omitempty"`
		Satisfies         []PackageTypes        `json:"satisfies,omitempty"`
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
			fmt.Fprintf(&sb, "%s // %s, callers(%d pkg, %d proj, imported %d)\n", m.Signature, m.Definition, m.Refs.Internal, m.Refs.External, m.Refs.Packages)
		}
		if len(r.Methods) > 0 {
			sb.WriteString("\n")
		}
	}

	// Constructors (show for types, even if empty)
	if isType || len(r.Constructors) > 0 {
		fmt.Fprintf(&sb, "## Constructors (%d)\n\n", len(r.Constructors))
		for _, c := range r.Constructors {
			fmt.Fprintf(&sb, "%s // %s, callers(%d pkg, %d proj, imported %d)\n", c.Signature, c.Definition, c.Refs.Internal, c.Refs.External, c.Refs.Packages)
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
				fmt.Fprintf(&sb, "##### %s\n", caller.Symbol)
				if caller.Snippet.Source != "" {
					sb.WriteString("```")
					sb.WriteString(caller.Snippet.Source)
					fmt.Fprintf(&sb, "``` // %s\n\n", caller.Snippet.Location)
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
					fmt.Fprintf(&sb, "##### %s\n", ref.Symbol)
				}
				if ref.Snippet.Source != "" {
					sb.WriteString("```")
					sb.WriteString(ref.Snippet.Source)
					fmt.Fprintf(&sb, "``` // %s%s\n\n", ref.Snippet.Location, refAnnotation)
				}
			}
		}
	}

	// Implementations (show for interfaces, even if empty)
	if isInterface || len(r.Implementations) > 0 {
		// Count total implementations across all packages
		totalImpls := 0
		for _, pkg := range r.Implementations {
			totalImpls += len(pkg.Types)
		}
		fmt.Fprintf(&sb, "## Implementations (%d)\n\n", totalImpls)
		for _, pkg := range r.Implementations {
			fmt.Fprintf(&sb, "### %s // %s\n\n", pkg.Package, pkg.Dir)
			for _, impl := range pkg.Types {
				fmt.Fprintf(&sb, "%s // %s, refs(%d pkg, %d proj, imported %d)\n", impl.Signature, impl.Definition, impl.Refs.Internal, impl.Refs.External, impl.Refs.Packages)
			}
			sb.WriteString("\n")
		}
	}

	// Satisfies (show for types, even if empty)
	if isType || len(r.Satisfies) > 0 {
		// Count total satisfies across all packages
		totalSats := 0
		for _, pkg := range r.Satisfies {
			totalSats += len(pkg.Types)
		}
		fmt.Fprintf(&sb, "## Satisfies (%d)\n\n", totalSats)
		for _, pkg := range r.Satisfies {
			fmt.Fprintf(&sb, "### %s // %s\n\n", pkg.Package, pkg.Dir)
			for _, sat := range pkg.Types {
				fmt.Fprintf(&sb, "%s // %s, impls(%d pkg, %d proj)\n", sat.Signature, sat.Definition, sat.Impls.Package, sat.Impls.Project)
			}
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
