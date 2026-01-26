package golang

import "fmt"

// SymbolNotFoundError is returned when a symbol lookup fails.
// Demonstrates a type that implements the builtin error interface.
type SymbolNotFoundError struct {
	Query string
}

func (e *SymbolNotFoundError) Error() string {
	return fmt.Sprintf("symbol not found: %s", e.Query)
}

// LookupError is an interface alias for error.
// Demonstrates StdlibEquivalent detection in the project command.
type LookupError error
