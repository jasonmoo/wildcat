package cmd

import (
	"fmt"
	"io"

	"github.com/jasonmoo/wildcat/internal/lsp"
	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/jasonmoo/wildcat/internal/servers"
)

// GetWriter returns an output writer with the configured format.
func GetWriter(w io.Writer) (*output.Writer, error) {
	if globalOutput == "" || globalOutput == "json" {
		return output.NewWriter(w, true), nil
	}
	return output.NewWriterWithFormat(w, globalOutput)
}

// GetServerConfig returns the LSP server configuration for Go.
func GetServerConfig(workDir string) (lsp.ServerConfig, error) {
	spec, _ := servers.Get("go")

	if !spec.Available() {
		return lsp.ServerConfig{}, fmt.Errorf("language server %q not found in PATH", spec.Command)
	}

	return spec.ToConfig(workDir), nil
}
