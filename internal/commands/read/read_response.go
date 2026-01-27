package read_cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
)

// ReadSection represents the result of reading a single path.
type ReadSection struct {
	Path        string   `json:"path"`                  // original query path
	Resolved    string   `json:"resolved,omitempty"`    // fully resolved path
	Source      string   `json:"source,omitempty"`      // rendered source code
	Error       string   `json:"error,omitempty"`       // error if resolution failed
	Suggestions []string `json:"suggestions,omitempty"` // fuzzy matches on error
}

// ReadResponse is the result of the read command.
type ReadResponse struct {
	Sections    []ReadSection         `json:"sections"`
	Diagnostics []commands.Diagnostic `json:"diagnostics,omitempty"`
}

var _ commands.Result = (*ReadResponse)(nil)

func (r *ReadResponse) SetDiagnostics(ds []commands.Diagnostic) {
	r.Diagnostics = ds
}

func (r *ReadResponse) MarshalJSON() ([]byte, error) {
	type Alias ReadResponse
	return json.Marshal((*Alias)(r))
}

func (r *ReadResponse) MarshalMarkdown() ([]byte, error) {
	var buf bytes.Buffer

	for i, section := range r.Sections {
		if i > 0 {
			buf.WriteString("\n---\n\n")
		}

		if section.Error != "" {
			fmt.Fprintf(&buf, "# error: %s - %s\n", section.Path, section.Error)
			if len(section.Suggestions) > 0 {
				fmt.Fprintf(&buf, "# did you mean: %s\n", strings.Join(section.Suggestions, ", "))
			}
			continue
		}

		// Header with resolved path
		fmt.Fprintf(&buf, "# %s\n\n", section.Resolved)

		// Source code in a code block
		fmt.Fprintf(&buf, "```\n%s\n```\n", section.Source)
	}

	commands.FormatDiagnosticsMarkdown(&buf, r.Diagnostics)

	return buf.Bytes(), nil
}
