package unused_cmd

import (
	"encoding/json"
	"fmt"
	"strings"
)

type QueryInfo struct {
	Command string `json:"command"`
	Scope   string `json:"scope"`
}

type UnusedSymbol struct {
	Symbol     string `json:"symbol"`
	Kind       string `json:"kind"`
	Signature  string `json:"signature"`
	Definition string `json:"definition"`
	ParentType string `json:"parent_type,omitempty"` // for methods: receiver type; for constructors: return type
}

type Summary struct {
	Candidates int `json:"candidates"`
	Unused     int `json:"unused"`
}

type UnusedCommandResponse struct {
	Query   QueryInfo      `json:"query"`
	Unused  []UnusedSymbol `json:"unused"`
	Summary Summary        `json:"summary"`
}

func (r *UnusedCommandResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Query   QueryInfo      `json:"query"`
		Summary Summary        `json:"summary"`
		Unused  []UnusedSymbol `json:"unused"`
	}{
		Query:   r.Query,
		Summary: r.Summary,
		Unused:  r.Unused,
	})
}

func (r *UnusedCommandResponse) MarshalMarkdown() ([]byte, error) {
	var sb strings.Builder

	sb.WriteString("# Unused Symbols\n\n")

	// Query info
	fmt.Fprintf(&sb, "scope: %s\n\n", r.Query.Scope)

	// Summary
	fmt.Fprintf(&sb, "## Summary\n\n")
	fmt.Fprintf(&sb, "- Candidates analyzed: %d\n", r.Summary.Candidates)
	fmt.Fprintf(&sb, "- Unused symbols: %d\n\n", r.Summary.Unused)

	if len(r.Unused) == 0 {
		sb.WriteString("No unused symbols found.\n")
		return []byte(sb.String()), nil
	}

	// Build set of unused types for grouping
	unusedTypes := make(map[string]UnusedSymbol)
	for _, u := range r.Unused {
		if u.Kind == "type" || u.Kind == "interface" {
			unusedTypes[u.Symbol] = u
		}
	}

	// Group methods/constructors by parent type
	methodsByType := make(map[string][]UnusedSymbol)
	constructorsByType := make(map[string][]UnusedSymbol)
	var standaloneFuncs []UnusedSymbol
	var standaloneMethods []UnusedSymbol
	var standaloneConsts []UnusedSymbol
	var standaloneVars []UnusedSymbol

	for _, u := range r.Unused {
		switch u.Kind {
		case "type", "interface":
			// Types are handled separately
			continue
		case "method":
			if u.ParentType != "" {
				if _, ok := unusedTypes[u.ParentType]; ok {
					methodsByType[u.ParentType] = append(methodsByType[u.ParentType], u)
				} else {
					standaloneMethods = append(standaloneMethods, u)
				}
			} else {
				standaloneMethods = append(standaloneMethods, u)
			}
		case "func":
			if u.ParentType != "" {
				if _, ok := unusedTypes[u.ParentType]; ok {
					constructorsByType[u.ParentType] = append(constructorsByType[u.ParentType], u)
				} else {
					standaloneFuncs = append(standaloneFuncs, u)
				}
			} else {
				standaloneFuncs = append(standaloneFuncs, u)
			}
		case "const":
			standaloneConsts = append(standaloneConsts, u)
		case "var":
			standaloneVars = append(standaloneVars, u)
		}
	}

	// Output unused types with their methods/constructors
	if len(unusedTypes) > 0 {
		fmt.Fprintf(&sb, "## Types (%d)\n\n", len(unusedTypes))
		for _, t := range r.Unused {
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
		fmt.Fprintf(&sb, "## Functions (%d)\n\n", len(standaloneFuncs))
		for _, u := range standaloneFuncs {
			fmt.Fprintf(&sb, "- **%s** `%s`\n", u.Symbol, u.Signature)
			fmt.Fprintf(&sb, "  %s\n", u.Definition)
		}
		sb.WriteString("\n")
	}

	// Output standalone methods (parent type is NOT unused) - flat list
	if len(standaloneMethods) > 0 {
		fmt.Fprintf(&sb, "## Unused Methods (%d)\n\n", len(standaloneMethods))
		for _, m := range standaloneMethods {
			fmt.Fprintf(&sb, "- **%s** `%s`\n", m.Symbol, m.Signature)
			fmt.Fprintf(&sb, "  %s\n", m.Definition)
		}
		sb.WriteString("\n")
	}

	// Output constants
	if len(standaloneConsts) > 0 {
		fmt.Fprintf(&sb, "## Constants (%d)\n\n", len(standaloneConsts))
		for _, u := range standaloneConsts {
			fmt.Fprintf(&sb, "- **%s** `%s`\n", u.Symbol, u.Signature)
			fmt.Fprintf(&sb, "  %s\n", u.Definition)
		}
		sb.WriteString("\n")
	}

	// Output variables
	if len(standaloneVars) > 0 {
		fmt.Fprintf(&sb, "## Variables (%d)\n\n", len(standaloneVars))
		for _, u := range standaloneVars {
			fmt.Fprintf(&sb, "- **%s** `%s`\n", u.Symbol, u.Signature)
			fmt.Fprintf(&sb, "  %s\n", u.Definition)
		}
		sb.WriteString("\n")
	}

	return []byte(sb.String()), nil
}
