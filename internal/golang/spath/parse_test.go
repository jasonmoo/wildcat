package spath

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     *Path
		wantErr  bool
	}{
		// Simple cases
		{
			name:  "simple package and symbol",
			input: "golang.Symbol",
			want: &Path{
				Package: "golang",
				Symbol:  "Symbol",
			},
		},
		{
			name:  "package with slash",
			input: "internal/golang.Symbol",
			want: &Path{
				Package: "internal/golang",
				Symbol:  "Symbol",
			},
		},
		{
			name:  "full module path",
			input: "github.com/jasonmoo/wildcat/internal/golang.Symbol",
			want: &Path{
				Package: "github.com/jasonmoo/wildcat/internal/golang",
				Symbol:  "Symbol",
			},
		},

		// Stdlib
		{
			name:  "stdlib simple",
			input: "io.Reader",
			want: &Path{
				Package: "io",
				Symbol:  "Reader",
			},
		},
		{
			name:  "stdlib nested",
			input: "encoding/json.Marshal",
			want: &Path{
				Package: "encoding/json",
				Symbol:  "Marshal",
			},
		},
		{
			name:  "stdlib go/ast",
			input: "go/ast.Node",
			want: &Path{
				Package: "go/ast",
				Symbol:  "Node",
			},
		},

		// Methods
		{
			name:  "method",
			input: "golang.Symbol.String",
			want: &Path{
				Package: "golang",
				Symbol:  "Symbol",
				Method:  "String",
			},
		},
		{
			name:  "method with full path",
			input: "github.com/jasonmoo/wildcat/internal/golang.Symbol.String",
			want: &Path{
				Package: "github.com/jasonmoo/wildcat/internal/golang",
				Symbol:  "Symbol",
				Method:  "String",
			},
		},
		{
			name:  "interface method",
			input: "io.Reader.Read",
			want: &Path{
				Package: "io",
				Symbol:  "Reader",
				Method:  "Read",
			},
		},

		// Subpaths
		{
			name:  "field access",
			input: "golang.Symbol/fields[Name]",
			want: &Path{
				Package: "golang",
				Symbol:  "Symbol",
				Segments: []Segment{
					{Category: "fields", Selector: "Name"},
				},
			},
		},
		{
			name:  "field by index",
			input: "golang.Symbol/fields[0]",
			want: &Path{
				Package: "golang",
				Symbol:  "Symbol",
				Segments: []Segment{
					{Category: "fields", Selector: "0", IsIndex: true},
				},
			},
		},
		{
			name:  "param access",
			input: "golang.WalkReferences/params[ctx]",
			want: &Path{
				Package: "golang",
				Symbol:  "WalkReferences",
				Segments: []Segment{
					{Category: "params", Selector: "ctx"},
				},
			},
		},
		{
			name:  "returns access",
			input: "encoding/json.Marshal/returns[0]",
			want: &Path{
				Package: "encoding/json",
				Symbol:  "Marshal",
				Segments: []Segment{
					{Category: "returns", Selector: "0", IsIndex: true},
				},
			},
		},
		{
			name:  "body access",
			input: "golang.Symbol.String/body",
			want: &Path{
				Package: "golang",
				Symbol:  "Symbol",
				Method:  "String",
				Segments: []Segment{
					{Category: "body"},
				},
			},
		},
		{
			name:  "doc access",
			input: "golang.Symbol/doc",
			want: &Path{
				Package: "golang",
				Symbol:  "Symbol",
				Segments: []Segment{
					{Category: "doc"},
				},
			},
		},
		{
			name:  "receiver access",
			input: "golang.Symbol.String/receiver",
			want: &Path{
				Package: "golang",
				Symbol:  "Symbol",
				Method:  "String",
				Segments: []Segment{
					{Category: "receiver"},
				},
			},
		},

		// Chained subpaths
		{
			name:  "field tag",
			input: "golang.Symbol/fields[Name]/tag",
			want: &Path{
				Package: "golang",
				Symbol:  "Symbol",
				Segments: []Segment{
					{Category: "fields", Selector: "Name"},
					{Category: "tag"},
				},
			},
		},
		{
			name:  "field tag key",
			input: "golang.Symbol/fields[Name]/tag[json]",
			want: &Path{
				Package: "golang",
				Symbol:  "Symbol",
				Segments: []Segment{
					{Category: "fields", Selector: "Name"},
					{Category: "tag", Selector: "json"},
				},
			},
		},
		{
			name:  "param type",
			input: "golang.WalkReferences/params[ctx]/type",
			want: &Path{
				Package: "golang",
				Symbol:  "WalkReferences",
				Segments: []Segment{
					{Category: "params", Selector: "ctx"},
					{Category: "type"},
				},
			},
		},
		{
			name:  "receiver name",
			input: "golang.Symbol.String/receiver/name",
			want: &Path{
				Package: "golang",
				Symbol:  "Symbol",
				Method:  "String",
				Segments: []Segment{
					{Category: "receiver"},
					{Category: "name"},
				},
			},
		},
		{
			name:  "typeparam constraint",
			input: "golang.Min/typeparams[T]/constraint",
			want: &Path{
				Package: "golang",
				Symbol:  "Min",
				Segments: []Segment{
					{Category: "typeparams", Selector: "T"},
					{Category: "constraint"},
				},
			},
		},

		// Interface method via /methods[]
		{
			name:  "interface method via slash",
			input: "io.Reader/methods[Read]",
			want: &Path{
				Package: "io",
				Symbol:  "Reader",
				Segments: []Segment{
					{Category: "methods", Selector: "Read"},
				},
			},
		},
		{
			name:  "interface method by index",
			input: "io.Reader/methods[0]",
			want: &Path{
				Package: "io",
				Symbol:  "Reader",
				Segments: []Segment{
					{Category: "methods", Selector: "0", IsIndex: true},
				},
			},
		},

		// Errors
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "no symbol",
			input:   "golang",
			wantErr: true,
		},
		{
			name:    "invalid category",
			input:   "golang.Symbol/invalid[X]",
			wantErr: true,
		},
		{
			name:    "unclosed bracket",
			input:   "golang.Symbol/fields[Name",
			wantErr: true,
		},
		{
			name:    "empty selector",
			input:   "golang.Symbol/fields[]",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Parse(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("Parse(%q) unexpected error: %v", tt.input, err)
				return
			}

			// Compare
			if got.Package != tt.want.Package {
				t.Errorf("Package = %q, want %q", got.Package, tt.want.Package)
			}
			if got.Symbol != tt.want.Symbol {
				t.Errorf("Symbol = %q, want %q", got.Symbol, tt.want.Symbol)
			}
			if got.Method != tt.want.Method {
				t.Errorf("Method = %q, want %q", got.Method, tt.want.Method)
			}
			if len(got.Segments) != len(tt.want.Segments) {
				t.Errorf("Segments len = %d, want %d", len(got.Segments), len(tt.want.Segments))
			} else {
				for i := range got.Segments {
					if got.Segments[i] != tt.want.Segments[i] {
						t.Errorf("Segments[%d] = %+v, want %+v", i, got.Segments[i], tt.want.Segments[i])
					}
				}
			}
		})
	}
}

