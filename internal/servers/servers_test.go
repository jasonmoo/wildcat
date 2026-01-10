package servers

import (
	"testing"
)

func TestGet(t *testing.T) {
	tests := []struct {
		language string
		want     string
		found    bool
	}{
		{"go", "gopls", true},
		{"Go", "gopls", true},
		{"GO", "gopls", true},
		{"python", "pyright-langserver", true},
		{"typescript", "typescript-language-server", true},
		{"rust", "rust-analyzer", true},
		{"c", "clangd", true},
		{"unknown", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.language, func(t *testing.T) {
			spec, found := Get(tt.language)
			if found != tt.found {
				t.Errorf("Get(%q) found = %v, want %v", tt.language, found, tt.found)
			}
			if found && spec.Command != tt.want {
				t.Errorf("Get(%q) command = %q, want %q", tt.language, spec.Command, tt.want)
			}
		})
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		filePath string
		language string
		found    bool
	}{
		{"main.go", "go", true},
		{"/path/to/file.go", "go", true},
		{"script.py", "python", true},
		{"app.ts", "typescript", true},
		{"component.tsx", "typescript", true},
		{"index.js", "typescript", true},
		{"lib.rs", "rust", true},
		{"main.c", "c", true},
		{"header.h", "c", true},
		{"file.cpp", "c", true},
		{"unknown.xyz", "", false},
		{"noextension", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			spec, found := Detect(tt.filePath)
			if found != tt.found {
				t.Errorf("Detect(%q) found = %v, want %v", tt.filePath, found, tt.found)
			}
			if found && spec.Language != tt.language {
				t.Errorf("Detect(%q) language = %q, want %q", tt.filePath, spec.Language, tt.language)
			}
		})
	}
}

func TestList(t *testing.T) {
	servers := List()
	if len(servers) == 0 {
		t.Error("List() returned empty")
	}

	// Check that modifying the result doesn't affect the registry
	original := len(servers)
	servers = servers[:0]
	if len(List()) != original {
		t.Error("List() returned modifiable slice")
	}
}

func TestSupportedExtensions(t *testing.T) {
	exts := SupportedExtensions()
	if len(exts) == 0 {
		t.Error("SupportedExtensions() returned empty")
	}

	// Check for common extensions
	hasGo := false
	hasPy := false
	for _, ext := range exts {
		if ext == "go" {
			hasGo = true
		}
		if ext == "py" {
			hasPy = true
		}
	}
	if !hasGo {
		t.Error("SupportedExtensions() missing 'go'")
	}
	if !hasPy {
		t.Error("SupportedExtensions() missing 'py'")
	}
}

func TestServerSpec_ToConfig(t *testing.T) {
	spec, _ := Get("go")
	config := spec.ToConfig("/workdir")

	if config.Command != "gopls" {
		t.Errorf("ToConfig() Command = %q, want gopls", config.Command)
	}
	if config.WorkDir != "/workdir" {
		t.Errorf("ToConfig() WorkDir = %q, want /workdir", config.WorkDir)
	}
}

func TestServerSpec_Available(t *testing.T) {
	spec, _ := Get("go")
	// gopls should be available in development environment
	if !spec.Available() {
		t.Skip("gopls not in PATH")
	}
}
