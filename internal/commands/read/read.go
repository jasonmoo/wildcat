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
  <path>    Semantic path or pattern to read (multiple allowed)

Pattern Matching:
  Paths can include wildcards:
    *   matches within a segment (stops at . / [ ])
    **  matches across segments

Examples:
  wildcat read golang.WalkReferences           # whole function
  wildcat read golang.WalkReferences/body      # function body only
  wildcat read golang.WalkReferences/params[visitor]  # a parameter
  wildcat read golang.Symbol                   # a type definition
  wildcat read golang.Symbol/fields[Name]      # a specific field
  wildcat read ls_cmd.PathEntry/doc            # doc comment
  wildcat read golang.Symbol golang.Package    # multiple paths

  # Pattern examples
  wildcat read 'spath.Path/fields[*]'          # all fields of a type
  wildcat read 'spath.Path.*'                  # type + all methods
  wildcat read '**.Execute'                    # all Execute methods

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
		targetSections := c.readTarget(ctx, wc, target)
		sections = append(sections, targetSections...)
	}

	return &ReadResponse{
		Sections: sections,
	}, nil
}

// readTarget reads a single target and returns a section.
func (c *ReadCommand) readTarget(ctx context.Context, wc *commands.Wildcat, target string) []ReadSection {
	// Check if this is a glob pattern
	if commands.IsSpathPattern(target) {
		return c.readGlob(ctx, wc, target)
	}

	// Try to resolve as a package first
	if pi, err := wc.Project.ResolvePackageName(ctx, target); err == nil {
		if pkg, err := wc.Package(pi); err == nil {
			return []ReadSection{c.readPackage(pkg)}
		}
	}

	// Try to resolve as a semantic path (symbol or deeper)
	res, err := wc.ResolveSpath(ctx, target)
	if err != nil {
		section := ReadSection{
			Path:  target,
			Error: "not found",
		}
		for _, s := range wc.Suggestions(target, &golang.SearchOptions{Limit: 5}) {
			section.Suggestions = append(section.Suggestions, s.Symbol)
		}
		return []ReadSection{section}
	}

	// Get FileSet for rendering
	var fset *token.FileSet
	if res.Package != nil && res.Package.Package != nil {
		fset = res.Package.Package.Fset
	}

	// Render the AST node
	source, err := golang.RenderSource(res.Node, fset)
	if err != nil {
		return []ReadSection{{
			Path:  target,
			Error: fmt.Sprintf("render error: %v", err),
		}}
	}

	return []ReadSection{{
		Path:     target,
		Resolved: res.FullPath(),
		Source:   source,
	}}
}

// readGlob matches a pattern and reads all matching paths.
func (c *ReadCommand) readGlob(ctx context.Context, wc *commands.Wildcat, pattern string) []ReadSection {
	result, err := wc.MatchSpathGlob(pattern, 0) // no limit
	if err != nil {
		return []ReadSection{{
			Path:  pattern,
			Error: fmt.Sprintf("pattern error: %v", err),
		}}
	}

	if len(result.Matches) == 0 {
		return []ReadSection{{
			Path:  pattern,
			Error: fmt.Sprintf("no paths match pattern %q", pattern),
		}}
	}

	var sections []ReadSection
	for _, match := range result.Matches {
		// Skip packages - only read actual code paths
		if match.Kind == "package" {
			continue
		}
		// Read each matched path (non-pattern, so won't recurse)
		s := c.readTarget(ctx, wc, match.Path)
		sections = append(sections, s...)
	}

	return sections
}

// readPackage renders a package as unified source code.
func (c *ReadCommand) readPackage(pkg *golang.Package) ReadSection {
	source, err := golang.RenderPackageSource(pkg)
	if err != nil {
		return ReadSection{
			Path:  pkg.Identifier.PkgShortPath,
			Error: fmt.Sprintf("render error: %v", err),
		}
	}

	return ReadSection{
		Path:     pkg.Identifier.PkgShortPath,
		Resolved: pkg.Identifier.PkgPath,
		Source:   source,
	}
}
