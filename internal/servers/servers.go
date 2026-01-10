package servers

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jasonmoo/wildcat/internal/lsp"
)

// ServerSpec defines how to start a language server.
type ServerSpec struct {
	Language     string         // e.g., "go", "python"
	Name         string         // human-readable name
	Command      string         // binary name
	Args         []string       // startup arguments
	Extensions   []string       // file extensions (without dot)
	InitOptions  map[string]any // LSP initializationOptions
	Capabilities []string       // required LSP capabilities
}

// registry holds all known language server configurations.
var registry = []ServerSpec{
	{
		Language:   "go",
		Name:       "gopls",
		Command:    "gopls",
		Args:       []string{"serve"},
		Extensions: []string{"go"},
		Capabilities: []string{
			"textDocument/references",
			"textDocument/definition",
			"callHierarchy/incomingCalls",
			"callHierarchy/outgoingCalls",
		},
	},
	{
		Language:   "python",
		Name:       "pyright",
		Command:    "pyright-langserver",
		Args:       []string{"--stdio"},
		Extensions: []string{"py", "pyi"},
		Capabilities: []string{
			"textDocument/references",
			"textDocument/definition",
		},
	},
	{
		Language:   "typescript",
		Name:       "typescript-language-server",
		Command:    "typescript-language-server",
		Args:       []string{"--stdio"},
		Extensions: []string{"ts", "tsx", "js", "jsx"},
		Capabilities: []string{
			"textDocument/references",
			"textDocument/definition",
			"callHierarchy/incomingCalls",
			"callHierarchy/outgoingCalls",
		},
	},
	{
		Language:   "rust",
		Name:       "rust-analyzer",
		Command:    "rust-analyzer",
		Args:       []string{},
		Extensions: []string{"rs"},
		Capabilities: []string{
			"textDocument/references",
			"textDocument/definition",
			"callHierarchy/incomingCalls",
			"callHierarchy/outgoingCalls",
		},
	},
	{
		Language:   "c",
		Name:       "clangd",
		Command:    "clangd",
		Args:       []string{},
		Extensions: []string{"c", "h", "cpp", "hpp", "cc", "cxx"},
		Capabilities: []string{
			"textDocument/references",
			"textDocument/definition",
			"callHierarchy/incomingCalls",
			"callHierarchy/outgoingCalls",
		},
	},
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
func Get(language string) (*ServerSpec, bool) {
	lang := strings.ToLower(language)
	for i := range registry {
		if registry[i].Language == lang {
			return &registry[i], true
		}
	}
	return nil, false
}

// Detect finds the appropriate server for a file path.
func Detect(filePath string) (*ServerSpec, bool) {
	ext := strings.TrimPrefix(filepath.Ext(filePath), ".")
	if ext == "" {
		return nil, false
	}
	ext = strings.ToLower(ext)

	for i := range registry {
		for _, e := range registry[i].Extensions {
			if e == ext {
				return &registry[i], true
			}
		}
	}
	return nil, false
}

// List returns all registered server specs.
func List() []ServerSpec {
	result := make([]ServerSpec, len(registry))
	copy(result, registry)
	return result
}

// Available returns all servers that are currently available in PATH.
func Available() []ServerSpec {
	var result []ServerSpec
	for _, s := range registry {
		if s.Available() {
			result = append(result, s)
		}
	}
	return result
}

// SupportedExtensions returns all file extensions we can handle.
func SupportedExtensions() []string {
	seen := make(map[string]bool)
	var exts []string
	for _, s := range registry {
		for _, ext := range s.Extensions {
			if !seen[ext] {
				seen[ext] = true
				exts = append(exts, ext)
			}
		}
	}
	return exts
}
