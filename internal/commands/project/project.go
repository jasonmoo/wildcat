package project_cmd

import (
	"context"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/spf13/cobra"
)

type ProjectCommand struct{}

var _ commands.Command[*ProjectCommand] = (*ProjectCommand)(nil)

func NewProjectCommand() *ProjectCommand {
	return &ProjectCommand{}
}

func (c *ProjectCommand) Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "project",
		Short: "Show project-level overview and package relationships",
		Long: `Show a project-level overview optimized for AI orientation.

Provides:
- Entry points (main packages)
- Core packages ranked by dependents (architectural spine)
- Cross-package interfaces (architectural contracts)
- Package dependency graph

Examples:
  wildcat project`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.RunCommand(cmd, c)
		},
	}
}

func (c *ProjectCommand) README() string {
	return "TODO"
}

func (c *ProjectCommand) Execute(ctx context.Context, wc *commands.Wildcat, opts ...func(*ProjectCommand) error) (commands.Result, error) {

	// Compute package metrics
	pkgMetrics := make(map[string]*PackageMetrics)
	for _, pkg := range wc.Project.Packages {
		pm := &PackageMetrics{
			Path:      pkg.Identifier.PkgPath,
			ShortPath: pkg.Identifier.PkgShortPath,
			Dir:       pkg.Identifier.PkgDir,
			IsMain:    pkg.Identifier.Name == "main",
		}

		// Count files and lines
		for _, f := range pkg.Files {
			pm.Files++
			pm.Lines += f.LineCount
		}

		// Count symbols and methods
		pm.Symbols = len(pkg.Symbols)
		for _, sym := range pkg.Symbols {
			switch sym.Kind {
			case golang.SymbolKindFunc:
				pm.Functions++
			case golang.SymbolKindType, golang.SymbolKindInterface:
				pm.Types++
				pm.Methods += len(sym.Methods)
			}
		}

		// Collect imports (internal only for dependency analysis)
		for _, fi := range pkg.Imports {
			for _, imp := range fi.Imports {
				if imp.Package != nil { // internal package
					pm.Imports = append(pm.Imports, imp.Path)
				} else {
					pm.ExternalImports = append(pm.ExternalImports, imp.Path)
				}
			}
		}
		// Dedupe imports
		slices.Sort(pm.Imports)
		pm.Imports = slices.Compact(pm.Imports)
		slices.Sort(pm.ExternalImports)
		pm.ExternalImports = slices.Compact(pm.ExternalImports)

		pkgMetrics[pkg.Identifier.PkgPath] = pm
	}

	// Compute dependents (reverse of imports)
	for _, pm := range pkgMetrics {
		for _, imp := range pm.Imports {
			if target, ok := pkgMetrics[imp]; ok {
				target.Dependents = append(target.Dependents, pm.Path)
			}
		}
	}

	// Collect entry points (main packages) - find the main() function
	var entryPoints []EntryPoint
	for _, pkg := range wc.Project.Packages {
		if pkg.Identifier.Name != "main" {
			continue
		}
		for _, sym := range pkg.Symbols {
			if sym.Name == "main" && sym.Kind == golang.SymbolKindFunc {
				entryPoints = append(entryPoints, EntryPoint{
					Symbol:   pkg.Identifier.PkgPath + ".main",
					Location: sym.FileLocation(),
				})
				break
			}
		}
	}
	sort.Slice(entryPoints, func(i, j int) bool {
		return entryPoints[i].Symbol < entryPoints[j].Symbol
	})

	// Build core packages list sorted by dependents (descending)
	var corePackages []CorePackage
	for _, pm := range pkgMetrics {
		if pm.IsMain {
			continue // skip main packages in core list
		}
		corePackages = append(corePackages, CorePackage{
			Path:       pm.ShortPath,
			Methods:    pm.Methods + pm.Functions, // total callable symbols
			Dependents: len(pm.Dependents),
			Files:      pm.Files,
			Symbols:    pm.Symbols,
			Lines:      pm.Lines,
		})
	}
	sort.Slice(corePackages, func(i, j int) bool {
		// Sort by dependents desc, then by path asc
		if corePackages[i].Dependents != corePackages[j].Dependents {
			return corePackages[i].Dependents > corePackages[j].Dependents
		}
		return corePackages[i].Path < corePackages[j].Path
	})

	// Find cross-package interfaces
	crossIfaces := c.findCrossPackageInterfaces(wc)

	// Find stdlib interface aliases (project interfaces identical to stdlib/builtin)
	stdlibAliases := c.findStdlibInterfaceAliases(wc)

	// Collect external dependencies, tracking which packages use each
	stdlibUsers := make(map[string][]string)  // dep -> list of packages using it
	thirdPartyUsers := make(map[string][]string)
	for _, pm := range pkgMetrics {
		pkgShort := pm.ShortPath
		if pkgShort == "" || pkgShort == pm.Path {
			pkgShort = path.Base(pm.Path)
		}
		for _, ext := range pm.ExternalImports {
			if isStdlib(ext) {
				stdlibUsers[ext] = append(stdlibUsers[ext], pkgShort)
			} else {
				thirdPartyUsers[ext] = append(thirdPartyUsers[ext], pkgShort)
			}
		}
	}

	// Build DepInfo slices, only including users if <=2 packages
	var stdlibList []DepInfo
	for dep, users := range stdlibUsers {
		di := DepInfo{Path: dep}
		if len(users) <= 2 {
			sort.Strings(users)
			di.Users = users
		}
		stdlibList = append(stdlibList, di)
	}
	sort.Slice(stdlibList, func(i, j int) bool {
		return stdlibList[i].Path < stdlibList[j].Path
	})

	var thirdPartyList []DepInfo
	for dep, users := range thirdPartyUsers {
		di := DepInfo{Path: dep}
		if len(users) <= 2 {
			sort.Strings(users)
			di.Users = users
		}
		thirdPartyList = append(thirdPartyList, di)
	}
	sort.Slice(thirdPartyList, func(i, j int) bool {
		return thirdPartyList[i].Path < thirdPartyList[j].Path
	})

	// Build package graph (internal dependencies only)
	var pkgGraph []PackageNode
	modulePath := wc.Project.Module.Path
	moduleName := path.Base(modulePath)
	for _, pm := range pkgMetrics {
		if len(pm.Imports) > 0 {
			p := pm.ShortPath
			if p == "" || p == pm.Path {
				p = moduleName
			}
			pkgGraph = append(pkgGraph, PackageNode{
				Path:    p,
				Imports: shortenPaths(pm.Imports, modulePath),
			})
		}
	}
	sort.Slice(pkgGraph, func(i, j int) bool {
		return pkgGraph[i].Path < pkgGraph[j].Path
	})

	return &ProjectCommandResponse{
		Module: ModuleInfo{
			Path:    wc.Project.Module.Path,
			Dir:     wc.Project.Module.Dir,
			Version: wc.Project.Module.GoVersion,
		},
		Summary: ProjectSummary{
			Packages:      len(wc.Project.Packages),
			EntryPoints:   len(entryPoints),
			StdlibDeps:    len(stdlibList),
			ThirdPartyDeps: len(thirdPartyList),
		},
		EntryPoints:    entryPoints,
		CorePackages:   corePackages,
		Interfaces:     crossIfaces,
		StdlibAliases:  stdlibAliases,
		StdlibDeps:     stdlibList,
		ThirdPartyDeps: thirdPartyList,
		PackageGraph:   pkgGraph,
	}, nil
}

