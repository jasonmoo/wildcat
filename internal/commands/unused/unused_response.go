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

	// Group by kind
	byKind := make(map[string][]UnusedSymbol)
	for _, u := range r.Unused {
		byKind[u.Kind] = append(byKind[u.Kind], u)
	}

	// Output order
	kindOrder := []string{"func", "method", "type", "interface", "const", "var"}
	kindTitles := map[string]string{
		"func":      "Functions",
		"method":    "Methods",
		"type":      "Types",
		"interface": "Interfaces",
		"const":     "Constants",
		"var":       "Variables",
	}

	for _, kind := range kindOrder {
		symbols, ok := byKind[kind]
		if !ok || len(symbols) == 0 {
			continue
		}

		title := kindTitles[kind]
		if title == "" {
			title = kind
		}

		fmt.Fprintf(&sb, "## %s (%d)\n\n", title, len(symbols))
		for _, u := range symbols {
			fmt.Fprintf(&sb, "- **%s** `%s`\n", u.Symbol, u.Signature)
			fmt.Fprintf(&sb, "  %s\n", u.Definition)
		}
		sb.WriteString("\n")
	}

	return []byte(sb.String()), nil
}
