package symbols

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input       string
		wantPkg     string
		wantType    string
		wantPointer bool
		wantName    string
		wantErr     bool
	}{
		// Simple function name
		{
			input:    "Load",
			wantName: "Load",
		},
		// Package.Function
		{
			input:    "config.Load",
			wantPkg:  "config",
			wantName: "Load",
		},
		// Type.Method
		{
			input:    "Server.Start",
			wantType: "Server",
			wantName: "Start",
		},
		// Pointer receiver
		{
			input:       "(*Server).Start",
			wantType:    "Server",
			wantPointer: true,
			wantName:    "Start",
		},
		// Non-pointer receiver with parens
		{
			input:    "(Server).Start",
			wantType: "Server",
			wantName: "Start",
		},
		// Path/package.Function
		{
			input:    "internal/config.Load",
			wantPkg:  "internal/config",
			wantName: "Load",
		},
		// Full path
		{
			input:    "github.com/user/proj/config.Load",
			wantPkg:  "github.com/user/proj/config",
			wantName: "Load",
		},
		// Lowercase first letter = package
		{
			input:    "myPkg.myFunc",
			wantPkg:  "myPkg",
			wantName: "myFunc",
		},
		// Empty input
		{
			input:   "",
			wantErr: true,
		},
		// Unclosed paren
		{
			input:   "(*Server.Start",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := Parse(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.Package != tt.wantPkg {
				t.Errorf("Package = %q, want %q", got.Package, tt.wantPkg)
			}
			if got.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", got.Type, tt.wantType)
			}
			if got.Pointer != tt.wantPointer {
				t.Errorf("Pointer = %v, want %v", got.Pointer, tt.wantPointer)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.Raw != tt.input {
				t.Errorf("Raw = %q, want %q", got.Raw, tt.input)
			}
		})
	}
}

func TestQuery_String(t *testing.T) {
	tests := []struct {
		query *Query
		want  string
	}{
		{
			query: &Query{Name: "Load"},
			want:  "Load",
		},
		{
			query: &Query{Package: "config", Name: "Load"},
			want:  "config.Load",
		},
		{
			query: &Query{Type: "Server", Name: "Start"},
			want:  "Server.Start",
		},
		{
			query: &Query{Type: "Server", Pointer: true, Name: "Start"},
			want:  "(*Server).Start",
		},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.query.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQuery_IsMethod(t *testing.T) {
	tests := []struct {
		query *Query
		want  bool
	}{
		{&Query{Name: "Load"}, false},
		{&Query{Package: "config", Name: "Load"}, false},
		{&Query{Type: "Server", Name: "Start"}, true},
		{&Query{Type: "Server", Pointer: true, Name: "Start"}, true},
	}

	for _, tt := range tests {
		got := tt.query.IsMethod()
		if got != tt.want {
			t.Errorf("IsMethod() for %v = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestParseError(t *testing.T) {
	err := &ParseError{Input: "bad.input", Message: "something wrong"}
	got := err.Error()

	if got != "invalid symbol 'bad.input': something wrong" {
		t.Errorf("Error() = %q", got)
	}
}
