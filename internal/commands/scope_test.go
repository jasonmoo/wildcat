package commands

import (
	"testing"
)

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern string
		pkgPath string
		want    bool
	}{
		// Exact match (no pattern chars)
		{"internal", "internal", true},
		{"internal", "internal/lsp", false},

		// Go-style /... suffix
		{"internal/...", "internal", true},
		{"internal/...", "internal/lsp", true},
		{"internal/...", "internal/commands/search", true},
		{"internal/...", "cmd", false},
		{"internal/...", "internalfoo", false}, // not a prefix match

		// Single * (matches one segment)
		{"internal/*", "internal/lsp", true},
		{"internal/*", "internal/commands", true},
		{"internal/*", "internal/commands/search", false}, // * doesn't cross /
		{"internal/*", "internal", false},

		// Double ** (matches zero or more segments)
		{"internal/**", "internal/lsp", true},
		{"internal/**", "internal/commands/search", true},
		{"**/lsp", "internal/lsp", true},
		{"**/lsp", "foo/bar/lsp", true},
		{"**/lsp", "lsp", true},

		// Complex patterns
		{"**/commands/*", "internal/commands/search", true},
		{"**/commands/*", "internal/commands/search/foo", false},
		{"**/internal/**/search", "foo/internal/commands/search", true},
	}

	for _, tt := range tests {
		got, err := matchPattern(tt.pattern, tt.pkgPath)
		if err != nil {
			t.Errorf("matchPattern(%q, %q) error: %v", tt.pattern, tt.pkgPath, err)
			continue
		}
		if got != tt.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.pattern, tt.pkgPath, got, tt.want)
		}
	}
}

func TestIsPattern(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"internal", false},
		{"internal/lsp", false},
		{"internal/...", true},
		{"internal/*", true},
		{"internal/**", true},
		{"**/lsp", true},
		{"internal/?sp", true},
		{"internal/[abc]", true},
	}

	for _, tt := range tests {
		got := isPattern(tt.s)
		if got != tt.want {
			t.Errorf("isPattern(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}
