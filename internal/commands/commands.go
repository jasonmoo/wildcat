package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/spf13/cobra"
)

type (
	Command[T any] interface {
		Execute(context.Context, *Wildcat, ...func(T) error) (Result, error)
		Cmd() *cobra.Command
		README() string
	}

	Result interface {
		SetDiagnostics([]Diagnostic)
		MarshalMarkdown() ([]byte, error)
		MarshalJSON() ([]byte, error)
	}

	Diagnostic struct {
		Level   string `json:"level"`             // "warning", "info"
		Package string `json:"package,omitempty"` // package path (if applicable)
		Message string `json:"message"`
	}

	// Suggestion represents a fuzzy match suggestion with type info.
	Suggestion struct {
		Symbol string `json:"symbol"` // qualified: pkg.Name or pkg.Type.Method
		Kind   string `json:"kind"`   // func, method, type, interface, const, var
	}

	ErrorResult struct {
		Code        string                 `json:"code"`
		Error       error                  `json:"error"`
		Suggestions []string               `json:"suggestions"`
		Context     map[string]interface{} `json:"context"`
		Diagnostics []Diagnostic           `json:"diagnostics"`
	}
)

func RunCommand[T any](cmd *cobra.Command, c Command[T], opts ...func(T) error) error {

	for _, o := range opts {
		if err := o(c.(T)); err != nil {
			return fmt.Errorf("internal_error: failed to apply opt: %w", err)
		}
	}

	wc, err := LoadWildcat(cmd.Context(), ".")
	if err != nil {
		return err
	}

	result, err := c.Execute(cmd.Context(), wc, opts...)
	if err != nil {
		return err
	}
	result.SetDiagnostics(wc.Diagnostics)

	if outputFlag := cmd.Flag("output"); outputFlag != nil && outputFlag.Changed && outputFlag.Value.String() == "json" {
		data, err := result.MarshalJSON()
		if err != nil {
			return err
		}
		os.Stdout.Write(data)
		os.Stdout.WriteString("\n")
		return nil
	}

	md, err := result.MarshalMarkdown()
	if err != nil {
		return err
	}
	os.Stdout.Write(md)
	os.Stdout.WriteString("\n")
	return nil

}

func NewInfoDiagnostic(pi *golang.PackageIdentifier, msg string) Diagnostic {
	var pkg string
	if pi != nil {
		pkg = pi.PkgPath
	}
	return Diagnostic{
		Level:   "info",
		Package: pkg,
		Message: msg,
	}
}
func NewWarningDiagnostic(pi *golang.PackageIdentifier, msg string) Diagnostic {
	var pkg string
	if pi != nil {
		pkg = pi.PkgPath
	}
	return Diagnostic{
		Level:   "warning",
		Package: pkg,
		Message: msg,
	}
}

var _ Result = (*ErrorResult)(nil)

// FormatDiagnosticsMarkdown renders diagnostics as markdown to w.
// Does nothing if there are no diagnostics.
func FormatDiagnosticsMarkdown(w io.Writer, ds []Diagnostic) {
	if len(ds) == 0 {
		return
	}
	fmt.Fprintf(w, "\n## ⚠️ Diagnostics (%d)\n\n", len(ds))
	fmt.Fprintln(w, "**Note:** Issues below may affect analysis. Results shown are accurate for successfully loaded packages but may be incomplete.")
	fmt.Fprintln(w)
	for _, d := range ds {
		if d.Package != "" {
			fmt.Fprintf(w, "- [%s] %s: %s\n", d.Level, d.Package, d.Message)
		} else {
			fmt.Fprintf(w, "- [%s] %s\n", d.Level, d.Message)
		}
	}
}

func NewErrorResultf(code, format string, a ...interface{}) *ErrorResult {
	return &ErrorResult{
		Code:  code,
		Error: fmt.Errorf(format, a...),
	}
}

func (e *ErrorResult) SetDiagnostics(ds []Diagnostic) {
	e.Diagnostics = ds
}

func (e *ErrorResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Code        string                 `json:"code"`
		Error       string                 `json:"error"`
		Suggestions []string               `json:"suggestions,omitempty"`
		Context     map[string]interface{} `json:"context,omitempty"`
		Diagnostics []Diagnostic           `json:"diagnostics,omitempty"`
	}{
		Code:        e.Code,
		Error:       e.Error.Error(),
		Suggestions: e.Suggestions,
		Context:     e.Context,
		Diagnostics: e.Diagnostics,
	})
}

func (e *ErrorResult) MarshalMarkdown() ([]byte, error) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Error: (%s) %s\n", e.Code, e.Error)
	if len(e.Suggestions) > 0 {
		buf.WriteString("Suggestions:\n")
		for _, s := range e.Suggestions {
			buf.WriteString(" - ")
			buf.WriteString(s)
			buf.WriteByte('\n')
		}
	}
	if len(e.Context) > 0 {
		buf.WriteString("Context:\n")
		for k, v := range e.Context {
			fmt.Fprintf(&buf, " - %s: %v\n", k, v)
		}
	}
	if len(e.Diagnostics) > 0 {
		buf.WriteString("\nDiagnostics:\n")
		for _, d := range e.Diagnostics {
			fmt.Fprintf(&buf, " - [%s] %s\n", d.Level, d.Message)
		}
	}
	return buf.Bytes(), nil
}
