package project_cmd

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
)

// ModuleInfo describes the Go module.
type ModuleInfo struct {
	Path    string `json:"path"`
	Dir     string `json:"dir"`
	Version string `json:"go_version"`
}

// ProjectSummary provides aggregate counts.
type ProjectSummary struct {
	Packages       int `json:"packages"`
	EntryPoints    int `json:"entry_points"`
	StdlibDeps     int `json:"stdlib_deps"`
	ThirdPartyDeps int `json:"third_party_deps"`
}

// EntryPoint represents a main package.
type EntryPoint struct {
	Symbol   string `json:"symbol"`
	Location string `json:"location"`
}

// CorePackage represents a package with its metrics.
type CorePackage struct {
	Path       string `json:"path"`
	Methods    int    `json:"methods"`    // functions + methods (callable symbols)
	Dependents int    `json:"dependents"` // packages that import this
	Files      int    `json:"files"`
	Symbols    int    `json:"symbols"`
	Lines      int    `json:"lines"`
}

// CrossPackageInterface represents an interface implemented across packages.
type CrossPackageInterface struct {
	Interface     string   `json:"interface"`
	Package       string   `json:"package"`
	ImplementedBy []string `json:"implemented_by"`
}

// StdlibInterfaceAlias represents a project interface that is identical to a stdlib/builtin interface.
type StdlibInterfaceAlias struct {
	Interface string `json:"interface"` // project interface (e.g., "model.NotFoundError")
	Satisfies string `json:"satisfies"` // stdlib/builtin interface (e.g., "builtin.error")
}

// PackageNode represents a package in the dependency graph.
type PackageNode struct {
	Path    string   `json:"path"`
	Imports []string `json:"imports"`
}

// DepInfo represents a dependency with its usage info.
type DepInfo struct {
	Path  string   `json:"path"`
	Users []string `json:"users,omitempty"` // which packages use this (empty if >2)
}

// ProjectCommandResponse is the response for the project command.
type ProjectCommandResponse struct {
	Module         ModuleInfo              `json:"module"`
	Summary        ProjectSummary          `json:"summary"`
	EntryPoints    []EntryPoint            `json:"entry_points"`
	CorePackages   []CorePackage           `json:"core_packages"`
	Interfaces     []CrossPackageInterface `json:"cross_package_interfaces"`
	StdlibAliases  []StdlibInterfaceAlias  `json:"stdlib_interface_aliases,omitempty"`
	StdlibDeps     []DepInfo               `json:"stdlib_deps"`
	ThirdPartyDeps []DepInfo               `json:"third_party_deps"`
	PackageGraph   []PackageNode           `json:"package_graph"`
	Diagnostics    []commands.Diagnostic   `json:"diagnostics,omitempty"`
}

var _ commands.Result = (*ProjectCommandResponse)(nil)

func (r *ProjectCommandResponse) SetDiagnostics(ds []commands.Diagnostic) {
	r.Diagnostics = ds
}

func (r *ProjectCommandResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Module         ModuleInfo              `json:"module"`
		Summary        ProjectSummary          `json:"summary"`
		EntryPoints    []EntryPoint            `json:"entry_points"`
		CorePackages   []CorePackage           `json:"core_packages"`
		Interfaces     []CrossPackageInterface `json:"cross_package_interfaces"`
		StdlibAliases  []StdlibInterfaceAlias  `json:"stdlib_interface_aliases,omitempty"`
		StdlibDeps     []DepInfo               `json:"stdlib_deps"`
		ThirdPartyDeps []DepInfo               `json:"third_party_deps"`
		PackageGraph   []PackageNode           `json:"package_graph"`
		Diagnostics    []commands.Diagnostic   `json:"diagnostics,omitempty"`
	}{
		Module:         r.Module,
		Summary:        r.Summary,
		EntryPoints:    r.EntryPoints,
		CorePackages:   r.CorePackages,
		Interfaces:     r.Interfaces,
		StdlibAliases:  r.StdlibAliases,
		StdlibDeps:     r.StdlibDeps,
		ThirdPartyDeps: r.ThirdPartyDeps,
		PackageGraph:   r.PackageGraph,
		Diagnostics:    r.Diagnostics,
	})
}

