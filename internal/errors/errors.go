// Package errors provides structured error types for Wildcat.
package errors

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Code represents an error code.
type Code string

const (
	CodeSymbolNotFound  Code = "symbol_not_found"
	CodeAmbiguousSymbol Code = "ambiguous_symbol"
	CodePackageNotFound Code = "package_not_found"
	CodeParseError      Code = "parse_error"
	CodeLoadError       Code = "load_error"
	CodeLSPError        Code = "lsp_error"
	CodeTimeout         Code = "timeout"
	CodeServerNotFound  Code = "server_not_found"
)

// WildcatError is a structured error with suggestions for self-correction.
type WildcatError struct {
	Code        Code           `json:"code"`
	Message     string         `json:"message"`
	Suggestions []string       `json:"suggestions,omitempty"`
	Context     map[string]any `json:"context,omitempty"`
}

// Error implements the error interface.
func (e *WildcatError) Error() string {
	if len(e.Suggestions) > 0 {
		return fmt.Sprintf("%s (did you mean: %s?)", e.Message, strings.Join(e.Suggestions, ", "))
	}
	return e.Message
}

// ToJSON returns the error as JSON bytes.
func (e *WildcatError) ToJSON() ([]byte, error) {
	wrapper := struct {
		Error *WildcatError `json:"error"`
	}{Error: e}
	return json.MarshalIndent(wrapper, "", "  ")
}

// NewSymbolNotFound creates a symbol not found error with suggestions.
func NewSymbolNotFound(symbol string, suggestions []string) *WildcatError {
	return &WildcatError{
		Code:        CodeSymbolNotFound,
		Message:     fmt.Sprintf("Cannot resolve symbol '%s'", symbol),
		Suggestions: suggestions,
		Context:     map[string]any{"symbol": symbol},
	}
}

// NewAmbiguousSymbol creates an ambiguous symbol error with candidates.
func NewAmbiguousSymbol(symbol string, candidates []string) *WildcatError {
	return &WildcatError{
		Code:        CodeAmbiguousSymbol,
		Message:     fmt.Sprintf("Ambiguous symbol '%s' matches multiple definitions", symbol),
		Suggestions: candidates,
		Context:     map[string]any{"symbol": symbol, "matches": len(candidates)},
	}
}

// NewPackageNotFound creates a package not found error.
func NewPackageNotFound(pkg string) *WildcatError {
	return &WildcatError{
		Code:    CodePackageNotFound,
		Message: fmt.Sprintf("Package '%s' not found", pkg),
		Context: map[string]any{"package": pkg},
	}
}

// NewParseError creates a parse error.
func NewParseError(file string, line int, msg string) *WildcatError {
	return &WildcatError{
		Code:    CodeParseError,
		Message: fmt.Sprintf("Parse error in %s:%d: %s", file, line, msg),
		Context: map[string]any{"file": file, "line": line},
	}
}

// NewLoadError creates a load error.
func NewLoadError(patterns []string, err error) *WildcatError {
	return &WildcatError{
		Code:    CodeLoadError,
		Message: fmt.Sprintf("Failed to load packages: %v", err),
		Context: map[string]any{"patterns": patterns},
	}
}

// NewLSPError creates an LSP error.
func NewLSPError(method string, err error) *WildcatError {
	return &WildcatError{
		Code:    CodeLSPError,
		Message: fmt.Sprintf("LSP error in %s: %v", method, err),
		Context: map[string]any{"method": method},
	}
}

// NewTimeout creates a timeout error.
func NewTimeout(operation string) *WildcatError {
	return &WildcatError{
		Code:    CodeTimeout,
		Message: fmt.Sprintf("Operation timed out: %s", operation),
		Context: map[string]any{"operation": operation},
	}
}

// NewServerNotFound creates a server not found error.
func NewServerNotFound(language, server string) *WildcatError {
	return &WildcatError{
		Code:    CodeServerNotFound,
		Message: fmt.Sprintf("Language server '%s' not found for %s", server, language),
		Context: map[string]any{"language": language, "server": server},
	}
}

// SuggestSimilar finds strings similar to the target from a list of candidates.
// Uses Levenshtein distance, returns up to limit suggestions.
func SuggestSimilar(target string, candidates []string, limit int) []string {
	if len(candidates) == 0 || limit <= 0 {
		return nil
	}

	// Calculate distances
	type scored struct {
		s        string
		distance int
	}
	var scored_list []scored

	targetLower := strings.ToLower(target)
	for _, c := range candidates {
		cLower := strings.ToLower(c)
		d := levenshtein(targetLower, cLower)
		// Only include if reasonably similar (distance less than half the target length)
		if d <= len(target)/2+2 {
			scored_list = append(scored_list, scored{c, d})
		}
	}

	// Sort by distance
	sort.Slice(scored_list, func(i, j int) bool {
		return scored_list[i].distance < scored_list[j].distance
	})

	// Return top suggestions
	result := make([]string, 0, limit)
	for i := 0; i < len(scored_list) && i < limit; i++ {
		result = append(result, scored_list[i].s)
	}

	return result
}

// levenshtein calculates the Levenshtein distance between two strings.
func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Create matrix
	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(a)][len(b)]
}
