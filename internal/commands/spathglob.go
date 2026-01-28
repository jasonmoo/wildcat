package commands

import (
	"regexp"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands/spath"
	"github.com/jasonmoo/wildcat/internal/golang"
)

// SpathEntry represents an enumerated semantic path.
type SpathEntry struct {
	// Path is the canonical spath format: "pkg.Symbol/fields[Name]"
	Path string

	// Kind describes what this path points to (func, type, field, param, etc.)
	Kind string

	// Type is the type annotation if available
	Type string

	// Package is the full package path for grouping
	Package string

	// PackageShort is the module-relative package path for display
	PackageShort string
}

// SpathGlobResult holds the results of a pattern match operation.
type SpathGlobResult struct {
	Pattern string
	Matches []SpathEntry
	Total   int // Total matches before any limit applied
}

// EnumerateAllSpaths returns all semantic paths in the project.
func (wc *Wildcat) EnumerateAllSpaths() []SpathEntry {
	var entries []SpathEntry

	for _, pkg := range wc.Project.Packages {
		pkgShort := pkg.Identifier.PkgShortPath

		// Add the package itself as a path
		entries = append(entries, SpathEntry{
			Path:         pkgShort,
			Kind:         "package",
			Package:      pkg.Identifier.PkgPath,
			PackageShort: pkgShort,
		})

		for _, sym := range pkg.Symbols {
			// Build base path for this symbol
			basePath := &spath.Path{
				Package: pkgShort,
				Symbol:  sym.Name,
			}

			// Add the symbol itself
			entries = append(entries, SpathEntry{
				Path:         basePath.String(),
				Kind:         string(sym.Kind),
				Type:         spathSymbolTypeString(sym),
				Package:      pkg.Identifier.PkgPath,
				PackageShort: pkgShort,
			})

			// Resolve and enumerate children
			res, err := spath.NewResolution(basePath, pkg, sym)
			if err != nil {
				continue
			}

			// Recursively enumerate all children
			children := enumerateAllChildren(res, sym, basePath, pkg)
			for _, child := range children {
				entries = append(entries, SpathEntry{
					Path:         child.Path,
					Kind:         child.Kind,
					Type:         child.Type,
					Package:      pkg.Identifier.PkgPath,
					PackageShort: pkgShort,
				})
			}
		}
	}

	return entries
}

// enumerateAllChildren recursively enumerates all descendants of a resolution.
func enumerateAllChildren(res *spath.Resolution, sym *golang.Symbol, basePath *spath.Path, pkg *golang.Package) []spath.ChildPath {
	var all []spath.ChildPath

	children := spath.EnumerateChildrenWithBase(res, sym, basePath)
	for _, child := range children {
		all = append(all, child)

		// For methods, resolve and enumerate their children
		if child.Category == "methods" && child.Selector != "" {
			methodPath := &spath.Path{
				Package: basePath.Package,
				Symbol:  basePath.Symbol,
				Method:  child.Selector,
			}
			// Find the method symbol
			var methodSym *golang.Symbol
			for _, m := range sym.Methods {
				if m.Name == child.Selector {
					methodSym = m
					break
				}
			}
			if methodSym != nil {
				methodRes, err := spath.NewResolution(methodPath, pkg, methodSym)
				if err == nil {
					methodChildren := enumerateAllChildren(methodRes, methodSym, methodPath, pkg)
					all = append(all, methodChildren...)
				}
			}
		}

		// For fields/params/returns that can have children (tag, doc)
		if child.Category == "fields" || child.Category == "params" || child.Category == "returns" || child.Category == "embeds" {
			childPath, err := spath.Parse(child.Path)
			if err != nil {
				continue
			}
			childRes, err := spath.NewResolution(childPath, pkg, sym)
			if err != nil {
				continue
			}
			fieldChildren := spath.EnumerateChildrenWithBase(childRes, nil, childPath)
			all = append(all, fieldChildren...)
		}
	}

	return all
}

// MatchSpathGlob matches a wildcard pattern against all spaths and returns matches.
// Patterns use spath syntax with wildcards:
//   - * matches any chars within a segment (stops at . / [ ])
//   - ** matches any chars across segments
func (wc *Wildcat) MatchSpathGlob(pattern string, limit int) (*SpathGlobResult, error) {
	// Strip leading ./ since paths are module-relative
	pattern = strings.TrimPrefix(pattern, "./")

	re, err := patternToRegex(pattern)
	if err != nil {
		return nil, err
	}

	entries := wc.EnumerateAllSpaths()

	var matches []SpathEntry
	for _, entry := range entries {
		if re.MatchString(entry.Path) {
			matches = append(matches, entry)
		}
	}

	total := len(matches)
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}

	return &SpathGlobResult{
		Pattern: pattern,
		Matches: matches,
		Total:   total,
	}, nil
}

// patternToRegex converts a wildcard pattern to a compiled regex.
//   - **/ becomes (.*/)? (match any prefix ending with /, or nothing)
//   - /** becomes (/.*)? (match any suffix starting with /, or nothing)
//   - ** becomes .* (match anything)
//   - * becomes [^./\[\]]* (match within segment)
//   - everything else is escaped as literal
func patternToRegex(pattern string) (*regexp.Regexp, error) {
	// Use placeholders to protect wildcards during QuoteMeta
	const (
		doubleStarSlash = "\x00"
		slashDoubleStar = "\x01"
		doubleStar      = "\x02"
		singleStar      = "\x03"
	)

	// Order matters: replace **/ and /** before standalone **
	result := strings.ReplaceAll(pattern, "**/", doubleStarSlash)
	result = strings.ReplaceAll(result, "/**", slashDoubleStar)
	result = strings.ReplaceAll(result, "**", doubleStar)
	result = strings.ReplaceAll(result, "*", singleStar)

	// Escape all regex special chars
	result = regexp.QuoteMeta(result)

	// Replace placeholders with regex patterns
	result = strings.ReplaceAll(result, doubleStarSlash, `(.*/)?`)
	result = strings.ReplaceAll(result, slashDoubleStar, `(/.*)?`)
	result = strings.ReplaceAll(result, doubleStar, `.*`)
	result = strings.ReplaceAll(result, singleStar, `[^./\[\]]*`)

	return regexp.Compile("^" + result + "$")
}

// IsSpathPattern returns true if the string contains glob wildcards.
// Only checks for * since [ is valid spath syntax for selectors.
func IsSpathPattern(s string) bool {
	return strings.Contains(s, "*")
}

// spathSymbolTypeString returns the type annotation for a symbol.
func spathSymbolTypeString(sym *golang.Symbol) string {
	switch sym.Kind {
	case golang.SymbolKindFunc, golang.SymbolKindMethod:
		return sym.Signature()
	case golang.SymbolKindType, golang.SymbolKindInterface:
		return sym.TypeKind()
	default:
		if sym.Object != nil {
			return sym.Object.Type().String()
		}
		return string(sym.Kind)
	}
}
