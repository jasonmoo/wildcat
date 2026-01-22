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
	return json.Marshal(struct {
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
	}{
		Query:      resp.Query,
		Package:    resp.Package,
		Summary:    resp.Summary,
		Files:      resp.Files,
		Constants:  resp.Constants,
		Variables:  resp.Variables,
		Functions:  resp.Functions,
		Types:      resp.Types,
		Imports:    resp.Imports,
		ImportedBy: resp.ImportedBy,
	})
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
	fmt.Fprintf(&sb, "\n# Files (%d lines)\n", totalLines)
	for _, f := range r.Files {
		fmt.Fprintf(&sb, "%s // %d lines\n", f.Name, f.LineCount)
	}

	// Constants
	fmt.Fprintf(&sb, "\n# Constants (%d)\n", len(r.Constants))
	for _, c := range r.Constants {
		writeSymbolMd(&sb, c.Signature, c.Location, c.Refs, false)
	}

	// Variables
	fmt.Fprintf(&sb, "\n# Variables (%d)\n", len(r.Variables))
	for _, v := range r.Variables {
		writeSymbolMd(&sb, v.Signature, v.Location, v.Refs, false)
	}

	// Functions (standalone, not constructors)
	fmt.Fprintf(&sb, "\n# Functions (%d)\n", len(r.Functions))
	for _, f := range r.Functions {
		writeSymbolMd(&sb, f.Signature, f.Location, f.Refs, true)
	}

	// Types
	fmt.Fprintf(&sb, "\n# Types (%d)\n", len(r.Types))
	for _, t := range r.Types {
		sb.WriteString("\n")
		// Build location with method count and interface info
		loc := t.Location
		if len(t.Methods) > 0 {
			loc += fmt.Sprintf(", %d methods", len(t.Methods))
		}
		if len(t.Satisfies) > 0 {
			loc += ", satisfies: " + strings.Join(t.Satisfies, ", ")
		}
		if len(t.ImplementedBy) > 0 {
			loc += ", implemented by: " + strings.Join(t.ImplementedBy, ", ")
		}
		writeSymbolMd(&sb, t.Signature, loc, t.Refs, false)

		// Constructor functions
		for _, f := range t.Functions {
			writeSymbolMd(&sb, f.Signature, f.Location, f.Refs, true)
		}

		// Methods
		for _, m := range t.Methods {
			writeSymbolMd(&sb, m.Signature, m.Location, m.Refs, true)
		}
	}

	// Imports - dedupe and list alphabetically (already sorted)
	seen := make(map[string]bool)
	var uniqueImports []string
	for _, imp := range r.Imports {
		if !seen[imp.Package] {
			seen[imp.Package] = true
			uniqueImports = append(uniqueImports, imp.Package)
		}
	}
	fmt.Fprintf(&sb, "\n# Imports (%d)\n", len(uniqueImports))
	for _, pkg := range uniqueImports {
		sb.WriteString(pkg)
		sb.WriteString("\n")
	}

	// Imported By
	fmt.Fprintf(&sb, "\n# Imported By (%d)\n", len(r.ImportedBy))
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
func writeSymbolMd(sb *strings.Builder, signature, location string, refs *output.TargetRefs, useCallers bool) {
	sb.WriteString(signature)
	sb.WriteString(" // ")
	sb.WriteString(location)
	if refs != nil {
		if useCallers {
			fmt.Fprintf(sb, ", callers(%d pkg, %d proj, imported %d)", refs.Internal, refs.External, refs.Packages)
		} else {
			fmt.Fprintf(sb, ", refs(%d pkg, %d proj, imported %d)", refs.Internal, refs.External, refs.Packages)
		}
	}
	sb.WriteString("\n")
}
