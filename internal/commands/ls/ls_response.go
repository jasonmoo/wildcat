package ls_cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
)

// PathEntry represents a single path in the listing.
type PathEntry struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	Type string `json:"type,omitempty"`
}

// TargetSection represents the listing for a single target.
type TargetSection struct {
	Target      string      `json:"target"`
	Scope       string      `json:"scope,omitempty"` // "package", "symbol", "field"
	Package     string      `json:"package,omitempty"`
	Symbol      string      `json:"symbol,omitempty"` // symbol name for header
	Paths       []PathEntry `json:"paths,omitempty"`
	Error       string      `json:"error,omitempty"`
	Suggestions []string    `json:"suggestions,omitempty"`
}

// LsResponse is the result of the ls command.
type LsResponse struct {
	Sections    []TargetSection       `json:"sections"`
	Diagnostics []commands.Diagnostic `json:"diagnostics,omitempty"`
}

var _ commands.Result = (*LsResponse)(nil)

func (r *LsResponse) SetDiagnostics(ds []commands.Diagnostic) {
	r.Diagnostics = ds
}

func (r *LsResponse) MarshalJSON() ([]byte, error) {
	type Alias LsResponse
	return json.Marshal((*Alias)(r))
}

func (r *LsResponse) MarshalMarkdown() ([]byte, error) {
	var buf bytes.Buffer

	// Header line: # query <full paths comma separated>
	var resolvedPaths []string
	for _, section := range r.Sections {
		if section.Error == "" && section.Package != "" {
			// Use full package path + symbol for the header
			path := section.Package
			if section.Symbol != "" {
				path += "." + section.Symbol
			}
			resolvedPaths = append(resolvedPaths, path)
		}
	}
	if len(resolvedPaths) > 0 {
		fmt.Fprintf(&buf, "# query %s\n", strings.Join(resolvedPaths, ", "))
	}

	// Flat list of all paths
	for _, section := range r.Sections {
		if section.Error != "" {
			fmt.Fprintf(&buf, "Error: (path_not_found) %q %s\n", section.Target, section.Error)
			if len(section.Suggestions) > 0 {
				buf.WriteString("Suggestions:\n")
				for _, s := range section.Suggestions {
					fmt.Fprintf(&buf, " - %s\n", s)
				}
			}
			continue
		}
		for _, p := range section.Paths {
			if p.Type != "" {
				fmt.Fprintf(&buf, "%s  // %s\n", p.Path, p.Type)
			} else {
				fmt.Fprintf(&buf, "%s\n", p.Path)
			}
		}
	}

	commands.FormatDiagnosticsMarkdown(&buf, r.Diagnostics)

	return buf.Bytes(), nil
}
