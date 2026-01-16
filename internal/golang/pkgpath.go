package golang

import (
	"fmt"
	"go/build"
	"path/filepath"
	"runtime/debug"
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
//   - Bare paths: internal/lsp -> resolved via module path
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

	pkg, err := build.Import(path, srcDir, build.FindOnly)
	if err != nil {
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			return "", fmt.Errorf("cannot resolve package %q: build info unavailable", path)
		}
		pkg, err = build.Import(filepath.Join(bi.Main.Path, path), srcDir, build.FindOnly)
	}
	if err == nil {
		return pkg.ImportPath, nil
	}
	return "", fmt.Errorf("cannot resolve package %q: %w", path, err)
}
