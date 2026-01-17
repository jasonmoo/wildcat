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
