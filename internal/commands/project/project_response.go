package project_cmd

import (
	"encoding/json"
	"fmt"
	"path"
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

	// Third-Party Dependencies
	fmt.Fprintf(&sb, "\n## Third-Party Dependencies (%d)\n", len(r.ThirdPartyDeps))
	for _, dep := range r.ThirdPartyDeps {
		if len(dep.Users) > 0 {
			fmt.Fprintf(&sb, "%s (%s only)\n", dep.Path, strings.Join(dep.Users, ", "))
		} else {
			fmt.Fprintf(&sb, "%s\n", dep.Path)
		}
	}

	// Stdlib Imports
	fmt.Fprintf(&sb, "\n## Stdlib Imports (%d)\n", len(r.StdlibDeps))
	for _, dep := range r.StdlibDeps {
		if len(dep.Users) > 0 {
			fmt.Fprintf(&sb, "%s (%s only)\n", dep.Path, strings.Join(dep.Users, ", "))
		} else {
			fmt.Fprintf(&sb, "%s\n", dep.Path)
		}
	}

	// Package Graph
	fmt.Fprintf(&sb, "\n## Package Graph (%d nodes)\n", len(r.PackageGraph))
	for _, node := range r.PackageGraph {
		fmt.Fprintf(&sb, "%s\n", node.Path)
		for i, imp := range node.Imports {
			if i == len(node.Imports)-1 {
				fmt.Fprintf(&sb, "└── %s\n", imp)
			} else {
				fmt.Fprintf(&sb, "├── %s\n", imp)
			}
		}
	}

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

		fmt.Fprintf(sb, "%3d methods %s %-*s %s imported %d\n",
			pkg.Methods, methodBarStr,
			maxPkgStrLen, pkgStrs[i],
			depBarStr, pkg.Dependents)
	}
}
