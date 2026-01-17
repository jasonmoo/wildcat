package servers

import (
	"os/exec"

	"github.com/jasonmoo/wildcat/internal/lsp"
)

// ServerSpec defines how to start a language server.
type ServerSpec struct {
	Command string   // binary name
	Args    []string // startup arguments
}

// gopls is the Go language server configuration.
var gopls = ServerSpec{
	Command: "gopls",
	Args:    []string{"serve"},
}

// Available checks if the server binary is in PATH.
func (s *ServerSpec) Available() bool {
	_, err := exec.LookPath(s.Command)
	return err == nil
}

// ToConfig converts a ServerSpec to an LSP ServerConfig.
func (s *ServerSpec) ToConfig(workDir string) lsp.ServerConfig {
	return lsp.ServerConfig{
		Command: s.Command,
		Args:    s.Args,
		WorkDir: workDir,
	}
}

// Get returns the server spec for a language.
// Currently only "go" is supported.
func Get(language string) (*ServerSpec, bool) {
	if language == "go" {
		return &gopls, true
	}
	return nil, false
}
