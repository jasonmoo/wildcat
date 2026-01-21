package deadcode_cmd

import (
	"encoding/json"
	"fmt"
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
	Definition string `json:"definition"`
	ParentType string `json:"parent_type,omitempty"` // for methods: receiver type; for constructors: return type
}

type Summary struct {
	TotalSymbols int `json:"total_symbols"`
	DeadSymbols  int `json:"dead_symbols"`
}

type DeadcodeCommandResponse struct {
	Query              QueryInfo      `json:"query"`
	Dead               []DeadSymbol   `json:"dead"`
	TotalMethodsByType map[string]int `json:"-"` // internal, for grouping logic
	Summary            Summary        `json:"summary"`
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

	// Build set of dead types for grouping
	deadTypes := make(map[string]DeadSymbol)
	for _, d := range r.Dead {
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

	for _, d := range r.Dead {
		switch d.Kind {
		case "type", "interface":
			// Types are handled separately
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

	// Output dead types with their methods/constructors
	if len(deadTypes) > 0 {
		fmt.Fprintf(&sb, "## Dead Types (%d)\n\n", len(deadTypes))
		for _, t := range r.Dead {
			if t.Kind != "type" && t.Kind != "interface" {
				continue
			}
			fmt.Fprintf(&sb, "### %s\n\n", t.Symbol)
			fmt.Fprintf(&sb, "`%s`\n\n", t.Signature)
			fmt.Fprintf(&sb, "%s\n\n", t.Definition)

			// Show constructors for this type
			if ctors := constructorsByType[t.Symbol]; len(ctors) > 0 {
				fmt.Fprintf(&sb, "**Constructors (%d):**\n", len(ctors))
				for _, c := range ctors {
					fmt.Fprintf(&sb, "- %s `%s`\n", c.Symbol, c.Signature)
				}
				sb.WriteString("\n")
			}

			// Show methods for this type
			if methods := methodsByType[t.Symbol]; len(methods) > 0 {
				fmt.Fprintf(&sb, "**Methods (%d):**\n", len(methods))
				for _, m := range methods {
					fmt.Fprintf(&sb, "- %s `%s`\n", m.Symbol, m.Signature)
				}
				sb.WriteString("\n")
			}
		}
	}

	// Output standalone functions
	if len(standaloneFuncs) > 0 {
		fmt.Fprintf(&sb, "## Dead Functions (%d)\n\n", len(standaloneFuncs))
		for _, d := range standaloneFuncs {
			fmt.Fprintf(&sb, "- **%s** `%s`\n", d.Symbol, d.Signature)
			fmt.Fprintf(&sb, "  %s\n", d.Definition)
		}
		sb.WriteString("\n")
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
				// ALL methods are dead - group them
				groupedTypes = append(groupedTypes, parent)
			} else {
				// Only some methods dead - flat list
				flatMethods = append(flatMethods, deadMethods...)
			}
		}

		fmt.Fprintf(&sb, "## Dead Methods (%d)\n\n", len(standaloneMethods))

		// Show grouped types first
		for _, parent := range groupedTypes {
			methods := methodsByParent[parent]
			fmt.Fprintf(&sb, "### %s (%d methods)\n\n", parent, len(methods))
			for _, m := range methods {
				name := m.Symbol
				if idx := strings.LastIndex(name, "."); idx >= 0 {
					name = name[idx+1:]
				}
				fmt.Fprintf(&sb, "- **%s** `%s`\n", name, m.Signature)
				fmt.Fprintf(&sb, "  %s\n", m.Definition)
			}
			sb.WriteString("\n")
		}

		// Show flat methods (partial dead)
		for _, m := range flatMethods {
			fmt.Fprintf(&sb, "- **%s** `%s`\n", m.Symbol, m.Signature)
			fmt.Fprintf(&sb, "  %s\n", m.Definition)
		}
		if len(flatMethods) > 0 {
			sb.WriteString("\n")
		}
	}

	// Output constants
	if len(standaloneConsts) > 0 {
		fmt.Fprintf(&sb, "## Dead Constants (%d)\n\n", len(standaloneConsts))
		for _, d := range standaloneConsts {
			fmt.Fprintf(&sb, "- **%s** `%s`\n", d.Symbol, d.Signature)
			fmt.Fprintf(&sb, "  %s\n", d.Definition)
		}
		sb.WriteString("\n")
	}

	// Output variables
	if len(standaloneVars) > 0 {
		fmt.Fprintf(&sb, "## Dead Variables (%d)\n\n", len(standaloneVars))
		for _, d := range standaloneVars {
			fmt.Fprintf(&sb, "- **%s** `%s`\n", d.Symbol, d.Signature)
			fmt.Fprintf(&sb, "  %s\n", d.Definition)
		}
		sb.WriteString("\n")
	}

	return []byte(sb.String()), nil
}
