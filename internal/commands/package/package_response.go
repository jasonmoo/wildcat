package package_cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
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

// ChannelOp represents a single channel operation.
type ChannelOp struct {
	Kind      string `json:"kind"`      // make, send, recv, close, select_send, select_recv
	Operation string `json:"operation"` // the code snippet
	Location  string `json:"location"`  // file:line
}

// ChannelFunc groups channel operations by enclosing function.
type ChannelFunc struct {
	Signature  string            `json:"signature"`            // function signature
	Definition string            `json:"definition"`           // file:line
	Refs       *output.TargetRefs `json:"refs,omitempty"`       // callers info
	Operations []ChannelOp       `json:"operations"`           // channel ops in this function
}

// ChannelGroup groups channel operations by element type.
type ChannelGroup struct {
	ElementType string        `json:"element_type"`
	Makes       []ChannelOp   `json:"makes,omitempty"`     // make() calls (not grouped by func)
	Functions   []ChannelFunc `json:"functions,omitempty"` // operations grouped by enclosing function
}

type PackageCommandResponse struct {
	Query       output.QueryInfo       `json:"query"`
	Package     output.PackageInfo     `json:"package"`
	Summary     output.PackageSummary  `json:"summary"`
	Files       []output.FileInfo      `json:"files"`
	Embeds      []EmbedInfo            `json:"embeds"`
	Constants   []output.PackageSymbol `json:"constants"`
	Variables   []output.PackageSymbol `json:"variables"`
	Functions   []output.PackageSymbol `json:"functions"`
	Types       []output.PackageType   `json:"types"`
	Channels    []ChannelGroup         `json:"channels"`
	Imports     []output.DepResult     `json:"imports"`
	ImportedBy  []output.DepResult     `json:"imported_by"`
	Diagnostics []commands.Diagnostics `json:"diagnostics,omitempty"`
}

var _ commands.Result = (*PackageCommandResponse)(nil)

func (r *PackageCommandResponse) SetDiagnostics(ds []commands.Diagnostics) {
	r.Diagnostics = ds
}

// MultiPackageResponse wraps multiple package responses for multi-package queries.
type MultiPackageResponse struct {
	Query       output.QueryInfo          `json:"query"`
	Packages    []*PackageCommandResponse `json:"packages"`
	Diagnostics []commands.Diagnostics    `json:"diagnostics,omitempty"`
}

var _ commands.Result = (*MultiPackageResponse)(nil)

func (r *MultiPackageResponse) SetDiagnostics(ds []commands.Diagnostics) {
	r.Diagnostics = ds
}

func (resp *MultiPackageResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Diagnostics []commands.Diagnostics    `json:"diagnostics,omitempty"`
		Query       output.QueryInfo          `json:"query"`
		Packages    []*PackageCommandResponse `json:"packages"`
	}{
		Diagnostics: resp.Diagnostics,
		Query:       resp.Query,
		Packages:    resp.Packages,
	})
}

func (resp *MultiPackageResponse) MarshalMarkdown() ([]byte, error) {
	var sb strings.Builder
	commands.FormatDiagnosticsMarkdown(&sb, resp.Diagnostics)
	for i, pkg := range resp.Packages {
		if i > 0 {
			sb.WriteString("\n---\n\n")
		}
		sb.WriteString(renderPackageMarkdown(pkg))
	}
	return []byte(sb.String()), nil
}

func (resp *PackageCommandResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Diagnostics []commands.Diagnostics `json:"diagnostics,omitempty"`
		Query       output.QueryInfo       `json:"query"`
		Package     output.PackageInfo     `json:"package"`
		Summary     output.PackageSummary  `json:"summary"`
		Files       []output.FileInfo      `json:"files"`
		Embeds      []EmbedInfo            `json:"embeds"`
		Constants   []output.PackageSymbol `json:"constants"`
		Variables   []output.PackageSymbol `json:"variables"`
		Functions   []output.PackageSymbol `json:"functions"`
		Types       []output.PackageType   `json:"types"`
		Channels    []ChannelGroup         `json:"channels"`
		Imports     []output.DepResult     `json:"imports"`
		ImportedBy  []output.DepResult     `json:"imported_by"`
	}{
		Diagnostics: resp.Diagnostics,
		Query:       resp.Query,
		Package:     resp.Package,
		Summary:     resp.Summary,
		Files:       resp.Files,
		Embeds:      resp.Embeds,
		Constants:   resp.Constants,
		Variables:   resp.Variables,
		Functions:   resp.Functions,
		Types:       resp.Types,
		Channels:    resp.Channels,
		Imports:     resp.Imports,
		ImportedBy:  resp.ImportedBy,
	})
}

func (resp *PackageCommandResponse) MarshalMarkdown() ([]byte, error) {
	var sb strings.Builder
	commands.FormatDiagnosticsMarkdown(&sb, resp.Diagnostics)
	sb.WriteString(renderPackageMarkdown(resp))
	return []byte(sb.String()), nil
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
				fmt.Fprintf(&sb, "%s // %s, %d files, %s (%s)\n", e.Variable, e.Location, e.FileCount, e.TotalSize, e.Error)
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

	// Channels - always show header for clear signal
	if len(r.Channels) == 0 {
		sb.WriteString("\n# Channels (0)\n")
	} else {
		// Count total ops
		var totalOps int
		for _, g := range r.Channels {
			totalOps += len(g.Makes)
			for _, f := range g.Functions {
				totalOps += len(f.Operations)
			}
		}
		fmt.Fprintf(&sb, "\n# Channels (%d ops, %d types)\n", totalOps, len(r.Channels))
		for _, g := range r.Channels {
			fmt.Fprintf(&sb, "\n## chan %s\n", g.ElementType)
			// Makes are not grouped by function
			for _, op := range g.Makes {
				fmt.Fprintf(&sb, "make: %s // %s\n", op.Operation, op.Location)
			}
			// Other operations grouped by enclosing function
			for _, f := range g.Functions {
				writeSymbolMd(&sb, f.Signature, f.Definition, f.Refs, true)
				for _, op := range f.Operations {
					fmt.Fprintf(&sb, "  %s: %s // %s\n", op.Kind, op.Operation, op.Location)
				}
			}
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
