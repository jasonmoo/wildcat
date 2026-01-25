package commands

import (
	"context"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jasonmoo/wildcat/internal/golang"
)

// ScopeFilter filters packages by include/exclude patterns.
//
// All patterns are resolved at parse time against the project's package list.
// This ensures:
//   - Clear visibility into what packages were examined
//   - No silent pattern matching that could hide issues
//   - Fast InScope() checks via map lookup
//
// # Pattern Syntax
//
// Patterns can be specified in three forms:
//
// 1. Exact package path - resolves and matches a single package
//
//	internal/lsp        → matches only "github.com/foo/internal/lsp"
//	./internal/lsp      → same, resolved relative to module root
//
// 2. Subtree pattern with /... suffix (Go-style) - matches a package and all subpackages
//
//	internal/...        → matches "internal", "internal/lsp", "internal/commands/search", etc.
//	./cmd/...           → matches "cmd", "cmd/foo", "cmd/foo/bar", etc.
//
// 3. Glob pattern with wildcards - uses doublestar matching (gitignore-style)
//
//   - → matches a single path segment (not crossing /)
//     **                  → matches zero or more path segments (crosses /)
//     ?                   → matches a single character
//     [abc]               → matches one character in the set
//     [!abc]              → matches one character not in the set
//
// Glob examples:
//
//	internal/*          → matches "internal/lsp", "internal/config" (direct children only)
//	internal/**         → matches "internal/lsp", "internal/commands/search" (all descendants)
//	**/test             → matches "test", "foo/test", "foo/bar/test" (anywhere)
//	**/internal/**/util → matches "a/internal/util", "a/internal/b/c/util"
//	*_test              → matches "foo_test", "bar_test" (suffix match)
//
// # Pattern Resolution
//
// Patterns are resolved at parse time by matching against all packages in the project.
// The resolved package list is available via ResolvedIncludes() and ResolvedExcludes()
// for inclusion in command output, ensuring full transparency about what was examined.
//
// # Include vs Exclude
//
// Patterns prefixed with - are exclusions. Exclusions take precedence over includes.
//
//	--scope "internal/...,-internal/testdata/..."
//	         ↑ include internal subtree, exclude testdata subtree
//
// # Special Keywords
//
//	all      → include all packages (project + dependencies)
//	project  → include only project packages (default)
//	package  → include only the target package
type ScopeFilter struct {
	wc      *Wildcat
	all     bool
	project bool
	target  *golang.PackageIdentifier

	// Resolved package paths (patterns expanded at parse time)
	includes map[string]bool
	excludes map[string]bool

	// Original patterns that were resolved (for reporting)
	includePatterns []string
	excludePatterns []string

	// Packages matched by each pattern (for detailed reporting)
	resolvedIncludes []string // sorted list of included packages
	resolvedExcludes []string // sorted list of excluded packages
}

// isPattern returns true if the string contains glob wildcards or /... suffix.
func isPattern(s string) bool {
	return strings.HasSuffix(s, "/...") || strings.ContainsAny(s, "*?[")
}

// matchPattern checks if pkgPath matches the given pattern.
//
// Pattern types:
//   - "foo/..." matches "foo" and any path starting with "foo/"
//   - glob patterns use doublestar.Match
func matchPattern(pattern, pkgPath string) (bool, error) {
	// Handle Go-style /... suffix (match package and all subpackages)
	if strings.HasSuffix(pattern, "/...") {
		base := strings.TrimSuffix(pattern, "/...")
		return pkgPath == base || strings.HasPrefix(pkgPath, base+"/"), nil
	}

	// Use doublestar for glob patterns
	return doublestar.Match(pattern, pkgPath)
}

// resolvePattern matches a pattern against all project packages and returns
// the list of matching package paths.
func (wc *Wildcat) resolvePattern(pattern string) ([]string, error) {
	var matches []string

	// Get the module path prefix to convert full paths to relative for matching
	modulePath := wc.Project.Module.Path

	for _, pkg := range wc.Project.Packages {
		pkgPath := pkg.Identifier.PkgPath

		// For patterns, match against the relative path within the module
		relPath := pkgPath
		if strings.HasPrefix(pkgPath, modulePath+"/") {
			relPath = strings.TrimPrefix(pkgPath, modulePath+"/")
		} else if pkgPath == modulePath {
			relPath = "."
		}

		// Try matching against both relative and full path
		relMatch, err := matchPattern(pattern, relPath)
		if err != nil {
			return nil, err
		}
		fullMatch, err := matchPattern(pattern, pkgPath)
		if err != nil {
			return nil, err
		}
		if relMatch || fullMatch {
			matches = append(matches, pkgPath)
		}
	}

	sort.Strings(matches)
	return matches, nil
}

