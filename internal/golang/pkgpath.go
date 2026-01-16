package golang

import (
	"fmt"
	"go/build"
	"path/filepath"
	"runtime/debug"
	"strings"
)

// ResolvePackagePath attempts to resolve a user-provided package path to its
// full import path. It handles:
//   - Full import paths: github.com/user/repo/pkg -> as-is
//   - Relative paths: ./internal/lsp -> resolved to full import path
//   - Bare paths: internal/lsp -> tries ./internal/lsp
//   - Short names: fmt, errors -> stdlib
func ResolvePackagePath(path, srcDir string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("cannot resolve package: empty path")
	}
	if strings.Contains(path, "...") {
		return "", fmt.Errorf("cannot resolve package %q: wildcard patterns not supported", path)
	}
	pkg, err := build.Import(path, srcDir, build.FindOnly)
	if err != nil {
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			panic("not ok")
		}
		pkg, err = build.Import(filepath.Join(bi.Main.Path, path), srcDir, build.FindOnly)
	}
	if err == nil {
		return pkg.ImportPath, nil
	}
	return "", fmt.Errorf("cannot resolve package %q: %w", path, err)
}
