package output

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnippetExtractor_Extract(t *testing.T) {
	// Create a temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

	content := `package main

func main() {
	hello()
}

func hello() {
	println("Hello")
}
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	extractor := NewSnippetExtractor()

	tests := []struct {
		name         string
		line         int
		contextLines int
		want         string
		wantErr      bool
	}{
		{
			name:         "single line no context",
			line:         4,
			contextLines: 0,
			want:         "\thello()\n",
		},
		{
			name:         "single line with context",
			line:         4,
			contextLines: 1,
			want:         "func main() {\n\thello()\n}\n",
		},
		{
			name:         "first line with context",
			line:         1,
			contextLines: 1,
			want:         "package main\n\n",
		},
		{
			name:         "last line with context",
			line:         9,
			contextLines: 1,
			want:         "\tprintln(\"Hello\")\n}\n",
		},
		{
			name:         "out of range",
			line:         100,
			contextLines: 0,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractor.Extract(testFile, tt.line, tt.contextLines)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

