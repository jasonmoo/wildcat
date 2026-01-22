// Package golang provides Go-specific utilities for Wildcat.
package golang

import (
	"os/exec"
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