func (r *ProjectCommandResponse) MarshalMarkdown() ([]byte, error) {
	var sb strings.Builder

	commands.FormatDiagnosticsMarkdown(&sb, r.Diagnostics)

	// Header
	fmt.Fprintf(&sb, "# project %s\n", path.Base(r.Module.Path))
	fmt.Fprintf(&sb, "module %s\n", r.Module.Path)
	fmt.Fprintf(&sb, "dir %s\n", r.Module.Dir)
	fmt.Fprintf(&sb, "go version %s\n", r.Module.Version)

	// Summary
	fmt.Fprintf(&sb, "\n## Summary\n")
	fmt.Fprintf(&sb, "%d packages, %d entry points, %d stdlib imports, %d third-party dependencies\n",
		r.Summary.Packages, r.Summary.EntryPoints, r.Summary.StdlibDeps, r.Summary.ThirdPartyDeps)

	// Entry Points
	fmt.Fprintf(&sb, "\n## Entry Points (%d)\n", len(r.EntryPoints))
	if len(r.EntryPoints) == 0 {
		sb.WriteString("(no main packages)\n")
	} else {
		for _, ep := range r.EntryPoints {
			fmt.Fprintf(&sb, "%s // %s\n", ep.Symbol, ep.Location)
		}
	}

	// Core Packages with dual-bar histogram
	fmt.Fprintf(&sb, "\n## Core Packages (%d)\n", len(r.CorePackages))
	if len(r.CorePackages) > 0 {
		renderCorePackagesHistogram(&sb, r.CorePackages)
	}

	// Cross-Package Interfaces
	fmt.Fprintf(&sb, "\n## Cross-Package Interfaces (%d)\n", len(r.Interfaces))
	for _, iface := range r.Interfaces {
		fmt.Fprintf(&sb, "%s\n", iface.Interface)
		for i, impl := range iface.ImplementedBy {
			if i == len(iface.ImplementedBy)-1 {
				fmt.Fprintf(&sb, "└── %s\n", impl)
			} else {
				fmt.Fprintf(&sb, "├── %s\n", impl)
			}
		}
	}

	// Stdlib Interface Aliases (only show if non-empty)
	if len(r.StdlibAliases) > 0 {
		fmt.Fprintf(&sb, "\n## Stdlib Interface Aliases (%d)\n", len(r.StdlibAliases))
		// Group by stdlib interface
		byStdlib := make(map[string][]string)
		var stdlibOrder []string
		for _, alias := range r.StdlibAliases {
			if _, exists := byStdlib[alias.Satisfies]; !exists {
				stdlibOrder = append(stdlibOrder, alias.Satisfies)
			}
			byStdlib[alias.Satisfies] = append(byStdlib[alias.Satisfies], alias.Interface)
		}
		for _, stdlib := range stdlibOrder {
			aliases := byStdlib[stdlib]
			fmt.Fprintf(&sb, "%s\n", stdlib)
			for i, alias := range aliases {
				if i == len(aliases)-1 {
					fmt.Fprintf(&sb, "└── %s\n", alias)
				} else {
					fmt.Fprintf(&sb, "├── %s\n", alias)
				}
			}
		}
	}

	// Third-Party Dependencies
	fmt.Fprintf(&sb, "\n## Third-Party Dependencies (%d)\n", len(r.ThirdPartyDeps))
	for _, dep := range r.ThirdPartyDeps {
		if len(dep.Users) > 0 {
			fmt.Fprintf(&sb, "%s (%s only)\n", dep.Path, strings.Join(dep.Users, ", "))
		} else {
			fmt.Fprintf(&sb, "%s\n", dep.Path)
		}
	}

	// Stdlib Imports - only show packages with limited usage
	var limitedStdlib []DepInfo
	for _, dep := range r.StdlibDeps {
		if len(dep.Users) > 0 {
			limitedStdlib = append(limitedStdlib, dep)
		}
	}
	if len(limitedStdlib) > 0 {
		fmt.Fprintf(&sb, "\n## Stdlib Imports - Specialized (%d of %d)\n", len(limitedStdlib), len(r.StdlibDeps))
		for _, dep := range limitedStdlib {
			fmt.Fprintf(&sb, "%s (%s only)\n", dep.Path, strings.Join(dep.Users, ", "))
		}
	}

	// Package Graph as tree
	fmt.Fprintf(&sb, "\n## Package Graph (%d nodes)\n", len(r.PackageGraph))
	renderPackageTree(&sb, r.PackageGraph)

	return []byte(sb.String()), nil
}

