package output

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriter_Write(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, false)

	resp := CallersResponse{
		Query: QueryInfo{
			Command:  "callers",
			Target:   "config.Load",
			Resolved: "github.com/user/proj/config.Load",
		},
		Target: TargetInfo{
			Symbol: "config.Load",
			File:   "/path/to/config.go",
			Line:   15,
		},
		Results: []Result{
			{
				Symbol: "main.main",
				File:   "/path/to/main.go",
				Line:   23,
				InTest: false,
			},
		},
		Summary: Summary{
			Count:     1,
			Packages:  []string{"main"},
			InTests:   0,
			Truncated: false,
		},
	}

	if err := w.Write(resp); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Verify it's valid JSON
	var parsed CallersResponse
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("parse output: %v", err)
	}

	if parsed.Query.Command != "callers" {
		t.Errorf("command = %q, want %q", parsed.Query.Command, "callers")
	}
	if parsed.Target.Symbol != "config.Load" {
		t.Errorf("symbol = %q, want %q", parsed.Target.Symbol, "config.Load")
	}
	if len(parsed.Results) != 1 {
		t.Errorf("results count = %d, want 1", len(parsed.Results))
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

func TestMarshal(t *testing.T) {
	data := Result{
		Symbol:  "test.Func",
		File:    "/path/to/file.go",
		Line:    10,
		Snippet: "func Func() {}",
		InTest:  false,
	}

	compact, err := Marshal(data, false)
	if err != nil {
		t.Fatalf("marshal compact: %v", err)
	}

	pretty, err := Marshal(data, true)
	if err != nil {
		t.Fatalf("marshal pretty: %v", err)
	}

	// Pretty should be longer
	if len(pretty) <= len(compact) {
		t.Errorf("pretty output should be longer than compact")
	}

	// Both should parse to same data
	var c, p Result
	json.Unmarshal(compact, &c)
	json.Unmarshal(pretty, &p)

	if c.Symbol != p.Symbol || c.File != p.File || c.Line != p.Line {
		t.Errorf("compact and pretty should have same data")
	}
}
