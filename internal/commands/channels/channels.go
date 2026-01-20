package channels_cmd

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/spf13/cobra"
	"golang.org/x/tools/go/packages"
)

type ChannelsCommand struct {
	pkgPaths     []string
	includeTests bool
}

var _ commands.Command[*ChannelsCommand] = (*ChannelsCommand)(nil)

func WithPackages(paths []string) func(*ChannelsCommand) error {
	return func(c *ChannelsCommand) error {
		c.pkgPaths = paths
		return nil
	}
}

func WithIncludeTests(include bool) func(*ChannelsCommand) error {
	return func(c *ChannelsCommand) error {
		c.includeTests = include
		return nil
	}
}

func NewChannelsCommand() *ChannelsCommand {
	return &ChannelsCommand{}
}

func (c *ChannelsCommand) Cmd() *cobra.Command {
	var includeTests bool

	cmd := &cobra.Command{
		Use:   "channels [package] ...",
		Short: "Show channel operations in packages",
		Long: `Report all channel operations grouped by package and element type.

Shows makes, sends, receives, closes, and select cases for channels.
Useful for understanding concurrency patterns without precise pointer analysis.

Examples:
  wildcat channels                         # Current package
  wildcat channels ./internal/lsp          # Specific package
  wildcat channels internal/lsp internal/output  # Multiple packages`,
		RunE: func(cmd *cobra.Command, args []string) error {
			wc, err := commands.LoadWildcat(cmd.Context(), ".")
			if err != nil {
				return err
			}

			result, cmdErr := c.Execute(cmd.Context(), wc,
				WithPackages(args),
				WithIncludeTests(includeTests),
			)
			if cmdErr != nil {
				return fmt.Errorf("%s: %w", cmdErr.Code, cmdErr.Error)
			}

			// Check if JSON output requested via inherited flag
			if outputFlag := cmd.Flag("output"); outputFlag != nil && outputFlag.Changed && outputFlag.Value.String() == "json" {
				data, err := result.MarshalJSON()
				if err != nil {
					return err
				}
				os.Stdout.Write(data)
				os.Stdout.WriteString("\n")
				return nil
			}

			// Default to markdown
			md, err := result.MarshalMarkdown()
			if err != nil {
				return err
			}
			os.Stdout.Write(md)
			os.Stdout.WriteString("\n")
			return nil
		},
	}

	cmd.Flags().BoolVar(&includeTests, "include-tests", false, "Include test files")
	return cmd
}

func (c *ChannelsCommand) README() string {
	return "TODO"
}

