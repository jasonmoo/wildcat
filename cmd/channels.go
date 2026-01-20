package cmd

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/spf13/cobra"
	"golang.org/x/tools/go/packages"
)

var channelsCmd = &cobra.Command{
	Use:   "channels [package] ...",
	Short: "Show channel operations in packages",
	Long: `Report all channel operations grouped by package and element type.

Shows makes, sends, receives, closes, and select cases for channels.
Useful for understanding concurrency patterns without precise pointer analysis.

Examples:
  wildcat channels                         # Current package
  wildcat channels ./internal/lsp          # Specific package
  wildcat channels internal/lsp internal/output  # Multiple packages`,
	RunE: runChannels,
}

var (
	channelsExcludeTests bool
)

func init() {
	channelsCmd.Flags().BoolVar(&channelsExcludeTests, "exclude-tests", false, "Exclude test files")
}

// ChannelGroup groups operations by element type (single-line format: "snippet // file.go:line")
type ChannelGroup struct {
	ElementType    string   `json:"element_type"`
	Makes          []string `json:"makes,omitempty"`
	Sends          []string `json:"sends,omitempty"`
	Receives       []string `json:"receives,omitempty"`
	Closes         []string `json:"closes,omitempty"`
	SelectSends    []string `json:"select_sends,omitempty"`
	SelectReceives []string `json:"select_receives,omitempty"`
}

// PackageChannels groups channel operations by package
type PackageChannels struct {
	Package  string         `json:"package"`
	Channels []ChannelGroup `json:"channels,omitempty"`
	Message  string         `json:"message,omitempty"`
}

// ChannelsResponse is the output format
type ChannelsResponse struct {
	Query    output.QueryInfo  `json:"query"`
	Packages []PackageChannels `json:"packages"`
	Summary  ChannelsSummary   `json:"summary"`
}

type ChannelsSummary struct {
	TotalOps int            `json:"total_ops"`
	ByKind   map[string]int `json:"by_kind"`
	Packages int            `json:"packages"`
	Types    int            `json:"types"`
}

func runChannels(cmd *cobra.Command, args []string) error {
	writer, err := GetWriter(os.Stdout)
	if err != nil {
		return fmt.Errorf("invalid output format: %w", err)
	}

	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Resolve package paths (default to current directory)
	if len(args) == 0 {
		args = []string{"."}
	}
	pkgPaths := make([]string, 0, len(args))
	for _, arg := range args {
		resolved, err := golang.ResolvePackagePath(arg, workDir)
		if err != nil {
			return writer.WriteError("package_not_found", fmt.Sprintf("cannot resolve %q: %v", arg, err), nil, nil)
		}
		pkgPaths = append(pkgPaths, resolved)
	}

	// Build query target string for output
	queryTarget := strings.Join(pkgPaths, ", ")

	// Load packages with type information
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir:   workDir,
		Tests: !channelsExcludeTests,
	}

	pkgs, err := packages.Load(cfg, pkgPaths...)
	if err != nil {
		return writer.WriteError("load_error", err.Error(), nil, nil)
	}

	// Collect channel operations with type info
	collector := newChannelCollector()
	seenFiles := make(map[string]bool)

	for _, pkg := range pkgs {
		// Skip synthetic test binary packages created by packages.Load
		if strings.HasSuffix(pkg.PkgPath, ".test") {
			continue
		}
		if pkg.TypesInfo == nil {
			continue
		}
		for _, f := range pkg.Syntax {
			filename := pkg.Fset.Position(f.Pos()).Filename
			if seenFiles[filename] {
				continue
			}
			seenFiles[filename] = true
			if channelsExcludeTests && strings.HasSuffix(filename, "_test.go") {
				continue
			}
			collector.processFile(pkg.Fset, pkg.TypesInfo, pkg.PkgPath, filename, f)
		}
	}

	// Build response
	pkgChannels := collector.buildPackages()

	summary := ChannelsSummary{
		ByKind:   make(map[string]int),
		Packages: len(pkgChannels),
	}
	typeSet := make(map[string]bool)
	for _, pc := range pkgChannels {
		for _, g := range pc.Channels {
			typeSet[g.ElementType] = true
			summary.TotalOps += len(g.Makes) + len(g.Sends) + len(g.Receives) + len(g.Closes) + len(g.SelectSends) + len(g.SelectReceives)
			summary.ByKind["make"] += len(g.Makes)
			summary.ByKind["send"] += len(g.Sends)
			summary.ByKind["receive"] += len(g.Receives)
			summary.ByKind["close"] += len(g.Closes)
			summary.ByKind["select_send"] += len(g.SelectSends)
			summary.ByKind["select_receive"] += len(g.SelectReceives)
		}
	}
	summary.Types = len(typeSet)

	response := ChannelsResponse{
		Query: output.QueryInfo{
			Command: "channels",
			Target:  queryTarget,
		},
		Packages: pkgChannels,
		Summary:  summary,
	}

	// Use custom markdown renderer for channels
	if globalOutput == "markdown" {
		fmt.Print(renderChannelsMarkdown(response))
		return nil
	}

	return writer.Write(response)
}

