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
		Tree: &TreeNode{
			Symbol: "config.Load",
			Calls: []*TreeNode{
				{Symbol: "main.main", Line: 10},
			},
		},
		Packages: []TreePackage{
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
			TotalCalls:      1,
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
	if parsed.Tree == nil || parsed.Tree.Symbol != "config.Load" {
		t.Errorf("tree root symbol = %v, want config.Load", parsed.Tree)
	}
	if len(parsed.Tree.Calls) != 1 {
		t.Errorf("tree calls count = %d, want 1", len(parsed.Tree.Calls))
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

