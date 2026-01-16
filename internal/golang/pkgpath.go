package golang

import (
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// reservedPatterns are Go toolchain patterns that expand to multiple packages.
// These are not actual importable packages - they're special patterns used by
// go list, go build, etc. We reject them to avoid unexpected behavior where
// packages.Load("cmd") would load all cmd/* packages from the Go toolchain.
// See: go help packages
var reservedPatterns = []string{"all", "cmd", "main", "std", "tool"}

// ResolvePackagePath attempts to resolve a user-provided package path to its
// full import path. It handles:
//   - Full import paths: github.com/user/repo/pkg -> as-is
//   - Relative paths: ./internal/lsp -> resolved to full import path
//   - Bare paths: internal/lsp -> tries ./internal/lsp if exists locally
//   - Short names: fmt, errors -> stdlib
//
// Returns an error for:
//   - Empty paths
//   - Wildcard patterns (containing "...")
//   - Reserved Go patterns (all, cmd, main, std, tool)
func ResolvePackagePath(path, srcDir string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("cannot resolve package: empty path")
	}
	if strings.Contains(path, "...") {
		return "", fmt.Errorf("cannot resolve package %q: wildcard patterns not supported", path)
	}
	if slices.Contains(reservedPatterns, path) {
		return "", fmt.Errorf("cannot resolve %q: reserved Go pattern %v (expands to multiple packages); use ./%s for local package", path, reservedPatterns, path)
	}

	// Try direct import (stdlib, full paths, ./ paths)
	pkg, err := build.Import(path, srcDir, build.FindOnly)
	if err == nil {
		return pkg.ImportPath, nil
	}

	// If path doesn't start with "." but exists locally, try with "./" prefix
	// This handles bare paths like "internal/lsp" -> "./internal/lsp"
	if !strings.HasPrefix(path, ".") {
		localPath := filepath.Join(srcDir, path)
		if _, statErr := os.Stat(localPath); statErr == nil {
			pkg, err = build.Import("./"+path, srcDir, build.FindOnly)
			if err == nil {
				return pkg.ImportPath, nil
			}
		}
	}

	return "", fmt.Errorf("cannot resolve package %q: %w", path, err)
}
