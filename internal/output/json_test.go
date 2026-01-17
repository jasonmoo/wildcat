package output

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriter_Write(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, false)

	resp := TreeResponse{
		Query: TreeQuery{
			Command:   "tree",
			Target:    "config.Load",
			Depth:     3,
			Direction: "up",
		},
		Paths: [][]string{
			{"main.main", "config.Load"},
		},
		Packages: []TreePackage{
			{
				Package: "main",
				Dir:     "/path/to",
				Symbols: []TreeFunction{
					{Name: "main", Signature: "func main()", Definition: "main.go:10:15"},
				},
			},
			{
				Package: "config",
				Dir:     "/path/to",
				Symbols: []TreeFunction{
					{Name: "Load", Signature: "func Load() error", Definition: "config.go:20:25"},
				},
			},
		},
		Summary: TreeSummary{
			PathCount:       1,
			MaxDepthReached: 2,
			Truncated:       false,
		},
	}

	if err := w.Write(resp); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Verify it's valid JSON
	var parsed TreeResponse
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("parse output: %v", err)
	}

	if parsed.Query.Command != "tree" {
		t.Errorf("command = %q, want %q", parsed.Query.Command, "tree")
	}
	if len(parsed.Paths) != 1 {
		t.Errorf("paths count = %d, want 1", len(parsed.Paths))
	}
}

func TestWriter_WriteError(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, false)

	err := w.WriteError(
		"symbol_not_found",
		"Cannot resolve 'config.Laod'",
		[]string{"config.Load", "config.LoadFromFile"},
		map[string]any{"searched": []string{"./..."}},
	)
	if err != nil {
		t.Fatalf("write error: %v", err)
	}

	var parsed ErrorResponse
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("parse output: %v", err)
	}

	if parsed.Error.Code != "symbol_not_found" {
		t.Errorf("code = %q, want %q", parsed.Error.Code, "symbol_not_found")
	}
	if len(parsed.Error.Suggestions) != 2 {
		t.Errorf("suggestions count = %d, want 2", len(parsed.Error.Suggestions))
	}
}

func TestWriter_PrettyPrint(t *testing.T) {
	var compact, pretty bytes.Buffer

	compactW := NewWriter(&compact, false)
	prettyW := NewWriter(&pretty, true)

	data := map[string]string{"key": "value"}

	if err := compactW.Write(data); err != nil {
		t.Fatalf("write compact: %v", err)
	}
	if err := prettyW.Write(data); err != nil {
		t.Fatalf("write pretty: %v", err)
	}

	// Pretty should be longer (has newlines and indentation)
	if len(pretty.Bytes()) <= len(compact.Bytes()) {
		t.Errorf("pretty output should be longer than compact")
	}

	// Both should be valid JSON
	var c, p map[string]string
	if err := json.Unmarshal(compact.Bytes(), &c); err != nil {
		t.Fatalf("parse compact: %v", err)
	}
	if err := json.Unmarshal(pretty.Bytes(), &p); err != nil {
		t.Fatalf("parse pretty: %v", err)
	}
}

