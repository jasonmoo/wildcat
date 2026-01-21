package deadcode_cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
)

var _ commands.Result = (*DeadcodeCommandResponse)(nil)

type QueryInfo struct {
	Command      string   `json:"command"`
	Targets      []string `json:"targets,omitempty"`
	IncludeTests bool     `json:"include_tests"`
}

type Summary struct {
	TotalSymbols int `json:"total_symbols"`
	DeadSymbols  int `json:"dead_symbols"`
}

// DeadSymbol represents a single dead symbol
type DeadSymbol struct {
	Symbol     string `json:"symbol"`
	Kind       string `json:"-"` // for future formats
	Signature  string `json:"signature"`
	Definition string `json:"definition"` // file:start:end
}

// FileInfo tracks symbol counts per file
type FileInfo struct {
	TotalSymbols int `json:"total_symbols"`
	DeadSymbols  int `json:"dead_symbols"`
}

// DeadType holds a dead type with its constructors and methods
type DeadType struct {
	Symbol       DeadSymbol   `json:"symbol"`
	Constructors []DeadSymbol `json:"constructors,omitempty"`
	Methods      []DeadSymbol `json:"methods,omitempty"`
}

// DeadMethodGroup groups methods by their parent type (for types that aren't dead but have dead methods)
type DeadMethodGroup struct {
	ParentType string       `json:"parent_type"`
	AllDead    bool         `json:"all_dead"` // true if all methods of this type are dead
	Methods    []DeadSymbol `json:"methods"`
}

// PackageDeadCode contains all dead code for a single package
type PackageDeadCode struct {
	Package     string              `json:"package"`
	IsDead      bool                `json:"is_dead"`                 // entire package is dead
	DeadFiles   []string            `json:"dead_files,omitempty"`    // files where all symbols dead
	FileInfo    map[string]FileInfo `json:"file_info,omitempty"`     // per-file stats
	Constants   []DeadSymbol        `json:"constants,omitempty"`     // dead constants
	Variables   []DeadSymbol        `json:"variables,omitempty"`     // dead variables
	Functions   []DeadSymbol        `json:"functions,omitempty"`     // dead standalone functions
	Types       []DeadType          `json:"types,omitempty"`         // dead types with their methods/constructors
	DeadMethods []DeadMethodGroup   `json:"dead_methods,omitempty"`  // methods whose type isn't dead
}

// DeadcodeCommandResponse is the structured response for deadcode analysis
type DeadcodeCommandResponse struct {
	Query        QueryInfo          `json:"query"`
	Summary      Summary            `json:"summary"`
	DeadPackages []string           `json:"dead_packages,omitempty"` // fully dead package paths
	Packages     []*PackageDeadCode `json:"packages,omitempty"`

	// Internal fields for building the response
	totalMethodsByType map[string]int // tracks total methods per type for grouping logic
}

func (r *DeadcodeCommandResponse) MarshalJSON() ([]byte, error) {
	// Sort for consistent output: dead first, then alphabetical
	sort.Strings(r.DeadPackages)
	sort.Slice(r.Packages, func(i, j int) bool {
		if r.Packages[i].IsDead != r.Packages[j].IsDead {
			return r.Packages[i].IsDead // dead packages first
		}
		return r.Packages[i].Package < r.Packages[j].Package
	})

	return json.Marshal(struct {
		Query        QueryInfo          `json:"query"`
		Summary      Summary            `json:"summary"`
		DeadPackages []string           `json:"dead_packages,omitempty"`
		Packages     []*PackageDeadCode `json:"packages,omitempty"`
	}{
		Query:        r.Query,
		Summary:      r.Summary,
		DeadPackages: r.DeadPackages,
		Packages:     r.Packages,
	})
}

