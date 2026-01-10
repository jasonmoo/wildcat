package lsp

import (
	"testing"
)

func TestFileURI(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/file.go", "file:///home/user/file.go"},
		{"/tmp/test.py", "file:///tmp/test.py"},
	}

	for _, tt := range tests {
		got := FileURI(tt.path)
		// FileURI uses filepath.Abs, so we just check prefix
		if got[:7] != "file://" {
			t.Errorf("FileURI(%q) = %q, want file:// prefix", tt.path, got)
		}
	}
}

func TestURIToPath(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"file:///home/user/file.go", "/home/user/file.go"},
		{"file:///tmp/test.py", "/tmp/test.py"},
		{"/already/path", "/already/path"},
	}

	for _, tt := range tests {
		got := URIToPath(tt.uri)
		if got != tt.want {
			t.Errorf("URIToPath(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}

func TestSymbolKindConstants(t *testing.T) {
	// Verify some common symbol kinds match LSP spec
	if SymbolKindFunction != 12 {
		t.Errorf("SymbolKindFunction = %d, want 12", SymbolKindFunction)
	}
	if SymbolKindMethod != 6 {
		t.Errorf("SymbolKindMethod = %d, want 6", SymbolKindMethod)
	}
	if SymbolKindClass != 5 {
		t.Errorf("SymbolKindClass = %d, want 5", SymbolKindClass)
	}
	if SymbolKindInterface != 11 {
		t.Errorf("SymbolKindInterface = %d, want 11", SymbolKindInterface)
	}
}
