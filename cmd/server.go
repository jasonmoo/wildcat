package cmd

import (
	"fmt"

	"github.com/jasonmoo/wildcat/internal/lsp"
	"github.com/jasonmoo/wildcat/internal/servers"
)

var (
	globalLanguage string
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&globalLanguage, "language", "l", "", "Language (go, python, typescript, rust, c)")
}

// GetServerConfig returns the LSP server configuration for the specified or detected language.
func GetServerConfig(workDir string) (lsp.ServerConfig, error) {
	var spec *servers.ServerSpec

	if globalLanguage != "" {
		// Explicit language flag
		s, found := servers.Get(globalLanguage)
		if !found {
			available := servers.List()
			langs := make([]string, len(available))
			for i, a := range available {
				langs[i] = a.Language
			}
			return lsp.ServerConfig{}, fmt.Errorf("unknown language %q, available: %v", globalLanguage, langs)
		}
		spec = s
	} else {
		// Default to Go for now
		// Future: could detect from go.mod, package.json, Cargo.toml, etc.
		s, _ := servers.Get("go")
		spec = s
	}

	if !spec.Available() {
		return lsp.ServerConfig{}, fmt.Errorf("language server %q not found in PATH", spec.Command)
	}

	return spec.ToConfig(workDir), nil
}