// opInfo holds a single channel operation
type opInfo struct {
	kind     string // make, send, receive, close, range, select_send, select_receive
	elemType string
	line     string // "snippet // file.go:line"
}

type channelCollector struct {
	// package -> list of ops
	ops map[string][]opInfo
}

func newChannelCollector() *channelCollector {
	return &channelCollector{
		ops: make(map[string][]opInfo),
	}
}

func (c *channelCollector) processFile(fset *token.FileSet, info *types.Info, pkgPath, filename string, f *ast.File) {
	// Ensure package appears in output even if no channel ops found
	if _, ok := c.ops[pkgPath]; !ok {
		c.ops[pkgPath] = nil
	}

	// Track nodes that are part of select cases so we don't double-count them
	selectNodes := make(map[ast.Node]bool)

	// First pass: identify all select statement channel operations
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectStmt); ok {
			for _, stmt := range sel.Body.List {
				if comm, ok := stmt.(*ast.CommClause); ok && comm.Comm != nil {
					switch node := comm.Comm.(type) {
					case *ast.SendStmt:
						selectNodes[node] = true
						if elemType := c.channelElemType(info, node.Chan); elemType != "" {
							c.addOp(fset, pkgPath, filename, "select_send", node, elemType)
						}
					case *ast.ExprStmt:
						// <-ch in select (value discarded)
						if recv, ok := node.X.(*ast.UnaryExpr); ok && recv.Op == token.ARROW {
							selectNodes[recv] = true
							if elemType := c.channelElemType(info, recv.X); elemType != "" {
								c.addOp(fset, pkgPath, filename, "select_receive", recv, elemType)
							}
						}
					case *ast.AssignStmt:
						// x := <-ch or x, ok := <-ch in select
						if len(node.Rhs) == 1 {
							if recv, ok := node.Rhs[0].(*ast.UnaryExpr); ok && recv.Op == token.ARROW {
								selectNodes[recv] = true
								if elemType := c.channelElemType(info, recv.X); elemType != "" {
									c.addOp(fset, pkgPath, filename, "select_receive", recv, elemType)
								}
							}
						}
					}
				}
			}
		}
		return true
	})

	// Second pass: collect non-select channel operations
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SendStmt:
			if selectNodes[node] {
				return true // already handled as select case
			}
			// ch <- value
			if elemType := c.channelElemType(info, node.Chan); elemType != "" {
				c.addOp(fset, pkgPath, filename, "send", node, elemType)
			}

		case *ast.UnaryExpr:
			if selectNodes[node] {
				return true // already handled as select case
			}
			// <-ch
			if node.Op == token.ARROW {
				if elemType := c.channelElemType(info, node.X); elemType != "" {
					c.addOp(fset, pkgPath, filename, "receive", node, elemType)
				}
			}

		case *ast.CallExpr:
			// make(chan T) or close(ch)
			if ident, ok := node.Fun.(*ast.Ident); ok {
				switch ident.Name {
				case "close":
					if len(node.Args) > 0 {
						if elemType := c.channelElemType(info, node.Args[0]); elemType != "" {
							c.addOp(fset, pkgPath, filename, "close", node, elemType)
						}
					}
				case "make":
					if t := info.TypeOf(node); t != nil {
						if ch, ok := t.Underlying().(*types.Chan); ok {
							elemType := ch.Elem().String()
							c.addOp(fset, pkgPath, filename, "make", node, elemType)
						}
					}
				}
			}

		case *ast.RangeStmt:
			// for x := range ch
			if elemType := c.channelElemType(info, node.X); elemType != "" {
				c.addOp(fset, pkgPath, filename, "range", node, elemType)
			}
		}
		return true
	})
}

func (c *channelCollector) channelElemType(info *types.Info, expr ast.Expr) string {
	if t := info.TypeOf(expr); t != nil {
		if ch, ok := t.Underlying().(*types.Chan); ok {
			return ch.Elem().String()
		}
	}
	return ""
}

func (c *channelCollector) addOp(fset *token.FileSet, pkgPath, filename string, kind string, node ast.Node, elemType string) {
	pos := fset.Position(node.Pos())
	snippet := c.extractSnippet(fset, node)
	base := filepath.Base(filename)

	// Format as "snippet // file.go:line"
	line := fmt.Sprintf("%s // %s:%d", snippet, base, pos.Line)

	c.ops[pkgPath] = append(c.ops[pkgPath], opInfo{
		kind:     kind,
		elemType: elemType,
		line:     line,
	})
}