// findCrossPackageInterfaces finds interfaces that are implemented by types in other packages.
func (c *ProjectCommand) findCrossPackageInterfaces(wc *commands.Wildcat) []CrossPackageInterface {
	var result []CrossPackageInterface

	// Look for interfaces with ImplementedBy from different packages
	for _, pkg := range wc.Project.Packages {
		for _, sym := range pkg.Symbols {
			if sym.Kind != golang.SymbolKindInterface {
				continue
			}
			if len(sym.ImplementedBy) == 0 {
				continue
			}

			// Skip interfaces that are aliases for stdlib interfaces (e.g., `type MyError error`)
			if sym.StdlibEquivalent != nil {
				continue
			}

			// Check if any implementors are from different packages
			var externalImpls []string
			for _, impl := range sym.ImplementedBy {
				if impl.PackageIdentifier.PkgPath != pkg.Identifier.PkgPath {
					externalImpls = append(externalImpls, impl.PkgShortSymbol())
				}
			}

			if len(externalImpls) > 0 {
				sort.Strings(externalImpls)
				result = append(result, CrossPackageInterface{
					Interface:     sym.PkgShortSymbol(),
					Package:       pkg.Identifier.PkgShortPath,
					ImplementedBy: externalImpls,
				})
			}
		}
	}

	// Sort by interface name
	sort.Slice(result, func(i, j int) bool {
		return result[i].Interface < result[j].Interface
	})

	return result
}

// findStdlibInterfaceAliases finds project interfaces that are identical to stdlib/builtin interfaces.
func (c *ProjectCommand) findStdlibInterfaceAliases(wc *commands.Wildcat) []StdlibInterfaceAlias {
	var result []StdlibInterfaceAlias

	for _, pkg := range wc.Project.Packages {
		for _, sym := range pkg.Symbols {
			if sym.Kind != golang.SymbolKindInterface {
				continue
			}
			if sym.StdlibEquivalent == nil {
				continue
			}

			result = append(result, StdlibInterfaceAlias{
				Interface: sym.PkgShortSymbol(),
				Satisfies: sym.StdlibEquivalent.PkgSymbol(),
			})
		}
	}

	// Sort by interface name
	sort.Slice(result, func(i, j int) bool {
		return result[i].Interface < result[j].Interface
	})

	return result
}

// shortenPaths converts full package paths to short paths relative to module.
func shortenPaths(paths []string, modulePath string) []string {
	result := make([]string, len(paths))
	for i, p := range paths {
		result[i] = strings.TrimPrefix(p, modulePath+"/")
	}
	return result
}

// isStdlib returns true if the import path is a Go standard library package.
func isStdlib(path string) bool {
	// Stdlib packages don't contain a dot in the first path component
	// e.g., "fmt", "go/ast", "encoding/json" are stdlib
	// e.g., "github.com/foo/bar", "golang.org/x/tools" are not
	firstSlash := strings.Index(path, "/")
	firstComponent := path
	if firstSlash >= 0 {
		firstComponent = path[:firstSlash]
	}
	return !strings.Contains(firstComponent, ".")
}

// PackageMetrics holds computed metrics for a package.
type PackageMetrics struct {
	Path            string
	ShortPath       string
	Dir             string
	IsMain          bool
	Files           int
	Lines           int
	Symbols         int
	Functions       int
	Types           int
	Methods         int
	Imports         []string // internal package imports
	ExternalImports []string // external package imports
	Dependents      []string // packages that import this one
}
