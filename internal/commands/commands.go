package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

type (
	Command[T any] interface {
		Execute(context.Context, *Wildcat, ...func(T) error) (Result, *Error)
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

func NewErrorf(code, format string, a ...interface{}) *Error {
	return &Error{
		Code:  code,
		Error: fmt.Errorf(format, a...),
	}
}
