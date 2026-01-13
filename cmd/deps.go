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
	Short: "Show package dependencies (imports and imported_by)",
	Long: `Show package dependencies in both directions.

Returns what the package imports and what packages import it.

Examples:
  wildcat deps                     # Current package
  wildcat deps ./internal/lsp      # Specific package
  wildcat deps --exclude-stdlib    # Exclude standard library imports`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeps,
}

var (
	depsExcludeStdlib bool
)

func init() {
	rootCmd.AddCommand(depsCmd)

	depsCmd.Flags().BoolVar(&depsExcludeStdlib, "exclude-stdlib", false, "Exclude standard library from imports")
}

// goListPackage represents the JSON output from `go list -json`
type goListPackage struct {
	Dir        string   `json:"Dir"`
	ImportPath string   `json:"ImportPath"`
	Name       string   `json:"Name"`
	GoFiles    []string `json:"GoFiles"`
	Imports    []string `json:"Imports"`
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

	// Get target package info
	goCmd := exec.Command("go", "list", "-json", pkgPath)
	goCmd.Dir = workDir
	out, err := goCmd.Output()
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

	var targetPkg goListPackage
	if err := json.Unmarshal(out, &targetPkg); err != nil {
		return writer.WriteError("parse_error", err.Error(), nil, nil)
	}

	// Get imports (what this package imports)
	var imports []output.DepResult
	for _, imp := range targetPkg.Imports {
		if depsExcludeStdlib && isStdlib(imp) {
			continue
		}
		location := findImportLocation(targetPkg.Dir, targetPkg.GoFiles, imp)
		imports = append(imports, output.DepResult{
			Package:  imp,
			Location: location,
		})
	}

	// Get imported_by (what packages import this one)
	importedBy, err := findImportedBy(workDir, targetPkg.ImportPath)
	if err != nil {
		return writer.WriteError("go_list_error", err.Error(), nil, nil)
	}

	response := output.DepsResponse{
		Query: output.QueryInfo{
			Command: "deps",
			Target:  pkgPath,
		},
		Package:    targetPkg.ImportPath,
		Imports:    imports,
		ImportedBy: importedBy,
		Summary: output.DepsSummary{
			ImportsCount:    len(imports),
			ImportedByCount: len(importedBy),
		},
	}

	return writer.Write(response)
}

// findImportedBy finds all packages in the module that import the target.
func findImportedBy(workDir, targetImportPath string) ([]output.DepResult, error) {
	// List all packages in the module
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var results []output.DepResult
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for decoder.More() {
		var pkg goListPackage
		if err := decoder.Decode(&pkg); err != nil {
			continue
		}

		// Check if this package imports our target
		for _, imp := range pkg.Imports {
			if imp == targetImportPath {
				location := findImportLocation(pkg.Dir, pkg.GoFiles, targetImportPath)
				results = append(results, output.DepResult{
					Package:  pkg.ImportPath,
					Location: location,
				})
				break
			}
		}
	}

	return results, nil
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
// Returns file:line format or empty string if not found.
func findImportLocation(dir string, files []string, importPath string) string {
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
				return fmt.Sprintf("%s:%d", fullPath, i+1)
			}
		}
	}
	return ""
}
