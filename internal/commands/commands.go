package commands

import (
	"context"
	"fmt"
)

type (
	Command[T any] interface {
		Execute(context.Context, ...func(T) error) (Result, *Error)
		Help() Help
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

	Help struct {
		Use   string
		Short string
		Long  string
	}
)

func NewErrorf(code, format string, a ...interface{}) *Error {
	return &Error{
		Code:  code,
		Error: fmt.Errorf(format, a...),
	}
}
