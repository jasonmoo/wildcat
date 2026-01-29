package commands

import "testing"

// =============================================================================
// SPATH GRAMMAR REFERENCE (from path-syntax-grammar.ebnf)
// =============================================================================
//
// path = package_path , "." , symbol , [ subpath ]
//      | package_path                                 (* package-only path *)
//
// package_path = package_component , { "/" , package_component }
// package_component = identifier | identifier , "." , identifier , { "." , identifier }
//
// symbol = identifier , [ "." , identifier ]          (* symbol or type.method *)
//
// subpath = "/" , segment , { "/" , segment }
// segment = category , [ selector ]
// selector = "[" , ( identifier | integer ) , "]"
//
// VALID SPATH EXAMPLES:
//   - "io"                                    (package only)
//   - "encoding/json"                         (nested package)
//   - "pkg.Symbol"                            (package + symbol)
//   - "pkg.Type.Method"                       (package + type + method)
//   - "pkg.Func/params[ctx]"                  (with subpath)
//   - "pkg.Type/fields[Name]/tag[json]"       (nested subpath)
//
// INVALID SPATH EXAMPLES:
//   - "Symbol"                                (bare symbol - no package)
//   - ""                                      (empty)
//   - ".Symbol"                               (missing package)
//   - "pkg."                                  (missing symbol)
//
// =============================================================================

