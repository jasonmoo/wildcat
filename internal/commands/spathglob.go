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

	// Expand short package names to full paths
	pattern = wc.expandPatternPackage(pattern)

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

// expandPatternPackage expands short package names in patterns to full module-relative paths.
// For example, "golang.Symbol*" becomes "internal/golang.Symbol*" if "golang" resolves to "internal/golang".
func (wc *Wildcat) expandPatternPackage(pattern string) string {
	// If pattern starts with ** or contains /, assume it's already a path pattern
	if strings.HasPrefix(pattern, "**") || strings.Contains(pattern, "/") {
		return pattern
	}

	// Extract the package part (before first .)
	dotIdx := strings.Index(pattern, ".")
	if dotIdx == -1 {
		// No dot - could be a package name pattern like "golang*"
		// Try to find matching packages
		return pattern
	}

	pkgPart := pattern[:dotIdx]
	rest := pattern[dotIdx:]

	// Skip if package part contains wildcards - can't resolve those
	if strings.Contains(pkgPart, "*") {
		return pattern
	}

	// Try to find a package matching this short name
	for _, pkg := range wc.Project.Packages {
		if pkg.Identifier.Name == pkgPart {
			// Found it - expand to full path
			return pkg.Identifier.PkgShortPath + rest
		}
	}

	// No match found, return original
	return pattern
}

// patternToRegex converts a wildcard pattern to a compiled regex.
//   - **/ becomes (.*/)? (match any prefix ending with /, or nothing)
//   - /** becomes (/.*)? (match any suffix starting with /, or nothing)
//   - **. becomes (.+[^.]|[^.])\. (match non-empty string not ending with dot, then dot)
//   - ** becomes .* (match anything)
//   - *.* becomes [^./\[\]]+\.[^./\[\]]+ (both stars are full identifiers, both need 1+)
//   - [*] becomes \[[^./\[\]]+\] (complete selector, star needs 1+)
//   - .* at end becomes \.[^./\[\]]+ (star is full symbol, needs 1+)
//   - /* at end becomes /[^./\[\]]+ (star is full package component, needs 1+)
//   - * elsewhere becomes [^./\[\]]* (can match 0+ chars - partial identifier)
//   - everything else is escaped as literal
func patternToRegex(pattern string) (*regexp.Regexp, error) {
	// Use placeholders to protect wildcards during QuoteMeta
	const (
		doubleStarSlash  = "\x00"
		slashDoubleStar  = "\x01"
		doubleStarDot    = "\x02"
		doubleStar       = "\x03"
		starDotStar      = "\x04" // *.* - both stars are full identifiers
		bracketStarClose = "\x05" // [*] - complete selector, needs 1+
		dotStarEnd       = "\x06" // .* at end - full symbol, needs 1+
		slashStarEnd     = "\x07" // /* at end - full component, needs 1+
		singleStar       = "\x08" // * elsewhere - partial, can be 0+
	)

	// Order matters: replace specific patterns before general ones
	result := strings.ReplaceAll(pattern, "**/", doubleStarSlash)
	result = strings.ReplaceAll(result, "/**", slashDoubleStar)
	result = strings.ReplaceAll(result, "**.", doubleStarDot)
	result = strings.ReplaceAll(result, "**", doubleStar)
	// Handle *.* (both stars are full identifiers)
	result = strings.ReplaceAll(result, "*.*", starDotStar)
	// Handle [*] (complete selector)
	result = strings.ReplaceAll(result, "[*]", bracketStarClose)
	// Handle .* and /* at end of pattern (full identifier)
	if strings.HasSuffix(result, ".*") {
		result = result[:len(result)-2] + dotStarEnd
	}
	if strings.HasSuffix(result, "/*") {
		result = result[:len(result)-2] + slashStarEnd
	}
	// Remaining stars are partial matches (can match 0+)
	result = strings.ReplaceAll(result, "*", singleStar)

	// Escape all regex special chars
	result = regexp.QuoteMeta(result)

	// Replace placeholders with regex patterns
	result = strings.ReplaceAll(result, doubleStarSlash, `(.*/)?`)
	result = strings.ReplaceAll(result, slashDoubleStar, `(/.*)?`)
	result = strings.ReplaceAll(result, doubleStarDot, `(.+[^.]|[^.])\.`)
	result = strings.ReplaceAll(result, doubleStar, `.*`)
	result = strings.ReplaceAll(result, starDotStar, `[^./\[\]]+\.[^./\[\]]+`)
	result = strings.ReplaceAll(result, bracketStarClose, `\[[^./\[\]]+\]`)
	result = strings.ReplaceAll(result, dotStarEnd, `\.[^./\[\]]+`)
	result = strings.ReplaceAll(result, slashStarEnd, `/[^./\[\]]+`)
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
