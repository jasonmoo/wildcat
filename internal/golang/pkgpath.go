package golang

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
// full module-qualified import path. It handles:
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

	// Try go list first - it returns the canonical module-qualified import path
	if importPath, err := goListImportPath(path, srcDir); err == nil {
		return importPath, nil
	}

	// If path doesn't start with "." but exists locally, try with "./" prefix
	// This handles bare paths like "internal/lsp" -> "./internal/lsp"
	if !strings.HasPrefix(path, ".") {
		localPath := filepath.Join(srcDir, path)
		if _, statErr := os.Stat(localPath); statErr == nil {
			if importPath, err := goListImportPath("./"+path, srcDir); err == nil {
				return importPath, nil
			}
		}
	}

	return "", fmt.Errorf("cannot resolve package %q", path)
}

// goListImportPath uses go list to get the canonical import path for a package.
func goListImportPath(path, srcDir string) (string, error) {
	cmd := exec.Command("go", "list", "-json", path)
	cmd.Dir = srcDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	var pkg struct {
		ImportPath string `json:"ImportPath"`
	}
	if err := json.Unmarshal(out, &pkg); err != nil {
		return "", err
	}
	return pkg.ImportPath, nil
}
