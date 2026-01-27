package spath

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Parse parses a path string into a Path struct.
//
// The input can use any of three package path forms:
//   - Package name: "golang.Symbol"
//   - Relative path: "internal/golang.Symbol"
//   - Full import path: "github.com/user/repo/internal/golang.Symbol"
//
// The package path is stored as-is; resolution to canonical form
// happens in Resolve().
//
// Examples:
//
//	Parse("encoding/json.Marshal")
//	Parse("golang.Symbol/fields[Name]")
//	Parse("io.Reader.Read/params[p]")
func Parse(input string) (*Path, error) {
	if input == "" {
		return nil, fmt.Errorf("empty path")
	}

	p := &parser{input: input}
	return p.parse()
}

// parser holds state during parsing.
type parser struct {
	input string
	pos   int
}

func (p *parser) parse() (*Path, error) {
	// Find the boundary between package path and symbol.
	// The package path can contain dots (github.com) and slashes.
	// The symbol starts after the last slash, at the first dot that
	// precedes an identifier (not another dot-separated domain component).
	//
	// Strategy: find the last slash, then parse from there.
	// If no slash, the entire thing before first dot is package name.

	pkgEnd, symbolStart, err := p.findPackageSymbolBoundary()
	if err != nil {
		return nil, err
	}

	path := &Path{
		Package: p.input[:pkgEnd],
	}

	// Parse symbol (and optional method) from symbolStart to first '/'
	p.pos = symbolStart
	if err := p.parseSymbol(path); err != nil {
		return nil, err
	}

	// Parse subpath segments if present
	if p.pos < len(p.input) && p.input[p.pos] == '/' {
		if err := p.parseSubpath(path); err != nil {
			return nil, err
		}
	}

	// Should have consumed entire input
	if p.pos < len(p.input) {
		return nil, fmt.Errorf("unexpected character at position %d: %q", p.pos, p.input[p.pos])
	}

	return path, nil
}

// findPackageSymbolBoundary finds where the package path ends and symbol begins.
// Returns (pkgEnd, symbolStart, error) where input[pkgEnd] is the dot before symbol.
func (p *parser) findPackageSymbolBoundary() (int, int, error) {
	// Strategy: Find the last slash that's part of the package path.
	// A slash in a subpath (after the symbol) comes after a symbol identifier.
	//
	// We scan for the pattern: identifier.Identifier where the second Identifier
	// is followed by end, '.', '/', or '['. The first dot in such a pattern
	// is the package/symbol boundary.

	// First, find all slashes and determine which are package separators vs subpath
	// A subpath slash is preceded by an identifier (the symbol or method name)
	// and the identifier before that is preceded by a dot (pkg.Symbol/)

	// Simpler approach: work backwards from potential symbol boundaries
	// The symbol boundary is the FIRST dot (after the last package-path slash)
	// where the identifier following it is a valid Go identifier AND
	// what follows that identifier is end, '.', '/', or '['.

	// Find where package path definitely ends: either at a '/' followed by
	// something that can't be a path component, or at the first '.' after
	// the last definite package slash.

	input := p.input

	// Find the last slash that's definitely part of the package path.
	// A slash is part of package path if what follows it could be a package component
	// (identifier, possibly with dots like github.com).
	// A slash is a subpath separator if it's preceded by an identifier that's
	// followed by '[' or preceded by a '.identifier' sequence.

	// Let's try a different approach: scan left to right, tracking state
	lastPkgSlash := -1
	i := 0
	for i < len(input) {
		if input[i] == '/' {
			// Is this a package path slash or a subpath slash?
			// Look ahead: if followed by a known category name and '[' or '/', it's subpath
			ahead := input[i+1:]
			isSubpath := false
			for cat := range ValidCategories {
				if strings.HasPrefix(ahead, cat) {
					rest := ahead[len(cat):]
					if len(rest) == 0 || rest[0] == '[' || rest[0] == '/' {
						isSubpath = true
						break
					}
				}
			}
			if isSubpath {
				// This and all following slashes are subpath separators
				break
			}
			lastPkgSlash = i
		}
		i++
	}

	// Now find the symbol boundary: first dot after lastPkgSlash
	// that starts an identifier
	searchStart := 0
	if lastPkgSlash >= 0 {
		searchStart = lastPkgSlash + 1
	}

	for i := searchStart; i < len(input); i++ {
		if input[i] == '.' {
			// Check if what follows is an identifier
			if i+1 < len(input) && isIdentStart(rune(input[i+1])) {
				// Find end of identifier
				j := i + 1
				for j < len(input) && isIdentChar(rune(input[j])) {
					j++
				}
				// What comes after? end, '.', '/', '[' all indicate this is the symbol
				if j >= len(input) || input[j] == '.' || input[j] == '/' || input[j] == '[' {
					return i, i + 1, nil
				}
				// Otherwise it's probably a domain component (github.com), continue
			}
		}
	}

	return 0, 0, fmt.Errorf("cannot find symbol in path: %q", p.input)
}

// parseSymbol parses the symbol and optional method name.
func (p *parser) parseSymbol(path *Path) error {
	// Read the symbol name
	start := p.pos
	for p.pos < len(p.input) && isIdentChar(rune(p.input[p.pos])) {
		p.pos++
	}
	if p.pos == start {
		return fmt.Errorf("expected symbol name at position %d", p.pos)
	}
	path.Symbol = p.input[start:p.pos]

	// Check for method: .MethodName
	if p.pos < len(p.input) && p.input[p.pos] == '.' {
		p.pos++ // skip '.'
		start = p.pos
		for p.pos < len(p.input) && isIdentChar(rune(p.input[p.pos])) {
			p.pos++
		}
		if p.pos == start {
			return fmt.Errorf("expected method name after '.' at position %d", p.pos)
		}
		path.Method = p.input[start:p.pos]
	}

	return nil
}

// parseSubpath parses the /category[selector] segments.
func (p *parser) parseSubpath(path *Path) error {
	for p.pos < len(p.input) && p.input[p.pos] == '/' {
		p.pos++ // skip '/'

		// Read category
		start := p.pos
		for p.pos < len(p.input) && isIdentChar(rune(p.input[p.pos])) {
			p.pos++
		}
		if p.pos == start {
			return fmt.Errorf("expected category name after '/' at position %d", p.pos)
		}
		category := p.input[start:p.pos]

		if !ValidCategories[category] {
			return fmt.Errorf("invalid category %q at position %d", category, start)
		}

		seg := Segment{Category: category}

		// Check for selector: [name] or [0]
		if p.pos < len(p.input) && p.input[p.pos] == '[' {
			p.pos++ // skip '['

			start = p.pos
			for p.pos < len(p.input) && p.input[p.pos] != ']' {
				p.pos++
			}
			if p.pos >= len(p.input) {
				return fmt.Errorf("unclosed '[' at position %d", start-1)
			}

			seg.Selector = p.input[start:p.pos]
			if seg.Selector == "" {
				return fmt.Errorf("empty selector at position %d", start)
			}

			// Check if it's a numeric index
			if _, err := strconv.Atoi(seg.Selector); err == nil {
				seg.IsIndex = true
			}

			p.pos++ // skip ']'
		}

		path.Segments = append(path.Segments, seg)
	}

	return nil
}

// isIdentStart returns true if r can start a Go identifier.
func isIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

// isIdentChar returns true if r can be part of a Go identifier.
func isIdentChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
