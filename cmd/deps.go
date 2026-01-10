package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/spf13/cobra"
)

var depsCmd = &cobra.Command{
	Use:   "deps [package]",
	Short: "Show package dependencies",
	Long: `Show package dependencies.

By default shows what the package imports. Use --reverse to show
what packages import this package.

Examples:
  wildcat deps                           # Current package imports
  wildcat deps ./internal/lsp            # Specific package imports
  wildcat deps ./cmd --reverse           # What imports ./cmd
  wildcat deps --exclude-stdlib          # Exclude standard library`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeps,
}

var (
	depsReverse       bool
	depsExcludeStdlib bool
	depsDepth         int
)

func init() {
	rootCmd.AddCommand(depsCmd)

	depsCmd.Flags().BoolVar(&depsReverse, "reverse", false, "Show what imports this package")
	depsCmd.Flags().BoolVar(&depsExcludeStdlib, "exclude-stdlib", false, "Exclude standard library")
	depsCmd.Flags().IntVar(&depsDepth, "depth", 1, "Transitive depth (1 = direct only)")
}

// goListPackage represents the JSON output from `go list -json`
type goListPackage struct {
	Dir         string   `json:"Dir"`
	ImportPath  string   `json:"ImportPath"`
	Name        string   `json:"Name"`
	GoFiles     []string `json:"GoFiles"`
	Imports     []string `json:"Imports"`
	Deps        []string `json:"Deps"`
	TestImports []string `json:"TestImports"`
}

func runDeps(cmd *cobra.Command, args []string) error {
	writer, err := GetWriter(os.Stdout)
	if err != nil {
		return fmt.Errorf("invalid output format: %w", err)
	}

	// Determine target package
	pkgPath := "."
	if len(args) > 0 {
		pkgPath = args[0]
	}

	// Get working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	if depsReverse {
		return runReverseDeps(writer, workDir, pkgPath)
	}

	return runForwardDeps(writer, workDir, pkgPath)
}

func runForwardDeps(writer *output.Writer, workDir, pkgPath string) error {
	// Run go list -json
	cmd := exec.Command("go", "list", "-json", pkgPath)
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return writer.WriteError(
				"go_list_error",
				fmt.Sprintf("go list failed: %s", string(exitErr.Stderr)),
				nil,
				nil,
			)
		}
		return writer.WriteError("go_list_error", err.Error(), nil, nil)
	}

	var pkg goListPackage
	if err := json.Unmarshal(out, &pkg); err != nil {
		return writer.WriteError("parse_error", err.Error(), nil, nil)
	}

	// Build dependency list
	var deps []output.DepResult
	imports := pkg.Imports
	if depsDepth > 1 {
		imports = pkg.Deps // Use transitive deps
	}

	for _, imp := range imports {
		if depsExcludeStdlib && isStdlib(imp) {
			continue
		}

		// Find the import location in source files
		importFile, importLine := findImportLocation(pkg.Dir, pkg.GoFiles, imp)

		deps = append(deps, output.DepResult{
			Package:    imp,
			ImportFile: importFile,
			ImportLine: importLine,
		})
	}

	response := output.DepsResponse{
		Query: output.QueryInfo{
			Command: "deps",
			Target:  pkgPath,
		},
		Package:      pkg.ImportPath,
		Direction:    "imports",
		Dependencies: deps,
		Summary: output.Summary{
			Count: len(deps),
		},
	}

	return writer.Write(response)
}

func runReverseDeps(writer *output.Writer, workDir, pkgPath string) error {
	// First get the import path of the target package
	cmd := exec.Command("go", "list", "-json", pkgPath)
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return writer.WriteError("go_list_error", err.Error(), nil, nil)
	}

	var targetPkg goListPackage
	if err := json.Unmarshal(out, &targetPkg); err != nil {
		return writer.WriteError("parse_error", err.Error(), nil, nil)
	}

	// List all packages in the module
	cmd = exec.Command("go", "list", "-json", "./...")
	cmd.Dir = workDir
	out, err = cmd.Output()
	if err != nil {
		return writer.WriteError("go_list_error", err.Error(), nil, nil)
	}

	// Parse multiple JSON objects (go list outputs one per line)
	var deps []output.DepResult
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for decoder.More() {
		var pkg goListPackage
		if err := decoder.Decode(&pkg); err != nil {
			continue
		}

		// Check if this package imports our target
		for _, imp := range pkg.Imports {
			if imp == targetPkg.ImportPath {
				importFile, importLine := findImportLocation(pkg.Dir, pkg.GoFiles, imp)
				deps = append(deps, output.DepResult{
					Package:    pkg.ImportPath,
					ImportFile: importFile,
					ImportLine: importLine,
				})
				break
			}
		}
	}

	response := output.DepsResponse{
		Query: output.QueryInfo{
			Command: "deps",
			Target:  pkgPath,
		},
		Package:      targetPkg.ImportPath,
		Direction:    "imported_by",
		Dependencies: deps,
		Summary: output.Summary{
			Count: len(deps),
		},
	}

	return writer.Write(response)
}

// isStdlib checks if an import path is from the standard library.
func isStdlib(importPath string) bool {
	// Standard library packages don't have dots in their first path component
	firstSlash := strings.Index(importPath, "/")
	if firstSlash == -1 {
		return !strings.Contains(importPath, ".")
	}
	firstPart := importPath[:firstSlash]
	return !strings.Contains(firstPart, ".")
}

// findImportLocation finds where a package is imported in source files.
func findImportLocation(dir string, files []string, importPath string) (string, int) {
	for _, file := range files {
		fullPath := filepath.Join(dir, file)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			// Simple heuristic: look for the import path in quotes
			if strings.Contains(line, `"`+importPath+`"`) {
				return fullPath, i + 1
			}
		}
	}
	return "", 0
}
