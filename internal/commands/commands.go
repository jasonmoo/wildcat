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

	Error struct {
		Code        string                 `json:"code"`
		Error       error                  `json:"error"`
		Suggestions []string               `json:"suggestions"`
		Context     map[string]interface{} `json:"context"`
	}
)

var _ Result = (*Error)(nil)

func NewErrorf(code, format string, a ...interface{}) *Error {
	return &Error{
		Code:  code,
		Error: fmt.Errorf(format, a...),
	}
}

func (e *Error) MarshalJSON() ([]byte, error) {
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

func (e *Error) MarshalMarkdown() ([]byte, error) {
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