func TestPathString(t *testing.T) {
	tests := []struct {
		path *Path
		want string
	}{
		{
			path: &Path{Package: "golang", Symbol: "Symbol"},
			want: "golang.Symbol",
		},
		{
			path: &Path{Package: "golang", Symbol: "Symbol", Method: "String"},
			want: "golang.Symbol.String",
		},
		{
			path: &Path{
				Package: "golang",
				Symbol:  "Symbol",
				Segments: []Segment{
					{Category: "fields", Selector: "Name"},
				},
			},
			want: "golang.Symbol/fields[Name]",
		},
		{
			path: &Path{
				Package: "golang",
				Symbol:  "Symbol",
				Segments: []Segment{
					{Category: "fields", Selector: "Name"},
					{Category: "tag", Selector: "json"},
				},
			},
			want: "golang.Symbol/fields[Name]/tag[json]",
		},
		{
			path: &Path{
				Package: "golang",
				Symbol:  "Symbol",
				Method:  "String",
				Segments: []Segment{
					{Category: "body"},
				},
			},
			want: "golang.Symbol.String/body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.path.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseRoundtrip(t *testing.T) {
	// Paths that should round-trip exactly
	paths := []string{
		"golang.Symbol",
		"golang.Symbol.String",
		"encoding/json.Marshal",
		"io.Reader.Read",
		"golang.Symbol/fields[Name]",
		"golang.Symbol/fields[0]",
		"golang.WalkReferences/params[ctx]",
		"golang.Symbol.String/body",
		"golang.Symbol/fields[Name]/tag[json]",
		"github.com/jasonmoo/wildcat/internal/golang.Symbol",
		"github.com/jasonmoo/wildcat/internal/golang.Symbol.String/receiver/type",
	}

	for _, input := range paths {
		t.Run(input, func(t *testing.T) {
			parsed, err := Parse(input)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", input, err)
			}
			got := parsed.String()
			if got != input {
				t.Errorf("Round-trip failed:\n  input:  %q\n  output: %q", input, got)
			}
		})
	}
}
