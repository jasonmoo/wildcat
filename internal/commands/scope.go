package commands

import (
	"context"
	"strings"

	"github.com/jasonmoo/wildcat/internal/golang"
)

// ScopeFilter filters packages by include/exclude patterns.
// Patterns are resolved to full package paths before matching.
type ScopeFilter struct {
	wc       *Wildcat
	all      bool
	project  bool
	target   *golang.PackageIdentifier
	includes map[string]bool
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
func (wc *Wildcat) ParseScope(ctx context.Context, scope, targetPkg string) (*ScopeFilter, error) {

	filter := &ScopeFilter{
		wc:       wc,
		includes: make(map[string]bool),
		excludes: make(map[string]bool),
	}

	if targetPkg == "" {
		targetPkg = "."
	}

	// explicitly allow the target path
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
			// redundant, ignore
		case strings.HasPrefix(part, "-"):
			// Exclude pattern - resolve to full path
			pattern := strings.TrimPrefix(part, "-")
			pi, err := wc.Project.ResolvePackageName(ctx, pattern)
			if err != nil {
				return nil, err
			}
			filter.excludes[pi.PkgPath] = true
			delete(filter.includes, pi.PkgPath)
		default:
			pi, err := wc.Project.ResolvePackageName(ctx, part)
			if err != nil {
				return nil, err
			}
			filter.includes[pi.PkgPath] = true
			delete(filter.excludes, pi.PkgPath)
		}
	}

	return filter, nil
}

// InScope returns true if the package path matches the filter.
func (f *ScopeFilter) InScope(pkgPath string) bool {
	if f.excludes[pkgPath] {
		return false
	}
	if f.all {
		return true
	}
	if f.project {
		return strings.HasPrefix(pkgPath, f.wc.Project.Module.Path)
	}
	return f.includes[pkgPath]
}
