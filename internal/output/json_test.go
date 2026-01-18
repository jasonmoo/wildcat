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
			Command: "tree",
			Target:  "config.Load",
			Up:      2,
			Down:    2,
		},
		Target: TreeTargetInfo{
			Symbol:     "config.Load",
			Signature:  "func Load() error",
			Definition: "/path/to/config.go:20:25",
		},
		Callers: []*CallNode{
			{Symbol: "main.main", Callsite: "/path/to/main.go:10"},
		},
		Calls: []*CallNode{
			{Symbol: "config.validate", Callsite: "/path/to/config.go:25"},
		},
		Definitions: []TreePackage{
			{
				Package: "main",
				Dir:     "/path/to",
				Symbols: []TreeFunction{
					{Symbol: "main.main", Signature: "func main()", Definition: "main.go:10:15"},
				},
			},
			{
				Package: "config",
				Dir:     "/path/to",
				Symbols: []TreeFunction{
					{Symbol: "config.Load", Signature: "func Load() error", Definition: "config.go:20:25"},
				},
			},
		},
		Summary: TreeSummary{
			Callers:      1,
			Callees:      1,
			MaxUpDepth:   1,
			MaxDownDepth: 1,
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
	if parsed.Target.Symbol != "config.Load" {
		t.Errorf("target symbol = %q, want config.Load", parsed.Target.Symbol)
	}
	if len(parsed.Callers) != 1 {
		t.Errorf("callers count = %d, want 1", len(parsed.Callers))
	}
	if len(parsed.Calls) != 1 {
		t.Errorf("calls count = %d, want 1", len(parsed.Calls))
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

