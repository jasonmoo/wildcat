package commands

import (
	"context"
	"strings"
)

// ScopeFilter filters packages by include/exclude patterns.
// Patterns are resolved to full package paths before matching.
type ScopeFilter struct {
	wc       *Wildcat
	all      bool
	project  bool
	includes map[string]bool // nil means project scope (all project packages)
	excludes map[string]bool
}

// ParseScope parses a scope string and returns a ScopeFilter.
// Scope formats:
//   - "project" - all project packages (default)
//   - "all" - all packages including dependencies
//   - "package" - requires targetPkgPath, limits to that package
//   - "pkg1,pkg2" - specific packages (resolved to full paths)
//   - "-pkg" - exclude packages matching pattern
//
// Each part is resolved using ResolvePackageName for consistent matching.
func (wc *Wildcat) ParseScope(ctx context.Context, scope string) ScopeFilter {

	filter := ScopeFilter{
		wc:       wc,
		includes: make(map[string]bool),
		excludes: make(map[string]bool),
	}

	// Parse comma-separated includes/excludes
	for _, part := range strings.Split(scope, ",") {
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
		case strings.HasPrefix(part, "-"):
			// Exclude pattern - resolve to full path
			pattern := strings.TrimPrefix(part, "-")
			if pi, err := wc.Project.ResolvePackageName(ctx, pattern); err == nil {
				filter.excludes[pi.PkgPath] = true
			}
		default:
			if pi, err := wc.Project.ResolvePackageName(ctx, part); err == nil {
				filter.includes[pi.PkgPath] = true
			}
		}
	}

	return filter
}

// InScope returns true if the package path matches the filter.
func (f *ScopeFilter) InScope(pkgPath string) bool {

	// Check excludes first
	if f.excludes[pkgPath] {
		return false
	}
	if f.all {
		return true
	}
	if f.project {
		if strings.HasPrefix(pkgPath, f.wc.Project.Module.Path) {
			return true
		}
		return false
	}
	return f.includes[pkgPath]
}