func (c *channelCollector) extractSnippet(fset *token.FileSet, node ast.Node) string {
	start := fset.Position(node.Pos())
	end := fset.Position(node.End())

	content, err := os.ReadFile(start.Filename)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(content), "\n")
	if start.Line > len(lines) {
		return ""
	}

	// Single line
	if start.Line == end.Line {
		return strings.TrimSpace(lines[start.Line-1])
	}

	// Multiline - extract and dedent
	extracted := make([]string, 0, end.Line-start.Line+1)
	for i := start.Line - 1; i < end.Line && i < len(lines); i++ {
		extracted = append(extracted, lines[i])
	}
	return dedent(extracted)
}

// dedent removes common leading whitespace from all lines
func dedent(lines []string) string {
	if len(lines) == 0 {
		return ""
	}

	// Find minimum indent (ignoring empty lines)
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if minIndent < 0 || indent < minIndent {
			minIndent = indent
		}
	}

	if minIndent <= 0 {
		return strings.Join(lines, "\n")
	}

	// Remove common indent
	result := make([]string, len(lines))
	for i, line := range lines {
		if len(line) >= minIndent {
			result[i] = line[minIndent:]
		} else {
			result[i] = strings.TrimLeft(line, " \t")
		}
	}
	return strings.Join(result, "\n")
}

func (c *channelCollector) buildPackages() []PackageChannels {
	// Sort packages for deterministic output
	var pkgPaths []string
	for p := range c.ops {
		pkgPaths = append(pkgPaths, p)
	}
	sort.Strings(pkgPaths)

	var packages []PackageChannels
	for _, pkgPath := range pkgPaths {
		ops := c.ops[pkgPath]

		// Group by element type within this package
		typeOps := make(map[string][]opInfo)
		for _, op := range ops {
			typeOps[op.elemType] = append(typeOps[op.elemType], op)
		}

		// Sort element types
		var elemTypes []string
		for t := range typeOps {
			elemTypes = append(elemTypes, t)
		}
		sort.Strings(elemTypes)

		var groups []ChannelGroup
		for _, elemType := range elemTypes {
			group := ChannelGroup{
				ElementType: elemType,
			}

			for _, op := range typeOps[elemType] {
				switch op.kind {
				case "make":
					group.Makes = append(group.Makes, op.line)
				case "send":
					group.Sends = append(group.Sends, op.line)
				case "receive", "range":
					group.Receives = append(group.Receives, op.line)
				case "close":
					group.Closes = append(group.Closes, op.line)
				case "select_send":
					group.SelectSends = append(group.SelectSends, op.line)
				case "select_receive":
					group.SelectReceives = append(group.SelectReceives, op.line)
				}
			}

			groups = append(groups, group)
		}

		pc := PackageChannels{Package: pkgPath}
		if len(groups) == 0 {
			pc.Message = "no channel operations found in this package"
		} else {
			pc.Channels = groups
		}
		packages = append(packages, pc)
	}

	return packages
}

// renderChannelsMarkdown renders channel operations in a compact markdown format
func renderChannelsMarkdown(r ChannelsResponse) string {
	var sb strings.Builder

	sb.WriteString("# Channels: ")
	sb.WriteString(r.Query.Target)
	sb.WriteString("\n")

	for _, pkg := range r.Packages {
		sb.WriteString("\n## ")
		sb.WriteString(pkg.Package)
		sb.WriteString("\n")

		for _, group := range pkg.Channels {
			sb.WriteString("\n### chan ")
			sb.WriteString(group.ElementType)
			sb.WriteString("\n")

			if len(group.Makes) > 0 {
				sb.WriteString("**make**\n")
				for _, op := range group.Makes {
					sb.WriteString("- ")
					sb.WriteString(op)
					sb.WriteString("\n")
				}
			}

			if len(group.Sends) > 0 {
				sb.WriteString("**send**\n")
				for _, op := range group.Sends {
					sb.WriteString("- ")
					sb.WriteString(op)
					sb.WriteString("\n")
				}
			}

			if len(group.Receives) > 0 {
				sb.WriteString("**receive**\n")
				for _, op := range group.Receives {
					sb.WriteString("- ")
					sb.WriteString(op)
					sb.WriteString("\n")
				}
			}

			if len(group.Closes) > 0 {
				sb.WriteString("**close**\n")
				for _, op := range group.Closes {
					sb.WriteString("- ")
					sb.WriteString(op)
					sb.WriteString("\n")
				}
			}

			if len(group.SelectSends) > 0 {
				sb.WriteString("**select send**\n")
				for _, op := range group.SelectSends {
					sb.WriteString("- ")
					sb.WriteString(op)
					sb.WriteString("\n")
				}
			}

			if len(group.SelectReceives) > 0 {
				sb.WriteString("**select receive**\n")
				for _, op := range group.SelectReceives {
					sb.WriteString("- ")
					sb.WriteString(op)
					sb.WriteString("\n")
				}
			}
		}
	}

	// Summary
	sb.WriteString("\n---\n")
	sb.WriteString(fmt.Sprintf("**Summary:** %d ops across %d packages, %d channel types\n", r.Summary.TotalOps, r.Summary.Packages, r.Summary.Types))

	return sb.String()
}
