package errors

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWildcatError_Error(t *testing.T) {
	tests := []struct {
		name        string
		err         *WildcatError
		wantContain string
	}{
		{
			name:        "no suggestions",
			err:         NewPackageNotFound("foo/bar"),
			wantContain: "foo/bar",
		},
		{
			name:        "with suggestions",
			err:         NewSymbolNotFound("config.Laod", []string{"config.Load"}),
			wantContain: "did you mean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if !strings.Contains(got, tt.wantContain) {
				t.Errorf("Error() = %q, want to contain %q", got, tt.wantContain)
			}
		})
	}
}

func TestWildcatError_ToJSON(t *testing.T) {
	err := NewSymbolNotFound("config.Laod", []string{"config.Load", "config.LoadFile"})

	jsonBytes, jsonErr := err.ToJSON()
	if jsonErr != nil {
		t.Fatalf("ToJSON() error: %v", jsonErr)
	}

	// Parse and verify structure
	var parsed struct {
		Error struct {
			Code        string   `json:"code"`
			Message     string   `json:"message"`
			Suggestions []string `json:"suggestions"`
		} `json:"error"`
	}

	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}

	if parsed.Error.Code != string(CodeSymbolNotFound) {
		t.Errorf("code = %q, want %q", parsed.Error.Code, CodeSymbolNotFound)
	}
	if len(parsed.Error.Suggestions) != 2 {
		t.Errorf("suggestions count = %d, want 2", len(parsed.Error.Suggestions))
	}
}

func TestNewSymbolNotFound(t *testing.T) {
	err := NewSymbolNotFound("test.Func", []string{"test.Function"})

	if err.Code != CodeSymbolNotFound {
		t.Errorf("Code = %q, want %q", err.Code, CodeSymbolNotFound)
	}
	if !strings.Contains(err.Message, "test.Func") {
		t.Errorf("Message should contain symbol")
	}
	if err.Context["symbol"] != "test.Func" {
		t.Errorf("Context[symbol] = %v, want %q", err.Context["symbol"], "test.Func")
	}
}

func TestNewAmbiguousSymbol(t *testing.T) {
	candidates := []string{"pkg1.Load", "pkg2.Load"}
	err := NewAmbiguousSymbol("Load", candidates)

	if err.Code != CodeAmbiguousSymbol {
		t.Errorf("Code = %q, want %q", err.Code, CodeAmbiguousSymbol)
	}
	if len(err.Suggestions) != 2 {
		t.Errorf("Suggestions count = %d, want 2", len(err.Suggestions))
	}
}

func TestNewParseError(t *testing.T) {
	err := NewParseError("/path/to/file.go", 42, "unexpected token")

	if err.Code != CodeParseError {
		t.Errorf("Code = %q, want %q", err.Code, CodeParseError)
	}
	if !strings.Contains(err.Message, "file.go:42") {
		t.Errorf("Message should contain file:line")
	}
}

func TestNewLSPError(t *testing.T) {
	err := NewLSPError("workspace/symbol", nil)

	if err.Code != CodeLSPError {
		t.Errorf("Code = %q, want %q", err.Code, CodeLSPError)
	}
}

func TestSuggestSimilar(t *testing.T) {
	candidates := []string{
		"config.Load",
		"config.LoadFile",
		"config.LoadFromPath",
		"server.Start",
		"server.Stop",
	}

	tests := []struct {
		target string
		limit  int
		want   []string
	}{
		{
			target: "config.Laod",
			limit:  3,
			want:   []string{"config.Load"},
		},
		{
			target: "config.Load",
			limit:  2,
			want:   []string{"config.Load", "config.LoadFile"},
		},
		{
			target: "server.Strat",
			limit:  1,
			want:   []string{"server.Start"},
		},
		{
			target: "completely.Different",
			limit:  3,
			want:   nil, // Nothing similar enough
		},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := SuggestSimilar(tt.target, candidates, tt.limit)

			if tt.want == nil {
				if len(got) > 0 {
					t.Errorf("SuggestSimilar(%q) = %v, want nil", tt.target, got)
				}
				return
			}

			// Check first result matches
			if len(got) == 0 {
				t.Errorf("SuggestSimilar(%q) returned empty, want %v", tt.target, tt.want)
				return
			}
			if got[0] != tt.want[0] {
				t.Errorf("SuggestSimilar(%q)[0] = %q, want %q", tt.target, got[0], tt.want[0])
			}
		})
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "adc", 1},
		{"kitten", "sitting", 3},
		{"Load", "Laod", 2},
	}

	for _, tt := range tests {
		got := levenshtein(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
