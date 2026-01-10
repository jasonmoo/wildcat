package output

import (
	"encoding/json"
	"io"
)

// Writer handles output formatting.
type Writer struct {
	w         io.Writer
	pretty    bool
	formatter Formatter
}

// NewWriter creates a new output writer with JSON formatter.
func NewWriter(w io.Writer, pretty bool) *Writer {
	return &Writer{
		w:         w,
		pretty:    pretty,
		formatter: &JSONFormatter{Pretty: pretty},
	}
}

// NewWriterWithFormat creates a new output writer with a specific formatter.
func NewWriterWithFormat(w io.Writer, formatName string) (*Writer, error) {
	formatter, err := DefaultRegistry.Get(formatName)
	if err != nil {
		return nil, err
	}
	return &Writer{
		w:         w,
		pretty:    true,
		formatter: formatter,
	}, nil
}

// Write writes any value using the configured formatter.
func (w *Writer) Write(v any) error {
	output, err := w.formatter.Format(v)
	if err != nil {
		return err
	}
	_, err = w.w.Write(output)
	if err != nil {
		return err
	}
	// Add newline if not present
	if len(output) > 0 && output[len(output)-1] != '\n' {
		w.w.Write([]byte{'\n'})
	}
	return nil
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
