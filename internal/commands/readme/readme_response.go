package readme_cmd

import "github.com/jasonmoo/wildcat/internal/commands"

type ReadmeCommandResponse struct {
	Compact     bool                   `json:"compact"`
	Diagnostics []commands.Diagnostics `json:"diagnostics,omitempty"`
}

var _ commands.Result = (*ReadmeCommandResponse)(nil)

func (r *ReadmeCommandResponse) SetDiagnostics(ds []commands.Diagnostics) {
	r.Diagnostics = ds
}

func (r *ReadmeCommandResponse) MarshalJSON() ([]byte, error) {
	// TODO
	return []byte("{}"), nil
}

func (r *ReadmeCommandResponse) MarshalMarkdown() ([]byte, error) {
	// TODO
	return []byte(""), nil
}