// TestPatternToRegex tests the conversion of glob patterns to regex patterns.
// Wildcard semantics:
//   - *    matches within a segment (stops at . / [ ])
//   - **   matches any chars across segments
//   - **/  matches any prefix ending with / (or nothing)
//   - /**  matches any suffix starting with / (or nothing)
func TestPatternToRegex(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		wantRe  string // expected regex pattern (without anchors)
	}{
		// === Literal patterns (no wildcards) ===
		{
			name:    "package only",
			pattern: "io",
			wantRe:  "io",
		},
		{
			name:    "nested package",
			pattern: "encoding/json",
			wantRe:  "encoding/json",
		},
		{
			name:    "package with domain",
			pattern: "github.com/user/repo",
			wantRe:  `github\.com/user/repo`,
		},
		{
			name:    "package and symbol",
			pattern: "pkg.Symbol",
			wantRe:  `pkg\.Symbol`,
		},
		{
			name:    "package symbol method",
			pattern: "pkg.Type.Method",
			wantRe:  `pkg\.Type\.Method`,
		},
		{
			name:    "full path with subpath",
			pattern: "pkg.Func/params[ctx]",
			wantRe:  `pkg\.Func/params\[ctx\]`,
		},
		{
			name:    "subpath with positional selector",
			pattern: "pkg.Func/returns[0]",
			wantRe:  `pkg\.Func/returns\[0\]`,
		},
		{
			name:    "category without selector",
			pattern: "pkg.Func/body",
			wantRe:  `pkg\.Func/body`,
		},
		{
			name:    "nested subpath",
			pattern: "pkg.Type/fields[Name]/tag[json]",
			wantRe:  `pkg\.Type/fields\[Name\]/tag\[json\]`,
		},

		// === Single star (*) - within segment ===
		{
			name:    "star in symbol name",
			pattern: "pkg.Sym*",
			wantRe:  `pkg\.Sym[^./\[\]]*`,
		},
		{
			name:    "star prefix symbol",
			pattern: "pkg.*Symbol",
			wantRe:  `pkg\.[^./\[\]]*Symbol`,
		},
		{
			name:    "star only symbol",
			pattern: "pkg.*",
			wantRe:  `pkg\.[^./\[\]]*`,
		},
		{
			name:    "star in package component",
			pattern: "internal/*",
			wantRe:  `internal/[^./\[\]]*`,
		},
		{
			name:    "star in selector",
			pattern: "pkg.Type/fields[*]",
			wantRe:  `pkg\.Type/fields\[[^./\[\]]*\]`,
		},
		{
			name:    "star in category",
			pattern: "pkg.Func/*",
			wantRe:  `pkg\.Func/[^./\[\]]*`,
		},

		// === Double star (**) - across segments ===
		{
			name:    "doublestar after dot",
			pattern: "pkg.**",
			wantRe:  `pkg\..*`,
		},
		{
			name:    "doublestar in middle",
			pattern: "pkg.**.Method",
			wantRe:  `pkg\..*\.Method`,
		},

		// === Double star with slash (**/  and /**) ===
		{
			name:    "doublestar slash prefix for package path",
			pattern: "**/golang.Symbol",
			wantRe:  `(.*/)?golang\.Symbol`,
		},
		{
			name:    "doublestar slash in package path",
			pattern: "internal/**/pkg.Symbol",
			wantRe:  `internal/(.*/)?pkg\.Symbol`,
		},
		{
			name:    "slash doublestar suffix",
			pattern: "pkg.Symbol/**",
			wantRe:  `pkg\.Symbol(/.*)?`,
		},
		{
			name:    "doublestar slash for subpath prefix",
			pattern: "**/fields[Name]",
			wantRe:  `(.*/)?fields\[Name\]`,
		},

		// === Combined wildcards ===
		{
			name:    "star and doublestar",
			pattern: "**/pkg.*",
			wantRe:  `(.*/)?pkg\.[^./\[\]]*`,
		},
		{
			name:    "complex pattern",
			pattern: "internal/**/Type.*/params[*]",
			wantRe:  `internal/(.*/)?Type\.[^./\[\]]*/params\[[^./\[\]]*\]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re, err := patternToRegex(tt.pattern)
			if err != nil {
				t.Fatalf("patternToRegex(%q) error: %v", tt.pattern, err)
			}
			want := "^" + tt.wantRe + "$"
			if re.String() != want {
				t.Errorf("patternToRegex(%q)\n  got:  %s\n  want: %s", tt.pattern, re.String(), want)
			}
		})
	}
}

// TestPatternMatchingPackages tests patterns against package-only spaths.
// Grammar: package_path = package_component , { "/" , package_component }
func TestPatternMatchingPackages(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		matches []string // valid package paths that should match
		rejects []string // valid package paths that should NOT match
	}{
		{
			name:    "exact package",
			pattern: "io",
			matches: []string{"io"},
			rejects: []string{"io/ioutil", "stdio", "myio"},
		},
		{
			name:    "nested package exact",
			pattern: "encoding/json",
			matches: []string{"encoding/json"},
			rejects: []string{"encoding", "encoding/json/v2", "myencoding/json"},
		},
		{
			name:    "package with domain",
			pattern: "github.com/user/repo",
			matches: []string{"github.com/user/repo"},
			rejects: []string{"github.com/user", "github.com/user/repo/pkg", "gitlab.com/user/repo"},
		},
		{
			name:    "package star suffix",
			pattern: "encoding/*",
			matches: []string{"encoding/json", "encoding/xml", "encoding/gob"},
			rejects: []string{"encoding", "encoding/json/v2"},
		},
		{
			name:    "package doublestar suffix",
			pattern: "internal/**",
			matches: []string{"internal", "internal/golang", "internal/commands/spath"},
			rejects: []string{"myinternal", "pkg/internal"},
		},
		{
			name:    "package doublestar prefix",
			pattern: "**/golang",
			matches: []string{"golang", "internal/golang", "foo/bar/golang"},
			rejects: []string{"golang/pkg", "mygolang"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re, err := patternToRegex(tt.pattern)
			if err != nil {
				t.Fatalf("patternToRegex(%q) error: %v", tt.pattern, err)
			}

			for _, path := range tt.matches {
				if !re.MatchString(path) {
					t.Errorf("pattern %q should match %q but didn't\n  regex: %s", tt.pattern, path, re.String())
				}
			}

			for _, path := range tt.rejects {
				if re.MatchString(path) {
					t.Errorf("pattern %q should NOT match %q but did\n  regex: %s", tt.pattern, path, re.String())
				}
			}
		})
	}
}

// TestPatternMatchingSymbols tests patterns against package.symbol spaths.
// Grammar: path = package_path , "." , symbol
func TestPatternMatchingSymbols(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		matches []string // valid spaths that should match
		rejects []string // valid spaths that should NOT match
	}{
		{
			name:    "exact symbol",
			pattern: "pkg.Symbol",
			matches: []string{"pkg.Symbol"},
			rejects: []string{"pkg.SymbolX", "pkg.MySymbol", "pkg.Symbol.Method", "pkg.Symbol/fields[X]"},
		},
		{
			name:    "symbol star suffix",
			pattern: "pkg.Sym*",
			matches: []string{"pkg.Symbol", "pkg.SymbolIndex", "pkg.Sym"},
			rejects: []string{"pkg.MySym", "pkg.Symbol.Method"},
		},
		{
			name:    "symbol star prefix",
			pattern: "pkg.*Index",
			matches: []string{"pkg.SymbolIndex", "pkg.PackageIndex", "pkg.Index"},
			rejects: []string{"pkg.IndexedSymbol", "pkg.SymbolIndex.Get"},
		},
		{
			name:    "any symbol in package",
			pattern: "pkg.*",
			matches: []string{"pkg.A", "pkg.Symbol", "pkg.WalkReferences"},
			rejects: []string{"pkg.Symbol.Method", "pkg.Func/params[x]", "otherpkg.Symbol"},
		},
		{
			name:    "exact method",
			pattern: "pkg.Type.Method",
			matches: []string{"pkg.Type.Method"},
			rejects: []string{"pkg.Type", "pkg.Type.MethodX", "pkg.Type.Method/body"},
		},
		{
			name:    "method star suffix",
			pattern: "pkg.Type.Get*",
			matches: []string{"pkg.Type.GetID", "pkg.Type.GetName", "pkg.Type.Get"},
			rejects: []string{"pkg.Type.SetID", "pkg.Type"},
		},
		{
			name:    "any method on type",
			pattern: "pkg.Type.*",
			matches: []string{"pkg.Type.Method", "pkg.Type.GetID", "pkg.Type.String"},
			rejects: []string{"pkg.Type", "pkg.Type.Method/body"},
		},
		{
			name:    "symbol with doublestar package prefix",
			pattern: "**.Symbol",
			matches: []string{"pkg.Symbol", "internal/golang.Symbol", "a/b/c.Symbol"},
			rejects: []string{"pkg.MySymbol", "pkg.Symbol.Method"},
		},
		{
			name:    "symbol with slash doublestar package prefix",
			pattern: "**/golang.Symbol",
			matches: []string{"golang.Symbol", "internal/golang.Symbol", "a/b/golang.Symbol"},
			rejects: []string{"pkg.Symbol", "mygolang.Symbol"},
		},
		{
			name:    "nested package with symbol",
			pattern: "internal/golang.*",
			matches: []string{"internal/golang.Symbol", "internal/golang.Package"},
			rejects: []string{"internal/golang", "internal/golang.Symbol.Method"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re, err := patternToRegex(tt.pattern)
			if err != nil {
				t.Fatalf("patternToRegex(%q) error: %v", tt.pattern, err)
			}

			for _, path := range tt.matches {
				if !re.MatchString(path) {
					t.Errorf("pattern %q should match %q but didn't\n  regex: %s", tt.pattern, path, re.String())
				}
			}

			for _, path := range tt.rejects {
				if re.MatchString(path) {
					t.Errorf("pattern %q should NOT match %q but did\n  regex: %s", tt.pattern, path, re.String())
				}
			}
		})
	}
}

// TestPatternMatchingSubpaths tests patterns with subpath navigation.
// Grammar: subpath = "/" , segment , { "/" , segment }
func TestPatternMatchingSubpaths(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		matches []string
		rejects []string
	}{
		// === Category only (no selector) ===
		{
			name:    "body category",
			pattern: "pkg.Func/body",
			matches: []string{"pkg.Func/body"},
			rejects: []string{"pkg.Func", "pkg.Func/params", "pkg.Func/body/x"},
		},
		{
			name:    "doc category",
			pattern: "pkg.Type/doc",
			matches: []string{"pkg.Type/doc"},
			rejects: []string{"pkg.Type", "pkg.Type/fields[X]/doc"},
		},
		{
			name:    "receiver category",
			pattern: "pkg.Type.Method/receiver",
			matches: []string{"pkg.Type.Method/receiver"},
			rejects: []string{"pkg.Type.Method", "pkg.Type.Method/receiver/name"},
		},

		// === Category with selector ===
		{
			name:    "params with name selector",
			pattern: "pkg.Func/params[ctx]",
			matches: []string{"pkg.Func/params[ctx]"},
			rejects: []string{"pkg.Func/params", "pkg.Func/params[0]", "pkg.Func/params[ctx]/type"},
		},
		{
			name:    "params with positional selector",
			pattern: "pkg.Func/params[0]",
			matches: []string{"pkg.Func/params[0]"},
			rejects: []string{"pkg.Func/params", "pkg.Func/params[ctx]", "pkg.Func/params[1]"},
		},
		{
			name:    "fields with name selector",
			pattern: "pkg.Type/fields[Name]",
			matches: []string{"pkg.Type/fields[Name]"},
			rejects: []string{"pkg.Type/fields", "pkg.Type/fields[ID]", "pkg.Type/fields[Name]/tag"},
		},
		{
			name:    "typeparams with selector",
			pattern: "pkg.Type/typeparams[T]",
			matches: []string{"pkg.Type/typeparams[T]"},
			rejects: []string{"pkg.Type/typeparams", "pkg.Type/typeparams[U]"},
		},
		{
			name:    "embeds with selector",
			pattern: "pkg.Type/embeds[Reader]",
			matches: []string{"pkg.Type/embeds[Reader]"},
			rejects: []string{"pkg.Type/embeds", "pkg.Type/embeds[Writer]"},
		},
		{
			name:    "methods with selector",
			pattern: "pkg.Interface/methods[Read]",
			matches: []string{"pkg.Interface/methods[Read]"},
			rejects: []string{"pkg.Interface/methods", "pkg.Interface/methods[Write]"},
		},

		// === Nested subpaths ===
		{
			name:    "param type",
			pattern: "pkg.Func/params[ctx]/type",
			matches: []string{"pkg.Func/params[ctx]/type"},
			rejects: []string{"pkg.Func/params[ctx]", "pkg.Func/params[ctx]/name"},
		},
		{
			name:    "field tag key",
			pattern: "pkg.Type/fields[Name]/tag[json]",
			matches: []string{"pkg.Type/fields[Name]/tag[json]"},
			rejects: []string{"pkg.Type/fields[Name]/tag", "pkg.Type/fields[Name]/tag[db]"},
		},
		{
			name:    "receiver type",
			pattern: "pkg.Type.Method/receiver/type",
			matches: []string{"pkg.Type.Method/receiver/type"},
			rejects: []string{"pkg.Type.Method/receiver", "pkg.Type.Method/receiver/name"},
		},
		{
			name:    "typeparam constraint",
			pattern: "pkg.Func/typeparams[T]/constraint",
			matches: []string{"pkg.Func/typeparams[T]/constraint"},
			rejects: []string{"pkg.Func/typeparams[T]"},
		},

		// === Wildcards in subpaths ===
		{
			name:    "any category",
			pattern: "pkg.Func/*",
			matches: []string{"pkg.Func/body", "pkg.Func/params", "pkg.Func/returns", "pkg.Func/doc"},
			rejects: []string{"pkg.Func", "pkg.Func/params[x]"},
		},
		{
			name:    "any selector in category",
			pattern: "pkg.Type/fields[*]",
			matches: []string{"pkg.Type/fields[Name]", "pkg.Type/fields[ID]", "pkg.Type/fields[0]"},
			rejects: []string{"pkg.Type/fields", "pkg.Type/fields[Name]/tag"},
		},
		{
			name:    "star in selector name",
			pattern: "pkg.Type/fields[Na*]",
			matches: []string{"pkg.Type/fields[Name]", "pkg.Type/fields[Namespace]", "pkg.Type/fields[Na]"},
			rejects: []string{"pkg.Type/fields[ID]", "pkg.Type/fields[MyName]"},
		},
		{
			name:    "doublestar suffix on symbol",
			pattern: "pkg.Type/**",
			matches: []string{
				"pkg.Type",                        // /** is optional
				"pkg.Type/doc",
				"pkg.Type/fields[Name]",
				"pkg.Type/fields[Name]/tag",
				"pkg.Type/fields[Name]/tag[json]",
			},
			rejects: []string{"pkg.TypeX/doc"},
		},
		{
			name:    "category with selector then leaf",
			pattern: "pkg.Func/*[*]/*",
			matches: []string{
				"pkg.Func/params[ctx]/type",
				"pkg.Func/returns[0]/type",
				"pkg.Func/typeparams[T]/constraint",
			},
			rejects: []string{"pkg.Func/body", "pkg.Func/params[ctx]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re, err := patternToRegex(tt.pattern)
			if err != nil {
				t.Fatalf("patternToRegex(%q) error: %v", tt.pattern, err)
			}

			for _, path := range tt.matches {
				if !re.MatchString(path) {
					t.Errorf("pattern %q should match %q but didn't\n  regex: %s", tt.pattern, path, re.String())
				}
			}

			for _, path := range tt.rejects {
				if re.MatchString(path) {
					t.Errorf("pattern %q should NOT match %q but did\n  regex: %s", tt.pattern, path, re.String())
				}
			}
		})
	}
}

// TestPatternMatchingComplex tests complex patterns combining multiple features.
func TestPatternMatchingComplex(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		matches []string
		rejects []string
	}{
		{
			name:    "any symbol with any subpath",
			pattern: "pkg.*/**",
			matches: []string{
				"pkg.Symbol",
				"pkg.Symbol/doc",
				"pkg.Type/fields[Name]",
				"pkg.Func/params[ctx]/type",
			},
			rejects: []string{"otherpkg.Symbol/doc"},
		},
		{
			name:    "deep package any symbol any subpath",
			pattern: "internal/**.*/**",
			matches: []string{
				"internal/golang.Symbol",
				"internal/golang.Symbol/doc",
				"internal/commands/spath.Path/fields[Package]",
			},
			rejects: []string{"pkg.Symbol/doc"},
		},
		{
			name:    "any params across codebase",
			pattern: "**.**/params[*]",
			matches: []string{
				"pkg.Func/params[ctx]",
				"internal/golang.WalkReferences/params[pkgs]",
			},
			rejects: []string{"pkg.Func/returns[0]"},
		},
		{
			name:    "any method body",
			pattern: "pkg.Type.*/body",
			matches: []string{
				"pkg.Type.GetID/body",
				"pkg.Type.String/body",
			},
			rejects: []string{"pkg.Type/body", "pkg.Func/body"},
		},
		{
			name:    "all json tags in subpaths",
			pattern: "**.*/fields[*]/tag[json]",
			matches: []string{
				"pkg.Type/fields[Name]/tag[json]",
				"internal/golang.Symbol/fields[Kind]/tag[json]",
			},
			rejects: []string{"pkg.Type/fields[Name]/tag[db]", "pkg.Type/fields[Name]/tag"},
		},
		{
			name:    "specific method in any type",
			pattern: "pkg.*.String",
			matches: []string{"pkg.Type.String", "pkg.Symbol.String"},
			rejects: []string{"pkg.String", "pkg.Type.String/body"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re, err := patternToRegex(tt.pattern)
			if err != nil {
				t.Fatalf("patternToRegex(%q) error: %v", tt.pattern, err)
			}

			for _, path := range tt.matches {
				if !re.MatchString(path) {
					t.Errorf("pattern %q should match %q but didn't\n  regex: %s", tt.pattern, path, re.String())
				}
			}

			for _, path := range tt.rejects {
				if re.MatchString(path) {
					t.Errorf("pattern %q should NOT match %q but did\n  regex: %s", tt.pattern, path, re.String())
				}
			}
		})
	}
}

// TestIsSpathPattern tests detection of glob wildcards in strings.
func TestIsSpathPattern(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Not patterns (no wildcards)
		{"pkg.Symbol", false},
		{"pkg.Type/fields[Name]", false},
		{"encoding/json.Marshal", false},
		{"pkg.Type/fields[0]/tag[json]", false},
		{"io", false},

		// Patterns (contain *)
		{"pkg.*", true},
		{"**.Symbol", true},
		{"pkg.Type/**", true},
		{"pkg.Type/fields[*]", true},
		{"**/pkg.Symbol", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsSpathPattern(tt.input)
			if got != tt.want {
				t.Errorf("IsSpathPattern(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestWildcardBoundaries tests that wildcards respect structural boundaries.
func TestWildcardBoundaries(t *testing.T) {
	t.Run("single star stops at dot", func(t *testing.T) {
		re, _ := patternToRegex("pkg.*")
		if !re.MatchString("pkg.Symbol") {
			t.Error("pkg.* should match pkg.Symbol")
		}
		if re.MatchString("pkg.Type.Method") {
			t.Error("pkg.* should NOT match pkg.Type.Method (star doesn't cross dots)")
		}
	})

	t.Run("single star stops at slash", func(t *testing.T) {
		re, _ := patternToRegex("internal/*")
		if !re.MatchString("internal/golang") {
			t.Error("internal/* should match internal/golang")
		}
		if re.MatchString("internal/a/b") {
			t.Error("internal/* should NOT match internal/a/b (star doesn't cross slashes)")
		}
	})

	t.Run("single star stops at bracket", func(t *testing.T) {
		re, _ := patternToRegex("pkg.Type/fields[*]")
		if !re.MatchString("pkg.Type/fields[Name]") {
			t.Error("pkg.Type/fields[*] should match pkg.Type/fields[Name]")
		}
		if re.MatchString("pkg.Type/fields[Name]/tag") {
			t.Error("pkg.Type/fields[*] should NOT match pkg.Type/fields[Name]/tag")
		}
	})

	t.Run("double star crosses all delimiters", func(t *testing.T) {
		re, _ := patternToRegex("pkg.**")
		if !re.MatchString("pkg.Type.Method") {
			t.Error("pkg.** should match pkg.Type.Method")
		}
		if !re.MatchString("pkg.Type/fields[Name]") {
			t.Error("pkg.** should match pkg.Type/fields[Name]")
		}
		if !re.MatchString("pkg.Type/fields[Name]/tag[json]") {
			t.Error("pkg.** should match pkg.Type/fields[Name]/tag[json]")
		}
	})

	t.Run("slash doublestar prefix matches package paths", func(t *testing.T) {
		re, _ := patternToRegex("**/golang.Symbol")
		if !re.MatchString("golang.Symbol") {
			t.Error("**/golang.Symbol should match golang.Symbol (no prefix)")
		}
		if !re.MatchString("internal/golang.Symbol") {
			t.Error("**/golang.Symbol should match internal/golang.Symbol")
		}
		if !re.MatchString("a/b/c/golang.Symbol") {
			t.Error("**/golang.Symbol should match a/b/c/golang.Symbol")
		}
	})

	t.Run("slash doublestar suffix matches subpaths", func(t *testing.T) {
		re, _ := patternToRegex("pkg.Symbol/**")
		if !re.MatchString("pkg.Symbol") {
			t.Error("pkg.Symbol/** should match pkg.Symbol (suffix optional)")
		}
		if !re.MatchString("pkg.Symbol/fields[Name]") {
			t.Error("pkg.Symbol/** should match pkg.Symbol/fields[Name]")
		}
		if !re.MatchString("pkg.Symbol/fields[Name]/tag[json]") {
			t.Error("pkg.Symbol/** should match pkg.Symbol/fields[Name]/tag[json]")
		}
	})
}

// TestStrictRejections tests that patterns correctly reject invalid matches.
func TestStrictRejections(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		rejects []string
	}{
		{
			name:    "star rejects paths crossing dot",
			pattern: "pkg.*",
			rejects: []string{
				"pkg.Type.Method",
				"pkg.Type.Method/body",
				"pkg.Symbol/fields[X]",
				"otherpkg.Symbol",
				"pkg",
			},
		},
		{
			name:    "star rejects paths crossing slash",
			pattern: "internal/*",
			rejects: []string{
				"internal/a/b",
				"internal/golang.Symbol",
				"external/golang",
				"internal",
			},
		},
		{
			name:    "star in selector rejects nested paths",
			pattern: "pkg.Type/fields[*]",
			rejects: []string{
				"pkg.Type/fields",
				"pkg.Type/fields[Name]/tag",
				"pkg.Type/methods[X]",
				"pkg.OtherType/fields[Name]",
			},
		},
		{
			name:    "exact package rejects similar names",
			pattern: "json.Marshal",
			rejects: []string{
				"encoding/json.Marshal",
				"myjson.Marshal",
				"json.MarshalIndent",
				"json.Marshal/body",
			},
		},
		{
			name:    "exact symbol rejects partial matches",
			pattern: "pkg.Read",
			rejects: []string{
				"pkg.Reader",
				"pkg.ReadAll",
				"pkg.DoRead",
				"pkg.Read.Something",
				"pkg.Read/body",
			},
		},
		{
			name:    "method pattern rejects non-methods",
			pattern: "pkg.Type.Method",
			rejects: []string{
				"pkg.Type",
				"pkg.Type.OtherMethod",
				"pkg.Type.Method/body",
				"pkg.OtherType.Method",
			},
		},
		{
			name:    "category rejects wrong categories",
			pattern: "pkg.Func/params",
			rejects: []string{
				"pkg.Func/returns",
				"pkg.Func/body",
				"pkg.Func/params[ctx]",
				"pkg.Func",
			},
		},
		{
			name:    "selector rejects wrong selectors",
			pattern: "pkg.Func/params[ctx]",
			rejects: []string{
				"pkg.Func/params[name]",
				"pkg.Func/params[0]",
				"pkg.Func/params",
				"pkg.Func/params[ctx]/type",
			},
		},
		{
			name:    "doublestar suffix rejects different base",
			pattern: "pkg.Type/**",
			rejects: []string{
				"pkg.TypeX/fields[X]",
				"otherpkg.Type/fields[X]",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re, err := patternToRegex(tt.pattern)
			if err != nil {
				t.Fatalf("patternToRegex(%q) error: %v", tt.pattern, err)
			}

			for _, path := range tt.rejects {
				if re.MatchString(path) {
					t.Errorf("pattern %q must NOT match %q but did\n  regex: %s", tt.pattern, path, re.String())
				}
			}
		})
	}
}

// TestCaseSensitivity verifies that pattern matching is case-sensitive.
func TestCaseSensitivity(t *testing.T) {
	tests := []struct {
		pattern string
		matches string
		rejects string
	}{
		{"pkg.Symbol", "pkg.Symbol", "pkg.symbol"},
		{"pkg.symbol", "pkg.symbol", "pkg.Symbol"},
		{"pkg.Type/fields[Name]", "pkg.Type/fields[Name]", "pkg.Type/fields[name]"},
		{"PKG.Symbol", "PKG.Symbol", "pkg.Symbol"},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			re, err := patternToRegex(tt.pattern)
			if err != nil {
				t.Fatalf("patternToRegex(%q) error: %v", tt.pattern, err)
			}

			if !re.MatchString(tt.matches) {
				t.Errorf("pattern %q should match %q (same case)", tt.pattern, tt.matches)
			}
			if re.MatchString(tt.rejects) {
				t.Errorf("pattern %q must NOT match %q (different case)", tt.pattern, tt.rejects)
			}
		})
	}
}

// TestPatternToRegexValidRegex ensures patterns compile to valid regex.
func TestPatternToRegexValidRegex(t *testing.T) {
	patterns := []string{
		"pkg.Symbol",
		"pkg.*",
		"pkg.**",
		"**/pkg.Symbol",
		"pkg.Symbol/**",
		"pkg.Type/fields[*]",
		"pkg.Type/fields[*]/tag[*]",
		"**/pkg.*/**",
		"github.com/user/repo.Type.Method/params[ctx]/type",
		"internal/**/golang.Symbol/fields[*]/tag[json]",
	}

	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) {
			re, err := patternToRegex(pattern)
			if err != nil {
				t.Errorf("patternToRegex(%q) should compile but got error: %v", pattern, err)
			}
			if re == nil {
				t.Errorf("patternToRegex(%q) returned nil regex", pattern)
			}
			_ = re.MatchString("test")
		})
	}
}

// TestInvalidSpathsNeverMatch verifies that patterns don't match invalid spath structures.
// These are strings that violate the spath grammar and should never be matched.
func TestInvalidSpathsNeverMatch(t *testing.T) {
	invalidSpaths := []string{
		"",                           // empty - not valid
		"Symbol",                     // bare symbol without package - not valid
		".Symbol",                    // missing package
		"pkg.",                       // missing symbol
		"pkg..Symbol",                // double dot
		"pkg.Symbol//body",           // double slash
		"/body",                      // leading slash without path
		"pkg.Symbol/",                // trailing slash
		"pkg.Symbol/fields[]",        // empty selector
		"pkg.Symbol/fields[",         // unclosed bracket
		"pkg.Symbol/[Name]",          // missing category
	}

	// Common patterns that users might write
	patterns := []string{
		"*.*",
		"pkg.*",
		"**.Symbol",
		"**/pkg.*",
	}

	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) {
			re, err := patternToRegex(pattern)
			if err != nil {
				t.Fatalf("patternToRegex(%q) error: %v", pattern, err)
			}

			for _, invalid := range invalidSpaths {
				if re.MatchString(invalid) {
					t.Errorf("pattern %q should NOT match invalid spath %q but did\n  regex: %s",
						pattern, invalid, re.String())
				}
			}
		})
	}
}

// TestSpathGrammarCoverage documents which grammar elements are tested.
func TestSpathGrammarCoverage(t *testing.T) {
	t.Log(`
Grammar coverage by test functions:

path = package_path , "." , symbol , [ subpath ]
  - TestPatternMatchingSymbols
  - TestPatternMatchingSubpaths

package_path = package_component , { "/" , package_component }
  - TestPatternMatchingPackages (io, encoding/json, github.com/user/repo)

package_component = identifier | identifier "." identifier
  - TestPatternMatchingPackages (github.com)

symbol = identifier , [ "." , identifier ]
  - TestPatternMatchingSymbols (Symbol, Type.Method)

subpath = "/" , segment , { "/" , segment }
  - TestPatternMatchingSubpaths (/body, /params[ctx]/type)

segment = category , [ selector ]
  - TestPatternMatchingSubpaths (body, params[ctx])

category: fields, methods, embeds, params, returns, receiver, typeparams,
          body, doc, tag, type, name, constraint, value
  - TestPatternMatchingSubpaths covers most categories

selector = "[" , ( identifier | integer ) , "]"
  - TestPatternMatchingSubpaths ([ctx], [0], [Name])
`)
}
