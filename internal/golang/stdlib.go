// Package golang provides Go-specific utilities for Wildcat.
package golang

import (
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var (
	gorootOnce sync.Once
	goroot     string
)

// GOROOT returns the Go root directory for the current machine.
// Result is cached after first call.
func GOROOT() string {
	gorootOnce.Do(func() {
		out, err := exec.Command("go", "env", "GOROOT").Output()
		if err == nil {
			goroot = strings.TrimSpace(string(out))
		}
	})
	return goroot
}

// IsStdlibPath checks if a file path is from the Go standard library.
func IsStdlibPath(path string) bool {
	root := GOROOT()
	if root == "" {
		return false
	}
	srcDir := filepath.Join(root, "src")
	return strings.HasPrefix(path, srcDir)
}

// IsStdlibImport checks if an import path is from the standard library.
// Standard library packages don't have dots in their first path component.
func IsStdlibImport(importPath string) bool {
	if importPath == "" {
		return false
	}
	firstSlash := strings.Index(importPath, "/")
	if firstSlash == -1 {
		// Single component like "fmt", "errors" - check for dots
		return !strings.Contains(importPath, ".")
	}
	// Check first component like "net" in "net/http"
	firstPart := importPath[:firstSlash]
	return !strings.Contains(firstPart, ".")
}
