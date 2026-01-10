package output

import (
	"encoding/json"
	"io"
)

// Writer handles output formatting.
type Writer struct {
	w      io.Writer
	pretty bool
}

// NewWriter creates a new output writer.
func NewWriter(w io.Writer, pretty bool) *Writer {
	return &Writer{
		w:      w,
		pretty: pretty,
	}
}

// Write writes any value as JSON.
func (w *Writer) Write(v any) error {
	enc := json.NewEncoder(w.w)
	if w.pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(v)
}

// WriteError writes an error response.
func (w *Writer) WriteError(code, message string, suggestions []string, context map[string]any) error {
	resp := ErrorResponse{
		Error: ErrorDetail{
			Code:        code,
			Message:     message,
			Suggestions: suggestions,
			Context:     context,
		},
	}
	return w.Write(resp)
}

// Marshal marshals any value to JSON bytes.
func Marshal(v any, pretty bool) ([]byte, error) {
	if pretty {
		return json.MarshalIndent(v, "", "  ")
	}
	return json.Marshal(v)
}
