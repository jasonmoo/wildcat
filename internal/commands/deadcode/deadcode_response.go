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
	Command      string `json:"command"`
	Target       string `json:"target,omitempty"`
	IncludeTests bool   `json:"include_tests"`
}

type DeadSymbol struct {
	Symbol     string `json:"symbol"`
	Kind       string `json:"kind"`
	Signature  string `json:"signature"`
	Package    string `json:"package"`              // full package path
	Filename   string `json:"filename"`             // relative filename within package
	Location   string `json:"location"`             // line:col or start:end
	ParentType string `json:"parent_type,omitempty"` // for methods: receiver type; for constructors: return type
}

type Summary struct {
	TotalSymbols int `json:"total_symbols"`
	DeadSymbols  int `json:"dead_symbols"`
}

// FileInfo tracks symbol counts per file for dead file detection
type FileInfo struct {
	Filename     string `json:"filename"`
	TotalSymbols int    `json:"total_symbols"`
	DeadSymbols  int    `json:"dead_symbols"`
	Lines        int    `json:"lines"`
}

// PackageInfo tracks symbol counts per package for dead package detection
type PackageInfo struct {
	Package      string              `json:"package"`
	TotalSymbols int                 `json:"total_symbols"`
	DeadSymbols  int                 `json:"dead_symbols"`
	Files        map[string]FileInfo `json:"files"`
	TotalLines   int                 `json:"total_lines"`
}

type DeadcodeCommandResponse struct {
	Query              QueryInfo              `json:"query"`
	Dead               []DeadSymbol           `json:"dead"`
	TotalMethodsByType map[string]int         `json:"-"` // internal, for grouping logic
	Packages           map[string]PackageInfo `json:"-"` // internal, for dead file/package detection
	Summary            Summary                `json:"summary"`
}

func (r *DeadcodeCommandResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Query   QueryInfo    `json:"query"`
		Summary Summary      `json:"summary"`
		Dead    []DeadSymbol `json:"dead"`
	}{
		Query:   r.Query,
		Summary: r.Summary,
		Dead:    r.Dead,
	})
}

