package readme_cmd

type ReadmeCommandResponse struct {
	Compact bool `json:"compact"`
}

func (r *ReadmeCommandResponse) MarshalJSON() ([]byte, error) {
	// TODO
	return []byte("{}"), nil
}

func (r *ReadmeCommandResponse) MarshalMarkdown() ([]byte, error) {
	// TODO
	return []byte(""), nil
}
