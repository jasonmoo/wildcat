package package_cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jasonmoo/wildcat/internal/output"
)

// EmbedInfo describes a //go:embed directive.
type EmbedInfo struct {
	Patterns  []string `json:"patterns"`            // embed patterns (e.g., "templates/*", "static/*.css")
	Variable  string   `json:"variable"`            // variable name and type (e.g., "var templates embed.FS")
	Location  string   `json:"location"`            // file:line
	FileCount int      `json:"file_count"`          // number of files matched
	TotalSize string   `json:"total_size"`          // formatted size (e.g., "1.2KB")
	Error     string   `json:"error,omitempty"`     // error message if directive couldn't be fully processed
	rawSize   int64    // raw bytes for internal aggregation
}

type PackageCommandResponse struct {
	Query      output.QueryInfo       `json:"query"`
	Package    output.PackageInfo     `json:"package"`
	Summary    output.PackageSummary  `json:"summary"`
	Files      []output.FileInfo      `json:"files"`
	Embeds     []EmbedInfo            `json:"embeds,omitempty"`
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
		Embeds     []EmbedInfo            `json:"embeds,omitempty"`
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
		Embeds:     resp.Embeds,
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
	sb.WriteString(" // ")
	sb.WriteString(r.Package.Dir)
	sb.WriteString("\n")

	// Files - calculate totals
	totalLines := 0
	totalExported := 0
	totalUnexported := 0
	for _, f := range r.Files {
		totalLines += f.LineCount
		totalExported += f.Exported
		totalUnexported += f.Unexported
	}
	fmt.Fprintf(&sb, "\n# Files (%d files, %d lines, %d exported, %d unexported)\n", len(r.Files), totalLines, totalExported, totalUnexported)
	for _, f := range r.Files {
		fmt.Fprintf(&sb, "%s // %d lines, %d exported, %d unexported", f.Name, f.LineCount, f.Exported, f.Unexported)
		if f.Refs != nil {
			fmt.Fprintf(&sb, ", refs(%d pkg, %d proj, imported %d)", f.Refs.Internal, f.Refs.External, f.Refs.Packages)
		}
		sb.WriteString("\n")
	}

	// Embeds - always show header for clear signal
	if len(r.Embeds) == 0 {
		sb.WriteString("\n# Embeds (0)\n")
	} else {
		var totalEmbedSize int64
		var totalEmbedFiles int
		for _, e := range r.Embeds {
			totalEmbedSize += e.rawSize
			totalEmbedFiles += e.FileCount
		}
		fmt.Fprintf(&sb, "\n# Embeds (%d directives, %d files, %s)\n", len(r.Embeds), totalEmbedFiles, formatSize(totalEmbedSize))
		for _, e := range r.Embeds {
			fmt.Fprintf(&sb, "//go:embed %s\n", strings.Join(e.Patterns, " "))
			if e.Error != "" {
				fmt.Fprintf(&sb, "%s // %s, ERROR: %s\n", e.Variable, e.Location, e.Error)
			} else {
				fmt.Fprintf(&sb, "%s // %s, %d files, %s\n", e.Variable, e.Location, e.FileCount, e.TotalSize)
			}
		}
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

// formatSize formats a byte size in human-readable form.
func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1fKB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
