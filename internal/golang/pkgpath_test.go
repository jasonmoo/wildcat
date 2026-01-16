package golang

import (
	"os"
	"path/filepath"
	"testing"
)

func projectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	// Test runs from internal/golang, go up two levels
	return filepath.Clean(filepath.Join(cwd, "../.."))
}

func TestResolvePackagePath(t *testing.T) {
	root := projectRoot()

	tests := []struct {
		name      string
		path      string
		srcDir    string
		wantPath  string // empty means we just check it resolves
		wantErr   bool
	}{
		// Stdlib packages
		{
			name:     "stdlib fmt",
			path:     "fmt",
			srcDir:   root,
			wantPath: "fmt",
		},
		{
			name:     "stdlib errors",
			path:     "errors",
			srcDir:   root,
			wantPath: "errors",
		},
		{
			name:     "stdlib nested",
			path:     "encoding/json",
			srcDir:   root,
			wantPath: "encoding/json",
		},

		// Stdlib precedence - these names exist locally but stdlib should win
		// Project has internal/errors, but "errors" should resolve to stdlib
		{
			name:     "stdlib precedence - errors over internal/errors",
			path:     "errors",
			srcDir:   root,
			wantPath: "errors", // NOT github.com/jasonmoo/wildcat/internal/errors
		},
		// Project has internal/config, but "config" should error (not in stdlib)
		// rather than silently resolve to local
		{
			name:    "no implicit local - config not in stdlib",
			path:    "config",
			srcDir:  root,
			wantErr: true, // should NOT resolve to internal/config implicitly
		},

		// Full import paths (external)
		{
			name:     "full import path - cobra",
			path:     "github.com/spf13/cobra",
			srcDir:   root,
			wantPath: "github.com/spf13/cobra",
		},

		// Relative paths within project
		{
			name:   "relative path - internal/lsp",
			path:   "internal/lsp",
			srcDir: root,
		},
		{
			name:   "relative path - internal/config",
			path:   "internal/config",
			srcDir: root,
		},
		{
			name:   "relative path with dot - ./internal/lsp",
			path:   "./internal/lsp",
			srcDir: root,
		},

		// Non-existent packages
		{
			name:    "non-existent package",
			path:    "github.com/doesnotexist/fake",
			srcDir:  root,
			wantErr: true,
		},
		{
			name:    "non-existent relative",
			path:    "internal/notreal",
			srcDir:  root,
			wantErr: true,
		},
		{
			name:    "typo in stdlib",
			path:    "fmtt",
			srcDir:  root,
			wantErr: true,
		},

		// Edge cases
		{
			name:    "empty path",
			path:    "",
			srcDir:  root,
			wantErr: true,
		},
		{
			name:    "wildcard internal/...",
			path:    "internal/...",
			srcDir:  root,
			wantErr: true,
		},
		{
			name:    "wildcard ./...",
			path:    "./...",
			srcDir:  root,
			wantErr: true,
		},
		{
			name:    "wildcard middle internal/.../model",
			path:    "internal/.../model",
			srcDir:  root,
			wantErr: true,
		},
		{
			name:   "current package - golang",
			path:   "internal/golang",
			srcDir: root,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolvePackagePath(tt.path, tt.srcDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ResolvePackagePath(%q, %q) = %q, want error", tt.path, tt.srcDir, got)
				} else {
					t.Logf("expected error: %v", err)
				}
				return
			}

			if err != nil {
				t.Errorf("ResolvePackagePath(%q, %q) error = %v", tt.path, tt.srcDir, err)
				return
			}

			t.Logf("resolved: %q -> %q", tt.path, got)

			if tt.wantPath != "" && got != tt.wantPath {
				t.Errorf("ResolvePackagePath(%q, %q) = %q, want %q", tt.path, tt.srcDir, got, tt.wantPath)
			}
		})
	}
}