func (c *ChannelsCommand) Execute(ctx context.Context, wc *commands.Wildcat, opts ...func(*ChannelsCommand) error) (commands.Result, *commands.Error) {
	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, commands.NewErrorf("opts_error", "failed to apply opt: %w", err)
		}
	}

	// Default to current directory
	if len(c.pkgPaths) == 0 {
		c.pkgPaths = []string{"."}
	}

	// Resolve package paths and find matching packages
	var targetPkgs []*packages.Package
	var resolvedPaths []string
	for _, arg := range c.pkgPaths {
		pi, err := wc.Project.ResolvePackageName(ctx, arg)
		if err != nil {
			return nil, commands.NewErrorf("package_not_found", "cannot resolve %q: %w", arg, err)
		}
		resolvedPaths = append(resolvedPaths, pi.PkgPath)

		pkg, err := wc.FindPackage(ctx, pi)
		if err != nil {
			return nil, commands.NewErrorf("find_package_error", "%w", err)
		}
		targetPkgs = append(targetPkgs, pkg)
	}

	queryTarget := strings.Join(resolvedPaths, ", ")

	// opInfo holds a single channel operation
	type opInfo struct {
		kind      string // make, send, receive, close, range, select_send, select_receive
		elemType  string
		operation string
		location  string
	}

	// Collect channel operations: package -> list of ops
	ops := make(map[string][]opInfo)
	seenFiles := make(map[string]bool)

	// Helper to add an operation
	addOp := func(fset *token.FileSet, pkgPath, filename string, kind string, node ast.Node, elemType string) {
		pos := fset.Position(node.Pos())
		operation, err := golang.FormatNode(node)
		if err != nil {
			operation = "<format error>"
		}
		base := filepath.Base(filename)
		location := fmt.Sprintf("%s:%d", base, pos.Line)
		ops[pkgPath] = append(ops[pkgPath], opInfo{
			kind:      kind,
			elemType:  elemType,
			operation: operation,
			location:  location,
		})
	}

	for _, pkg := range targetPkgs {
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

			if !c.includeTests && strings.HasSuffix(filename, "_test.go") {
				continue
			}

			// Ensure package appears in output even if no channel ops found
			if _, ok := ops[pkg.PkgPath]; !ok {
				ops[pkg.PkgPath] = nil
			}

			// Track nodes that are part of select cases so we don't double-count
			selectNodes := make(map[ast.Node]bool)

			// First pass: identify all select statement channel operations
			ast.Inspect(f, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectStmt); ok {
					for _, stmt := range sel.Body.List {
						if comm, ok := stmt.(*ast.CommClause); ok && comm.Comm != nil {
							switch node := comm.Comm.(type) {
							case *ast.SendStmt:
								selectNodes[node] = true
								if elemType := golang.ChannelElemType(pkg.TypesInfo, node.Chan); elemType != "" {
									addOp(pkg.Fset, pkg.PkgPath, filename, "select_send", node, elemType)
								}
							case *ast.ExprStmt:
								if recv, ok := node.X.(*ast.UnaryExpr); ok && recv.Op == token.ARROW {
									selectNodes[recv] = true
									if elemType := golang.ChannelElemType(pkg.TypesInfo, recv.X); elemType != "" {
										addOp(pkg.Fset, pkg.PkgPath, filename, "select_receive", recv, elemType)
									}
								}
							case *ast.AssignStmt:
								if len(node.Rhs) == 1 {
									if recv, ok := node.Rhs[0].(*ast.UnaryExpr); ok && recv.Op == token.ARROW {
										selectNodes[recv] = true
										if elemType := golang.ChannelElemType(pkg.TypesInfo, recv.X); elemType != "" {
											addOp(pkg.Fset, pkg.PkgPath, filename, "select_receive", recv, elemType)
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
						return true
					}
					if elemType := golang.ChannelElemType(pkg.TypesInfo, node.Chan); elemType != "" {
						addOp(pkg.Fset, pkg.PkgPath, filename, "send", node, elemType)
					}

				case *ast.UnaryExpr:
					if selectNodes[node] {
						return true
					}
					if node.Op == token.ARROW {
						if elemType := golang.ChannelElemType(pkg.TypesInfo, node.X); elemType != "" {
							addOp(pkg.Fset, pkg.PkgPath, filename, "receive", node, elemType)
						}
					}

				case *ast.CallExpr:
					if ident, ok := node.Fun.(*ast.Ident); ok {
						switch ident.Name {
						case "close":
							if len(node.Args) > 0 {
								if elemType := golang.ChannelElemType(pkg.TypesInfo, node.Args[0]); elemType != "" {
									addOp(pkg.Fset, pkg.PkgPath, filename, "close", node, elemType)
								}
							}
						case "make":
							if t := pkg.TypesInfo.TypeOf(node); t != nil {
								if ch, ok := t.Underlying().(*types.Chan); ok {
									elemType := ch.Elem().String()
									addOp(pkg.Fset, pkg.PkgPath, filename, "make", node, elemType)
								}
							}
						}
					}

				case *ast.RangeStmt:
					if elemType := golang.ChannelElemType(pkg.TypesInfo, node.X); elemType != "" {
						addOp(pkg.Fset, pkg.PkgPath, filename, "range", node, elemType)
					}
				}
				return true
			})
		}
	}

	// Build response: sort packages
	var pkgPaths []string
	for p := range ops {
		pkgPaths = append(pkgPaths, p)
	}
	sort.Strings(pkgPaths)

	var pkgChannels []PackageChannels
	for _, pkgPath := range pkgPaths {
		pkgOps := ops[pkgPath]

		// Group by element type
		typeOps := make(map[string][]opInfo)
		for _, op := range pkgOps {
			typeOps[op.elemType] = append(typeOps[op.elemType], op)
		}

		var elemTypes []string
		for t := range typeOps {
			elemTypes = append(elemTypes, t)
		}
		sort.Strings(elemTypes)

		var groups []ChannelGroup
		for _, elemType := range elemTypes {
			group := ChannelGroup{ElementType: elemType}
			for _, op := range typeOps[elemType] {
				channelOp := ChannelOp{
					Operation: op.operation,
					Location:  op.location,
				}
				switch op.kind {
				case "make":
					group.Makes = append(group.Makes, channelOp)
				case "send":
					group.Sends = append(group.Sends, channelOp)
				case "receive", "range":
					group.Receives = append(group.Receives, channelOp)
				case "close":
					group.Closes = append(group.Closes, channelOp)
				case "select_send":
					group.SelectSends = append(group.SelectSends, channelOp)
				case "select_receive":
					group.SelectReceives = append(group.SelectReceives, channelOp)
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
		pkgChannels = append(pkgChannels, pc)
	}

	// Build summary
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

	return &ChannelsCommandResponse{
		Query: output.QueryInfo{
			Command: "channels",
			Target:  queryTarget,
		},
		Operations: pkgChannels,
		Summary:    summary,
	}, nil
}