func (r *DeadcodeCommandResponse) MarshalMarkdown() ([]byte, error) {
	var sb strings.Builder

	sb.WriteString("# Dead Code Analysis\n\n")

	// Query info
	if len(r.Query.Targets) > 0 {
		fmt.Fprintf(&sb, "targets: %s\n", strings.Join(r.Query.Targets, ", "))
	}
	fmt.Fprintf(&sb, "include_tests: %v\n\n", r.Query.IncludeTests)

	// Summary
	fmt.Fprintf(&sb, "## Summary\n\n")
	fmt.Fprintf(&sb, "- Total symbols analyzed: %d\n", r.Summary.TotalSymbols)
	fmt.Fprintf(&sb, "- Dead (unreachable) symbols: %d\n\n", r.Summary.DeadSymbols)

	if len(r.Packages) == 0 {
		sb.WriteString("No dead code found.\n")
		return []byte(sb.String()), nil
	}

	// Show dead packages summary at top
	if len(r.DeadPackages) > 0 {
		fmt.Fprintf(&sb, "# Dead Packages (%d)\n\n", len(r.DeadPackages))
		for _, pkg := range r.DeadPackages {
			for _, pi := range r.Packages {
				if pi.Package == pkg {
					symbolCount := len(pi.Constants) + len(pi.Variables) + len(pi.Functions)
					for _, t := range pi.Types {
						symbolCount += 1 + len(t.Constructors) + len(t.Methods)
					}
					for _, mg := range pi.DeadMethods {
						symbolCount += len(mg.Methods)
					}
					fmt.Fprintf(&sb, "- %s (%d symbols, %d files)\n", pkg, symbolCount, len(pi.FileInfo))
					break
				}
			}
		}
		sb.WriteString("\n")
	}

	// Sort packages: dead first, then alphabetical
	sort.Slice(r.Packages, func(i, j int) bool {
		if r.Packages[i].IsDead != r.Packages[j].IsDead {
			return r.Packages[i].IsDead // dead packages first
		}
		return r.Packages[i].Package < r.Packages[j].Package
	})

	// Output each package
	for _, pi := range r.Packages {
		// Package header
		if pi.IsDead {
			fmt.Fprintf(&sb, "# package %s (DEAD)\n\n", pi.Package)
		} else {
			fmt.Fprintf(&sb, "# package %s\n\n", pi.Package)
		}

		// Show dead files (only if not all files are dead)
		if len(pi.DeadFiles) > 0 && len(pi.DeadFiles) < len(pi.FileInfo) {
			fmt.Fprintf(&sb, "# Dead Files (%d)\n", len(pi.DeadFiles))
			for _, f := range pi.DeadFiles {
				if fi, ok := pi.FileInfo[f]; ok {
					fmt.Fprintf(&sb, "- %s (%d symbols)\n", f, fi.TotalSymbols)
				}
			}
			sb.WriteString("\n")
		}

		// Output constants
		if len(pi.Constants) > 0 {
			fmt.Fprintf(&sb, "# Constants (%d)\n", len(pi.Constants))
			for _, d := range pi.Constants {
				fmt.Fprintf(&sb, "%s // %s\n", d.Signature, d.Definition)
			}
			sb.WriteString("\n")
		}

		// Output variables
		if len(pi.Variables) > 0 {
			fmt.Fprintf(&sb, "# Variables (%d)\n", len(pi.Variables))
			for _, d := range pi.Variables {
				fmt.Fprintf(&sb, "%s // %s\n", d.Signature, d.Definition)
			}
			sb.WriteString("\n")
		}

		// Output functions
		if len(pi.Functions) > 0 {
			fmt.Fprintf(&sb, "# Functions (%d)\n", len(pi.Functions))
			for _, d := range pi.Functions {
				fmt.Fprintf(&sb, "%s // %s\n", d.Signature, d.Definition)
			}
			sb.WriteString("\n")
		}

		// Output types with their methods/constructors
		if len(pi.Types) > 0 {
			fmt.Fprintf(&sb, "# Types (%d)\n\n", len(pi.Types))
			for _, t := range pi.Types {
				fmt.Fprintf(&sb, "%s // %s", t.Symbol.Signature, t.Symbol.Definition)
				if len(t.Methods) > 0 {
					fmt.Fprintf(&sb, " // %d methods", len(t.Methods))
				}
				sb.WriteString("\n")

				for _, c := range t.Constructors {
					fmt.Fprintf(&sb, "%s // %s\n", c.Signature, c.Definition)
				}
				for _, m := range t.Methods {
					fmt.Fprintf(&sb, "%s // %s\n", m.Signature, m.Definition)
				}
				sb.WriteString("\n")
			}
		}

		// Output dead methods (grouped by parent type)
		if len(pi.DeadMethods) > 0 {
			totalMethods := 0
			for _, mg := range pi.DeadMethods {
				totalMethods += len(mg.Methods)
			}
			fmt.Fprintf(&sb, "# Dead Methods (%d)\n\n", totalMethods)

			for _, mg := range pi.DeadMethods {
				if mg.AllDead {
					fmt.Fprintf(&sb, "## %s (%d methods)\n", mg.ParentType, len(mg.Methods))
				} else {
					fmt.Fprintf(&sb, "## %s\n", mg.ParentType)
				}
				for _, m := range mg.Methods {
					fmt.Fprintf(&sb, "%s // %s\n", m.Signature, m.Definition)
				}
				sb.WriteString("\n")
			}
		}
	}

	return []byte(sb.String()), nil
}
