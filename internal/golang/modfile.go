package golang

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

// DirectDeps returns the set of direct (non-indirect) module dependencies
// for the Go module rooted at the given directory.
// Returns nil if go.mod cannot be found or parsed.
func DirectDeps(dir string) map[string]bool {
	gomod := findGoMod(dir)
	if gomod == "" {
		return nil
	}

	data, err := os.ReadFile(gomod)
	if err != nil {
		return nil
	}

	f, err := modfile.ParseLax(gomod, data, nil)
	if err != nil {
		return nil
	}

	deps := make(map[string]bool)
	for _, req := range f.Require {
		if !req.Indirect {
			deps[req.Mod.Path] = true
		}
	}
	return deps
}

// findGoMod walks up from dir looking for go.mod.
func findGoMod(dir string) string {
	for {
		gomod := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(gomod); err == nil {
			return gomod
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// ModuleFromModPath extracts the module path from a file path in go/pkg/mod.
// e.g., "/home/user/go/pkg/mod/github.com/foo/bar@v1.0.0/file.go" -> "github.com/foo/bar"
// Returns empty string if path is not in go/pkg/mod.
func ModuleFromModPath(filePath string) string {
	// Look for /pkg/mod/ in the path
	idx := strings.Index(filePath, "/pkg/mod/")
	if idx == -1 {
		return ""
	}

	// Get everything after /pkg/mod/
	rest := filePath[idx+len("/pkg/mod/"):]

	// Find the @ version separator
	atIdx := strings.Index(rest, "@")
	if atIdx == -1 {
		return ""
	}

	return rest[:atIdx]
}

// IsDirectDep checks if a file path is from stdlib or a direct dependency.
// Returns true if the interface should be included in output.
func IsDirectDep(filePath string, directDeps map[string]bool) bool {
	// Stdlib is always included
	if IsStdlibPath(filePath) {
		return true
	}

	// Check if it's from go/pkg/mod (external dep)
	mod := ModuleFromModPath(filePath)
	if mod == "" {
		// Not in go/pkg/mod - probably project-local, include it
		return true
	}

	// Check if it's a direct dependency
	return directDeps[mod]
}
