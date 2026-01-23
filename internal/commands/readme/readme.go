package readme_cmd

import (
	"context"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/spf13/cobra"
)

type ReadmeCommand struct {
	compact bool
}

var _ commands.Command[*ReadmeCommand] = (*ReadmeCommand)(nil)

func WithCompact(compact bool) func(*ReadmeCommand) error {
	return func(c *ReadmeCommand) error {
		c.compact = compact
		return nil
	}
}

func NewReadmeCommand() *ReadmeCommand {
	return &ReadmeCommand{}
}

func (c *ReadmeCommand) Cmd() *cobra.Command {
	var compact bool

	cmd := &cobra.Command{
		Use:   "readme",
		Short: "Output AI onboarding instructions",
		Long: `Output comprehensive usage guidance for AI agents.

This generates instructions suitable for including in:
  - CLAUDE.md files
  - System prompts
  - MCP server context

Examples:
  wildcat readme > CLAUDE.md
  wildcat readme --compact`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.RunCommand(cmd, c, WithCompact(compact))
		},
	}

	cmd.Flags().BoolVar(&compact, "compact", false, "Quick reference only")

	return cmd
}

func (c *ReadmeCommand) README() string {
	return "TODO"
}

func (c *ReadmeCommand) Execute(ctx context.Context, wc *commands.Wildcat, opts ...func(*ReadmeCommand) error) (commands.Result, error) {
	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, err
		}
	}

	return &ReadmeCommandResponse{
		Compact: c.compact,
	}, nil
}
