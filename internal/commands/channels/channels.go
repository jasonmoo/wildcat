package channels_cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/spf13/cobra"
)

type ChannelsCommand struct {
	pkgPaths []string
}

var _ commands.Command[*ChannelsCommand] = (*ChannelsCommand)(nil)

func WithPackages(paths []string) func(*ChannelsCommand) error {
	return func(c *ChannelsCommand) error {
		c.pkgPaths = paths
		return nil
	}
}

func NewChannelsCommand() *ChannelsCommand {
	return &ChannelsCommand{}
}

func (c *ChannelsCommand) Cmd() *cobra.Command {
	return &cobra.Command{
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

			result, err := c.Execute(cmd.Context(), wc,
				WithPackages(args),
			)
			if err != nil {
				return err
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
}

func (c *ChannelsCommand) README() string {
	return "TODO"
}

func (c *ChannelsCommand) Execute(ctx context.Context, wc *commands.Wildcat, opts ...func(*ChannelsCommand) error) (commands.Result, error) {
	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, fmt.Errorf("interal_error: failed to apply opt: %w", err)
		}
	}

	// Default to current directory
	if len(c.pkgPaths) == 0 {
		c.pkgPaths = []string{"."}
	}

	// Resolve package paths and find matching packages
	var targetPkgs []*golang.Package
	var resolvedPaths []string
	for _, arg := range c.pkgPaths {
		pi, err := wc.Project.ResolvePackageName(ctx, arg)
		if err != nil {
			return commands.NewErrorResultf("package_not_found", "cannot resolve %q: %w", arg, err), nil
		}
		resolvedPaths = append(resolvedPaths, pi.PkgPath)
		targetPkgs = append(targetPkgs, wc.Package(pi))
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

	// Ensure all target packages appear in output even if no channel ops found
	for _, pkg := range targetPkgs {
		if !strings.HasSuffix(pkg.Identifier.PkgPath, ".test") {
			ops[pkg.Identifier.PkgPath] = nil
		}
	}

	// Walk channel operations using the visitor API
	golang.WalkChannelOps(targetPkgs, func(op golang.ChannelOp) bool {
		if strings.HasSuffix(op.Package.Identifier.PkgPath, ".test") {
			return true
		}

		operation, err := golang.FormatNode(op.Node)
		if err != nil {
			operation = "<format error>"
		}
		base := filepath.Base(op.File)
		location := fmt.Sprintf("%s:%d", base, op.Line)

		ops[op.Package.Identifier.PkgPath] = append(ops[op.Package.Identifier.PkgPath], opInfo{
			kind:      string(op.Kind),
			elemType:  op.ElemType,
			operation: operation,
			location:  location,
		})
		return true
	})

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
