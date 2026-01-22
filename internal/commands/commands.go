package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

type (
	Command[T any] interface {
		Execute(context.Context, *Wildcat, ...func(T) error) (Result, error)
		Cmd() *cobra.Command
		README() string
	}

	Result interface {
		MarshalMarkdown() ([]byte, error)
		MarshalJSON() ([]byte, error)
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
	}
)

var _ Result = (*ErrorResult)(nil)

func NewErrorResultf(code, format string, a ...interface{}) *ErrorResult {
	return &ErrorResult{
		Code:  code,
		Error: fmt.Errorf(format, a...),
	}
}

func (e *ErrorResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Code        string                 `json:"code"`
		Error       string                 `json:"error"`
		Suggestions []string               `json:"suggestions,omitempty"`
		Context     map[string]interface{} `json:"context,omitempty"`
	}{
		Code:        e.Code,
		Error:       e.Error.Error(),
		Suggestions: e.Suggestions,
		Context:     e.Context,
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
	return buf.Bytes(), nil
}