// ParseScope parses a scope string and returns a ScopeFilter.
//
// All patterns are resolved at parse time against the project's package list.
// This ensures clear visibility into what packages will be examined.
//
// The targetPkg is always included in scope (unless explicitly excluded).
func (wc *Wildcat) ParseScope(ctx context.Context, scope, targetPkg string) (*ScopeFilter, error) {
	filter := &ScopeFilter{
		wc:       wc,
		includes: make(map[string]bool),
		excludes: make(map[string]bool),
	}

	if targetPkg == "" {
		targetPkg = "."
	}

	// Explicitly allow the target path
	pi, err := wc.Project.ResolvePackageName(ctx, targetPkg)
	if err != nil {
		return nil, err
	}
	filter.target = pi
	filter.includes[pi.PkgPath] = true

	// Parse comma-separated includes/excludes
	for part := range strings.SplitSeq(scope, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
			continue
		case part == "all":
			filter.all = true
			filter.project = false
		case part == "project":
			filter.all = false
			filter.project = true
		case part == "package":
			// Limit to target package only - clear other includes
			filter.includes = map[string]bool{pi.PkgPath: true}
			filter.includePatterns = nil
		case strings.HasPrefix(part, "-"):
			// Exclude pattern
			pattern := strings.TrimPrefix(part, "-")
			if isPattern(pattern) {
				// Resolve pattern against all packages
				filter.excludePatterns = append(filter.excludePatterns, pattern)
				resolved, err := wc.resolvePattern(pattern)
				if err != nil {
					return nil, err
				}
				for _, pkgPath := range resolved {
					filter.excludes[pkgPath] = true
					delete(filter.includes, pkgPath)
				}
			} else {
				// Exact match - resolve to full path
				resolved, err := wc.Project.ResolvePackageName(ctx, pattern)
				if err != nil {
					return nil, err
				}
				filter.excludes[resolved.PkgPath] = true
				delete(filter.includes, resolved.PkgPath)
			}
		default:
			// Include pattern
			if isPattern(part) {
				// Resolve pattern against all packages
				filter.includePatterns = append(filter.includePatterns, part)
				resolved, err := wc.resolvePattern(part)
				if err != nil {
					return nil, err
				}
				for _, pkgPath := range resolved {
					filter.includes[pkgPath] = true
					delete(filter.excludes, pkgPath)
				}
			} else {
				// Exact match - resolve to full path
				resolved, err := wc.Project.ResolvePackageName(ctx, part)
				if err != nil {
					return nil, err
				}
				filter.includes[resolved.PkgPath] = true
				delete(filter.excludes, resolved.PkgPath)
			}
		}
	}

	// Build sorted lists for reporting
	for pkg := range filter.includes {
		filter.resolvedIncludes = append(filter.resolvedIncludes, pkg)
	}
	sort.Strings(filter.resolvedIncludes)

	for pkg := range filter.excludes {
		filter.resolvedExcludes = append(filter.resolvedExcludes, pkg)
	}
	sort.Strings(filter.resolvedExcludes)

	return filter, nil
}

// InScope returns true if the package path matches the filter.
//
// Since patterns are resolved at parse time, this is a simple map lookup.
func (f *ScopeFilter) InScope(pkgPath string) bool {
	// Check excludes first (exclusions take precedence)
	if f.excludes[pkgPath] {
		return false
	}

	// "all" mode includes everything not excluded
	if f.all {
		return true
	}

	// "project" mode includes all project packages not excluded
	if f.project {
		return strings.HasPrefix(pkgPath, f.wc.Project.Module.Path)
	}

	// Check explicit includes
	return f.includes[pkgPath]
}

// ResolvedIncludes returns the sorted list of package paths that were
// resolved as includes. Use this for reporting what was examined.
func (f *ScopeFilter) ResolvedIncludes() []string {
	return f.resolvedIncludes
}

// ResolvedExcludes returns the sorted list of package paths that were
// resolved as excludes. Use this for reporting what was excluded.
func (f *ScopeFilter) ResolvedExcludes() []string {
	return f.resolvedExcludes
}

// IncludePatterns returns the original include patterns that were specified.
func (f *ScopeFilter) IncludePatterns() []string {
	return f.includePatterns
}

// ExcludePatterns returns the original exclude patterns that were specified.
func (f *ScopeFilter) ExcludePatterns() []string {
	return f.excludePatterns
}
