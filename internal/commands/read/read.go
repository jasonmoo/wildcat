package read_cmd

import (
	"context"
	"fmt"
	"go/token"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/spf13/cobra"
)

type ReadCommand struct {
	targets []string
}

var _ commands.Command[*ReadCommand] = (*ReadCommand)(nil)

func WithTargets(targets []string) func(*ReadCommand) error {
	return func(c *ReadCommand) error {
		c.targets = targets
		return nil
	}
}

func NewReadCommand() *ReadCommand {
	return &ReadCommand{}
}

func (c *ReadCommand) Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "read <path> [path...]",
		Short: "Read source code at semantic paths",
		Long: `Read and display source code at semantic paths.

The read command resolves semantic paths to AST nodes and renders
the source code. This provides symbolic access to code without needing
to know file paths or line numbers.

Arguments:
  <path>    Semantic path to read (multiple allowed)

Examples:
  wildcat read golang.WalkReferences           # whole function
  wildcat read golang.WalkReferences/body      # function body only
  wildcat read golang.WalkReferences/params[visitor]  # a parameter
  wildcat read golang.Symbol                   # a type definition
  wildcat read golang.Symbol/fields[Name]      # a specific field
  wildcat read ls_cmd.PathEntry/doc            # doc comment
  wildcat read golang.Symbol golang.Package    # multiple paths

Output is the rendered AST, consistently formatted regardless of
original source formatting.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.RunCommand(cmd, c, WithTargets(args))
		},
	}

	return cmd
}

func (c *ReadCommand) README() string {
	return "TODO"
}

func (c *ReadCommand) Execute(ctx context.Context, wc *commands.Wildcat, opts ...func(*ReadCommand) error) (commands.Result, error) {
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	if len(c.targets) == 0 {
		return commands.NewErrorResultf("invalid_target", "at least one semantic path is required"), nil
	}

	var sections []ReadSection

	for _, target := range c.targets {
		section := c.readTarget(ctx, wc, target)
		sections = append(sections, section)
	}

	return &ReadResponse{
		Sections: sections,
	}, nil
}

// readTarget reads a single target and returns a section.
func (c *ReadCommand) readTarget(ctx context.Context, wc *commands.Wildcat, target string) ReadSection {
	// Resolve the path
	res, err := wc.ResolveSpath(ctx, target)
	if err != nil {
		return ReadSection{
			Path:  target,
			Error: fmt.Sprintf("cannot resolve: %v", err),
		}
	}

	// Get FileSet for rendering
	var fset *token.FileSet
	if res.Package != nil && res.Package.Package != nil {
		fset = res.Package.Package.Fset
	}

	// Render the AST node
	source, err := golang.RenderSource(res.Node, fset)
	if err != nil {
		return ReadSection{
			Path:  target,
			Error: fmt.Sprintf("render error: %v", err),
		}
	}

	return ReadSection{
		Path:     target,
		Resolved: res.FullPath(),
		Source:   source,
	}
}
