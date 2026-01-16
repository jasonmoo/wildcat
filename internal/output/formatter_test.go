package output

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONFormatter(t *testing.T) {
	f := &JSONFormatter{Pretty: true}

	data := map[string]any{"key": "value"}
	result, err := f.Format(data)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Invalid JSON output: %v", err)
	}

	if parsed["key"] != "value" {
		t.Errorf("Expected key=value, got %v", parsed["key"])
	}
}

func TestYAMLFormatter(t *testing.T) {
	f := &YAMLFormatter{}

	data := map[string]any{
		"name":  "test",
		"count": 42.0,
	}
	result, err := f.Format(data)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	output := string(result)
	if !strings.Contains(output, "name: test") {
		t.Errorf("Expected 'name: test' in output, got %s", output)
	}
	if !strings.Contains(output, "count: 42") {
		t.Errorf("Expected 'count: 42' in output, got %s", output)
	}
}

func TestMarkdownFormatter(t *testing.T) {
	f := &MarkdownFormatter{}

	data := map[string]any{
		"query": map[string]any{
			"command": "callers",
			"target":  "Execute",
		},
		"results": []any{
			map[string]any{
				"symbol": "main",
				"file":   "/path/to/main.go",
				"line":   10.0,
			},
		},
		"summary": map[string]any{
			"count": 1.0,
		},
	}
	result, err := f.Format(data)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	output := string(result)
	if !strings.Contains(output, "# Callers: Execute") {
		t.Errorf("Missing header, got: %s", output)
	}
	if !strings.Contains(output, "| Symbol | File | Line |") {
		t.Error("Missing table header")
	}
	if !strings.Contains(output, "| main |") {
		t.Error("Missing result row")
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()

	// Built-in formatters should be registered
	formatters := r.List()
	if len(formatters) == 0 {
		t.Error("Registry has no formatters")
	}

	// Get JSON formatter
	f, err := r.Get("json")
	if err != nil {
		t.Fatalf("Get(json) error = %v", err)
	}
	if f.Name() != "json" {
		t.Errorf("Expected json formatter, got %s", f.Name())
	}

	// Unknown formatter should error
	_, err = r.Get("unknown")
	if err == nil {
		t.Error("Expected error for unknown formatter")
	}
}
