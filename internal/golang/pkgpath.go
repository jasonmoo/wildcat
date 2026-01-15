package golang

import (
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"strings"
)

// ResolvePackagePath attempts to resolve a user-provided package path to its
// full import path. It handles:
//   - Full import paths: github.com/user/repo/pkg -> as-is
//   - Relative paths: ./internal/lsp -> resolved to full import path
//   - Bare paths: internal/lsp -> tries ./internal/lsp
//   - Short names: fmt, errors -> stdlib
func ResolvePackagePath(path, srcDir string) (string, error) {
	// Try build.Import first - handles full import paths, ./ paths, and stdlib
	pkg, err := build.Import(path, srcDir, build.FindOnly)
	if err == nil {
		return pkg.ImportPath, nil
	}

	// If failed and path doesn't start with "." but exists locally,
	// it's a bare relative path like "internal/lsp" - try with "./" prefix
	if !strings.HasPrefix(path, ".") {
		if _, statErr := os.Stat(filepath.Join(srcDir, path)); statErr == nil {
			pkg, err = build.Import("./"+path, srcDir, build.FindOnly)
			if err == nil {
				return pkg.ImportPath, nil
			}
		}
	}

	return "", fmt.Errorf("cannot resolve package %q: %w", path, err)
}
