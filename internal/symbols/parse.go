// Package symbols provides symbol parsing and resolution for Wildcat.
package symbols

import (
	"strings"
)

// Query represents a parsed symbol query.
type Query struct {
	Package  string // Package name or path (e.g., "config", "internal/config")
	Type     string // Receiver type for methods (e.g., "Server")
	Pointer  bool   // Whether receiver is pointer (*Type)
	Name     string // Function or method name (e.g., "Load", "Start")
	Raw      string // Original input string
}

// Parse parses a symbol string into a Query.
// Supported formats:
//   - Function           -> name only, resolve in context
//   - pkg.Function       -> package.name
//   - Type.Method        -> type.method
//   - (*Type).Method     -> pointer receiver method
//   - path/to/pkg.Func   -> full path
func Parse(input string) (*Query, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, &ParseError{Input: input, Message: "empty symbol"}
	}

	q := &Query{Raw: input}

	// Check for pointer receiver: (*Type).Method
	if strings.HasPrefix(input, "(*") {
		closeIdx := strings.Index(input, ")")
		if closeIdx < 0 {
			return nil, &ParseError{Input: input, Message: "unclosed parenthesis in pointer receiver"}
		}
		q.Type = input[2:closeIdx]
		q.Pointer = true

		// Rest should be .Method
		rest := input[closeIdx+1:]
		if !strings.HasPrefix(rest, ".") {
			return nil, &ParseError{Input: input, Message: "expected '.' after pointer receiver"}
		}
		q.Name = rest[1:]
		return q, nil
	}

	// Check for parenthesis without pointer: (Type).Method
	if strings.HasPrefix(input, "(") {
		closeIdx := strings.Index(input, ")")
		if closeIdx < 0 {
			return nil, &ParseError{Input: input, Message: "unclosed parenthesis"}
		}
		q.Type = input[1:closeIdx]

		rest := input[closeIdx+1:]
		if !strings.HasPrefix(rest, ".") {
			return nil, &ParseError{Input: input, Message: "expected '.' after receiver"}
		}
		q.Name = rest[1:]
		return q, nil
	}

	// Split by last dot to separate package/type from name
	lastDot := strings.LastIndex(input, ".")
	if lastDot < 0 {
		// No dot - just a name
		q.Name = input
		return q, nil
	}

	prefix := input[:lastDot]
	q.Name = input[lastDot+1:]

	// Check if prefix looks like a path (contains /)
	if strings.Contains(prefix, "/") {
		// Full path: path/to/pkg.Name
		// The last segment before . is the package
		lastSlash := strings.LastIndex(prefix, "/")
		q.Package = prefix[:lastSlash+1] + prefix[lastSlash+1:]
		return q, nil
	}

	// Check if prefix is capitalized (Type) or lowercase (package)
	if len(prefix) > 0 && isUppercase(prefix[0]) {
		// Looks like Type.Method
		q.Type = prefix
	} else {
		// Looks like pkg.Func
		q.Package = prefix
	}

	return q, nil
}

// String returns a string representation of the query.
func (q *Query) String() string {
	if q.Pointer {
		return "(*" + q.Type + ")." + q.Name
	}
	if q.Type != "" {
		return q.Type + "." + q.Name
	}
	if q.Package != "" {
		return q.Package + "." + q.Name
	}
	return q.Name
}

// IsMethod returns true if this is a method query.
func (q *Query) IsMethod() bool {
	return q.Type != ""
}

// ParseError represents a symbol parsing error.
type ParseError struct {
	Input   string
	Message string
}

func (e *ParseError) Error() string {
	return "invalid symbol '" + e.Input + "': " + e.Message
}

// isUppercase returns true if the byte is an uppercase ASCII letter.
func isUppercase(b byte) bool {
	return b >= 'A' && b <= 'Z'
}
