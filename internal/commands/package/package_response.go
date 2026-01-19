package package_cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jasonmoo/wildcat/internal/output"
)

type PackageCommandResponse struct {
	Query      output.QueryInfo       `json:"query"`
	Package    output.PackageInfo     `json:"package"`
	Summary    output.PackageSummary  `json:"summary"`
	Files      []output.FileInfo      `json:"files"`
	Constants  []output.PackageSymbol `json:"constants"`
	Variables  []output.PackageSymbol `json:"variables"`
	Functions  []output.PackageSymbol `json:"functions"`
	Types      []output.PackageType   `json:"types"`
	Imports    []output.DepResult     `json:"imports"`
	ImportedBy []output.DepResult     `json:"imported_by"`
}

func (resp *PackageCommandResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(resp)
}

func (resp *PackageCommandResponse) MarshalMarkdown() ([]byte, error) {
	return []byte(renderPackageMarkdown(resp)), nil
}

// renderPackageMarkdown renders the package response as compressed markdown.
func renderPackageMarkdown(r *PackageCommandResponse) string {
	var sb strings.Builder

	// Header
	sb.WriteString("# package ")
	sb.WriteString(r.Package.ImportPath)
	sb.WriteString("\n# dir ")
	sb.WriteString(r.Package.Dir)
	sb.WriteString("\n")

	// Files - calculate total lines first
	totalLines := 0
	for _, f := range r.Files {
		totalLines += f.LineCount
	}
	sb.WriteString(fmt.Sprintf("\n# Files (%d lines)\n", totalLines))
	for _, f := range r.Files {
		sb.WriteString(fmt.Sprintf("%s // %d lines\n", f.Name, f.LineCount))
	}

	// Constants
	sb.WriteString(fmt.Sprintf("\n# Constants (%d)\n", len(r.Constants)))
	for _, c := range r.Constants {
		writeSymbolMd(&sb, c.Signature, c.Location)
	}

	// Variables
	sb.WriteString(fmt.Sprintf("\n# Variables (%d)\n", len(r.Variables)))
	for _, v := range r.Variables {
		writeSymbolMd(&sb, v.Signature, v.Location)
	}

	// Functions (standalone, not constructors)
	sb.WriteString(fmt.Sprintf("\n# Functions (%d)\n", len(r.Functions)))
	for _, f := range r.Functions {
		writeSymbolMd(&sb, f.Signature, f.Location)
	}

	// Types
	sb.WriteString(fmt.Sprintf("\n# Types (%d)\n", len(r.Types)))
	for _, t := range r.Types {
		sb.WriteString("\n")
		// Build location with method count and interface info
		loc := t.Location
		if len(t.Methods) > 0 {
			loc += fmt.Sprintf(" // %d methods", len(t.Methods))
		}
		if len(t.Satisfies) > 0 {
			loc += ", satisfies: " + strings.Join(t.Satisfies, ", ")
		}
		if len(t.ImplementedBy) > 0 {
			loc += ", implemented by: " + strings.Join(t.ImplementedBy, ", ")
		}
		writeSymbolMd(&sb, t.Signature, loc)

		// Constructor functions
		for _, f := range t.Functions {
			writeSymbolMd(&sb, f.Signature, f.Location)
		}

		// Methods
		for _, m := range t.Methods {
			writeSymbolMd(&sb, m.Signature, m.Location)
		}
	}

	// Imports grouped by file
	sb.WriteString(fmt.Sprintf("\n# Imports (%d)\n", len(r.Imports)))
	if len(r.Imports) > 0 {
		// Group by file and track line ranges
		type fileImports struct {
			packages []string
			minLine  int
			maxLine  int
		}
		byFile := make(map[string]*fileImports)
		var fileOrder []string

		for _, imp := range r.Imports {
			if imp.Location == "" {
				continue
			}
			// Parse file:line from location
			file, line := parseFileLineFromLocation(imp.Location)
			if file == "" {
				continue
			}

			if fi, ok := byFile[file]; ok {
				fi.packages = append(fi.packages, imp.Package)
				if line < fi.minLine {
					fi.minLine = line
				}
				if line > fi.maxLine {
					fi.maxLine = line
				}
			} else {
				byFile[file] = &fileImports{
					packages: []string{imp.Package},
					minLine:  line,
					maxLine:  line,
				}
				fileOrder = append(fileOrder, file)
			}
		}

		// Output grouped by file
		for _, file := range fileOrder {
			fi := byFile[file]
			sb.WriteString(fmt.Sprintf("# %s:%d:%d\n", file, fi.minLine, fi.maxLine))
			for _, pkg := range fi.packages {
				sb.WriteString(pkg)
				sb.WriteString("\n")
			}
		}
	}

	// Imported By
	sb.WriteString(fmt.Sprintf("\n# Imported By (%d)\n", len(r.ImportedBy)))
	for _, imp := range r.ImportedBy {
		sb.WriteString(imp.Package)
		if imp.Location != "" {
			sb.WriteString(" // ")
			sb.WriteString(imp.Location)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// writeSymbolMd writes a symbol with its location as a trailing comment.
func writeSymbolMd(sb *strings.Builder, signature, location string) {
	sb.WriteString(signature)
	sb.WriteString(" // ")
	sb.WriteString(location)
	sb.WriteString("\n")
}