// renderCorePackagesHistogram renders packages with dual-bar histogram.
// Format: methods bar | package name | dependents bar
func renderCorePackagesHistogram(sb *strings.Builder, packages []CorePackage) {
	if len(packages) == 0 {
		return
	}

	// Pre-compute package+stats strings to find max width
	pkgStrs := make([]string, len(packages))
	maxPkgStrLen := 0
	for i, pkg := range packages {
		pkgStrs[i] = fmt.Sprintf("%s (%d files, %d symbols)", pkg.Path, pkg.Files, pkg.Symbols)
		if len(pkgStrs[i]) > maxPkgStrLen {
			maxPkgStrLen = len(pkgStrs[i])
		}
	}

	// Find max values for scaling
	maxMethods := 0
	maxDependents := 0
	for _, pkg := range packages {
		if pkg.Methods > maxMethods {
			maxMethods = pkg.Methods
		}
		if pkg.Dependents > maxDependents {
			maxDependents = pkg.Dependents
		}
	}

	// Bar width settings
	const maxBarWidth = 20

	for i, pkg := range packages {
		// Calculate bar widths
		methodBar := 0
		if maxMethods > 0 {
			methodBar = (pkg.Methods * maxBarWidth) / maxMethods
		}
		depBar := 0
		if maxDependents > 0 {
			depBar = (pkg.Dependents * maxBarWidth) / maxDependents
		}

		// Format: "N methods" bar | package (stats) | bar "imported N"
		methodBarStr := strings.Repeat(" ", maxBarWidth-methodBar) + strings.Repeat("█", methodBar)
		depBarStr := strings.Repeat("█", depBar)

		fmt.Fprintf(sb, "%3d methods %s %-*s %s imported by %d\n",
			pkg.Methods, methodBarStr,
			maxPkgStrLen, pkgStrs[i],
			depBarStr, pkg.Dependents)
	}
}

// renderPackageTree renders the package graph as dependency layers.
// Layer 0 = packages with no internal dependencies (foundation)
// Higher layers depend on lower layers.
func renderPackageTree(sb *strings.Builder, graph []PackageNode) {
	if len(graph) == 0 {
		return
	}

	// Build import map
	imports := make(map[string][]string)
	allPkgs := make(map[string]bool)
	for _, node := range graph {
		imports[node.Path] = node.Imports
		allPkgs[node.Path] = true
		for _, imp := range node.Imports {
			allPkgs[imp] = true
		}
	}

	// Compute layer for each package (0 = no internal deps, higher = more deps)
	layers := make(map[string]int)
	changed := true
	for changed {
		changed = false
		for pkg := range allPkgs {
			maxDep := -1
			for _, imp := range imports[pkg] {
				if l, ok := layers[imp]; ok && l > maxDep {
					maxDep = l
				}
			}
			newLayer := maxDep + 1
			if cur, ok := layers[pkg]; !ok || newLayer > cur {
				layers[pkg] = newLayer
				changed = true
			}
		}
	}

	// Group packages by layer
	byLayer := make(map[int][]string)
	maxLayer := 0
	for pkg, layer := range layers {
		byLayer[layer] = append(byLayer[layer], pkg)
		if layer > maxLayer {
			maxLayer = layer
		}
	}

	// Sort packages within each layer
	for layer := range byLayer {
		sort.Strings(byLayer[layer])
	}

	// Render layers
	for layer := 0; layer <= maxLayer; layer++ {
		pkgs := byLayer[layer]
		if len(pkgs) == 0 {
			continue
		}
		fmt.Fprintf(sb, "%d: %s\n", layer, strings.Join(pkgs, ", "))
	}
}
