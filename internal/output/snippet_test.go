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
			want:         "\thello()",
		},
		{
			name:         "single line with context",
			line:         4,
			contextLines: 1,
			want:         "func main() {\n\thello()\n}",
		},
		{
			name:         "first line with context",
			line:         1,
			contextLines: 1,
			want:         "package main\n",
		},
		{
			name:         "last line with context",
			line:         9,
			contextLines: 1,
			want:         "\tprintln(\"Hello\")\n}",
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

func TestSnippetExtractor_ExtractLine(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	content := "line 1\nline 2\nline 3\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	extractor := NewSnippetExtractor()

	tests := []struct {
		line    int
		want    string
		wantErr bool
	}{
		{1, "line 1", false},
		{2, "line 2", false},
		{3, "line 3", false},
		{0, "", true},
		{100, "", true},
	}

	for _, tt := range tests {
		got, err := extractor.ExtractLine(testFile, tt.line)
		if tt.wantErr {
			if err == nil {
				t.Errorf("line %d: expected error, got nil", tt.line)
			}
			continue
		}
		if err != nil {
			t.Errorf("line %d: unexpected error: %v", tt.line, err)
			continue
		}
		if got != tt.want {
			t.Errorf("line %d: got %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestSnippetExtractor_ExtractCallExpr(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

	content := "package main\n\nfunc main() { config.Load(path) }\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	extractor := NewSnippetExtractor()

	// Extract "config.Load(path)" - starts at char 14, ends at 31
	got, err := extractor.ExtractCallExpr(testFile, 3, 14, 31)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "config.Load(path)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"foo_test.go", true},
		{"foo.go", false},
		{"test_foo.py", true},
		{"foo_test.py", true},
		{"foo.py", false},
		{"foo.test.ts", true},
		{"foo.spec.ts", true},
		{"foo.ts", false},
		{"foo_test.rs", true},
		{"foo.rs", false},
		{"/path/to/foo_test.go", true},
		{"/path/to/foo.go", false},
	}

	for _, tt := range tests {
		got := IsTestFile(tt.path)
		if got != tt.want {
			t.Errorf("IsTestFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestSnippetExtractor_Cache(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	content := "line 1\nline 2\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	extractor := NewSnippetExtractor()

	// First call should cache
	_, err := extractor.ExtractLine(testFile, 1)
	if err != nil {
		t.Fatalf("first extract: %v", err)
	}

	// Modify file (cache should still return old content)
	if err := os.WriteFile(testFile, []byte("modified\n"), 0644); err != nil {
		t.Fatalf("modify file: %v", err)
	}

	got, err := extractor.ExtractLine(testFile, 1)
	if err != nil {
		t.Fatalf("second extract: %v", err)
	}
	if got != "line 1" {
		t.Errorf("cache not working: got %q, want %q", got, "line 1")
	}

	// Clear cache
	extractor.ClearCache()

	got, err = extractor.ExtractLine(testFile, 1)
	if err != nil {
		t.Fatalf("third extract: %v", err)
	}
	if got != "modified" {
		t.Errorf("after cache clear: got %q, want %q", got, "modified")
	}
}