func (r *DeadcodeCommandResponse) MarshalMarkdown() ([]byte, error) {
	var sb strings.Builder

	sb.WriteString("# Dead Code Analysis\n\n")

	// Query info
	if r.Query.Target != "" {
		fmt.Fprintf(&sb, "target: %s\n", r.Query.Target)
	}
	fmt.Fprintf(&sb, "include_tests: %v\n\n", r.Query.IncludeTests)

	// Summary
	fmt.Fprintf(&sb, "## Summary\n\n")
	fmt.Fprintf(&sb, "- Total symbols analyzed: %d\n", r.Summary.TotalSymbols)
	fmt.Fprintf(&sb, "- Dead (unreachable) symbols: %d\n\n", r.Summary.DeadSymbols)

	if len(r.Dead) == 0 {
		sb.WriteString("No dead code found.\n")
		return []byte(sb.String()), nil
	}

	// Identify fully dead packages
	var deadPackages []string
	var partialPackages []string
	for pkgPath, pi := range r.Packages {
		if pi.TotalSymbols > 0 && pi.DeadSymbols == pi.TotalSymbols {
			deadPackages = append(deadPackages, pkgPath)
		} else if pi.DeadSymbols > 0 {
			partialPackages = append(partialPackages, pkgPath)
		}
	}

	// Sort for consistent output
	sort.Strings(deadPackages)
	sort.Strings(partialPackages)

	// Show dead packages summary at top
	if len(deadPackages) > 0 {
		fmt.Fprintf(&sb, "# Dead Packages (%d)\n\n", len(deadPackages))
		sb.WriteString("These packages are entirely dead and can be deleted:\n\n")
		for _, pkg := range deadPackages {
			pi := r.Packages[pkg]
			fmt.Fprintf(&sb, "- %s (%d symbols, %d files)\n", pkg, pi.TotalSymbols, len(pi.Files))
		}
		sb.WriteString("\n")
	}

	// Group symbols by package
	byPackage := make(map[string][]DeadSymbol)
	var packageOrder []string
	for _, d := range r.Dead {
		if _, seen := byPackage[d.Package]; !seen {
			packageOrder = append(packageOrder, d.Package)
		}
		byPackage[d.Package] = append(byPackage[d.Package], d)
	}

	// Sort for consistent output
	sort.Strings(packageOrder)

	// Output each package
	for _, pkg := range packageOrder {
		symbols := byPackage[pkg]

		// Check if entire package is dead
		pi := r.Packages[pkg]
		if pi.TotalSymbols > 0 && pi.DeadSymbols == pi.TotalSymbols {
			fmt.Fprintf(&sb, "# package %s (DEAD)\n\n", pkg)
		} else {
			fmt.Fprintf(&sb, "# package %s\n\n", pkg)
		}

		// Show dead files
		var deadFiles []string
		for filename, fi := range pi.Files {
			if fi.TotalSymbols > 0 && fi.DeadSymbols == fi.TotalSymbols {
				deadFiles = append(deadFiles, filename)
			}
		}
		sort.Strings(deadFiles)

		if len(deadFiles) > 0 && len(deadFiles) < len(pi.Files) {
			// Only show dead files section if not all files are dead
			fmt.Fprintf(&sb, "# Dead Files (%d)\n", len(deadFiles))
			for _, f := range deadFiles {
				fi := pi.Files[f]
				fmt.Fprintf(&sb, "- %s (%d symbols)\n", f, fi.TotalSymbols)
			}
			sb.WriteString("\n")
		}

		// Build set of dead types in this package for grouping
		deadTypes := make(map[string]DeadSymbol)
		for _, d := range symbols {
			if d.Kind == "type" || d.Kind == "interface" {
				deadTypes[d.Symbol] = d
			}
		}

		// Group methods/constructors by parent type
		methodsByType := make(map[string][]DeadSymbol)
		constructorsByType := make(map[string][]DeadSymbol)
		var standaloneFuncs []DeadSymbol
		var standaloneMethods []DeadSymbol
		var standaloneConsts []DeadSymbol
		var standaloneVars []DeadSymbol

		for _, d := range symbols {
			switch d.Kind {
			case "type", "interface":
				continue
			case "method":
				if d.ParentType != "" {
					if _, ok := deadTypes[d.ParentType]; ok {
						methodsByType[d.ParentType] = append(methodsByType[d.ParentType], d)
					} else {
						standaloneMethods = append(standaloneMethods, d)
					}
				} else {
					standaloneMethods = append(standaloneMethods, d)
				}
			case "func":
				if d.ParentType != "" {
					if _, ok := deadTypes[d.ParentType]; ok {
						constructorsByType[d.ParentType] = append(constructorsByType[d.ParentType], d)
					} else {
						standaloneFuncs = append(standaloneFuncs, d)
					}
				} else {
					standaloneFuncs = append(standaloneFuncs, d)
				}
			case "const":
				standaloneConsts = append(standaloneConsts, d)
			case "var":
				standaloneVars = append(standaloneVars, d)
			}
		}

		// Output constants
		if len(standaloneConsts) > 0 {
			fmt.Fprintf(&sb, "# Constants (%d)\n", len(standaloneConsts))
			for _, d := range standaloneConsts {
				fmt.Fprintf(&sb, "%s // %s:%s\n", d.Signature, d.Filename, d.Location)
			}
			sb.WriteString("\n")
		}

		// Output variables
		if len(standaloneVars) > 0 {
			fmt.Fprintf(&sb, "# Variables (%d)\n", len(standaloneVars))
			for _, d := range standaloneVars {
				fmt.Fprintf(&sb, "%s // %s:%s\n", d.Signature, d.Filename, d.Location)
			}
			sb.WriteString("\n")
		}

		// Output standalone functions
		if len(standaloneFuncs) > 0 {
			fmt.Fprintf(&sb, "# Functions (%d)\n", len(standaloneFuncs))
			for _, d := range standaloneFuncs {
				fmt.Fprintf(&sb, "%s // %s:%s\n", d.Signature, d.Filename, d.Location)
			}
			sb.WriteString("\n")
		}

		// Output types with their methods/constructors
		if len(deadTypes) > 0 {
			fmt.Fprintf(&sb, "# Types (%d)\n\n", len(deadTypes))
			for _, d := range symbols {
				if d.Kind != "type" && d.Kind != "interface" {
					continue
				}
				fmt.Fprintf(&sb, "%s // %s:%s", d.Signature, d.Filename, d.Location)

				// Count methods and constructors
				ctors := constructorsByType[d.Symbol]
				methods := methodsByType[d.Symbol]
				var annotations []string
				if len(methods) > 0 {
					annotations = append(annotations, fmt.Sprintf("%d methods", len(methods)))
				}
				if len(annotations) > 0 {
					fmt.Fprintf(&sb, " // %s", strings.Join(annotations, ", "))
				}
				sb.WriteString("\n")

				// Show constructors
				for _, c := range ctors {
					fmt.Fprintf(&sb, "%s // %s:%s\n", c.Signature, c.Filename, c.Location)
				}

				// Show methods
				for _, m := range methods {
					fmt.Fprintf(&sb, "%s // %s:%s\n", m.Signature, m.Filename, m.Location)
				}

				sb.WriteString("\n")
			}
		}

		// Output standalone methods - group by type only if ALL methods of that type are dead
		if len(standaloneMethods) > 0 {
			// Group by parent type
			methodsByParent := make(map[string][]DeadSymbol)
			var parentOrder []string
			for _, m := range standaloneMethods {
				parent := m.ParentType
				if parent == "" {
					parent = "(no receiver)"
				}
				if _, seen := methodsByParent[parent]; !seen {
					parentOrder = append(parentOrder, parent)
				}
				methodsByParent[parent] = append(methodsByParent[parent], m)
			}

			// Separate into grouped (all methods dead) vs flat (partial)
			var groupedTypes []string
			var flatMethods []DeadSymbol
			for _, parent := range parentOrder {
				deadMethods := methodsByParent[parent]
				totalMethods := r.TotalMethodsByType[parent]
				if totalMethods > 0 && len(deadMethods) == totalMethods {
					groupedTypes = append(groupedTypes, parent)
				} else {
					flatMethods = append(flatMethods, deadMethods...)
				}
			}

			fmt.Fprintf(&sb, "# Dead Methods (%d)\n\n", len(standaloneMethods))

			// Show grouped types first
			for _, parent := range groupedTypes {
				methods := methodsByParent[parent]
				fmt.Fprintf(&sb, "## %s (%d methods)\n", parent, len(methods))
				for _, m := range methods {
					fmt.Fprintf(&sb, "%s // %s:%s\n", m.Signature, m.Filename, m.Location)
				}
				sb.WriteString("\n")
			}

			// Show flat methods (partial dead)
			for _, m := range flatMethods {
				fmt.Fprintf(&sb, "%s // %s:%s\n", m.Signature, m.Filename, m.Location)
			}
			if len(flatMethods) > 0 {
				sb.WriteString("\n")
			}
		}
	}

	return []byte(sb.String()), nil
}
