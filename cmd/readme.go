package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var readmeCmd = &cobra.Command{
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
	Run: runReadme,
}

var (
	readmeCompact bool
)

func init() {
	rootCmd.AddCommand(readmeCmd)

	readmeCmd.Flags().BoolVar(&readmeCompact, "compact", false, "Quick reference only")
}

func runReadme(cmd *cobra.Command, args []string) {
	if readmeCompact {
		printCompactReadme()
	} else {
		printFullReadme()
	}
}

func printCompactReadme() {
	fmt.Fprint(os.Stdout, `# Wildcat Quick Reference

## Commands
- wildcat search <query>       Fuzzy search for symbols
- wildcat symbol <symbol>      Complete symbol analysis (callers, refs, interfaces)
- wildcat package [path]       Package profile with all symbols
- wildcat tree <symbol>        Call graph traversal (up/down)
- wildcat channels [package]   Channel operations and concurrency

## Symbol Formats
- Function               pkg.Function, main.main
- Method                 Type.Method, Server.Start
- Full path              path/to/pkg.Function

## Scope Filtering
- search (default: all)        --scope project, --scope internal/lsp
- symbol (default: target pkg) --scope project, --scope cmd,lsp

## Common Flags
- --scope SCOPE          Filter packages (project, or comma-separated)
- --exclude-tests        Exclude test files
- --exclude-stdlib       Exclude standard library
- --depth N              Tree traversal depth
- -o, --output FORMAT    json|yaml|markdown
`)
}

func printFullReadme() {
	fmt.Fprint(os.Stdout, `# Wildcat - Static Analysis for AI Agents

Wildcat provides symbol-based code analysis optimized for AI tool integration.
Uses gopls (Go Language Server) for semantic understanding of Go code.

## When to Use Wildcat

- **search**: Find symbols by name (fuzzy matching)
- **symbol**: Understand a symbol's full footprint (callers, refs, interfaces)
- **package**: Get oriented in a package (all symbols, imports, dependents)
- **tree**: Explore call graphs (who calls what, what calls who)
- **channels**: Understand concurrency patterns

## When to Use Alternatives

- **grep/ripgrep**: Text patterns, non-code files
- **go doc**: Documentation lookup
- **go list**: Module info, build configuration

## Commands

### search - Find symbols
`+"`"+`wildcat search Config`+"`"+`
`+"`"+`wildcat search --scope project Config`+"`"+`

Fuzzy search across the workspace. Returns functions, types, methods, constants.
Default shows all packages (including dependencies). Use --scope to filter.

### symbol - Complete symbol analysis
`+"`"+`wildcat symbol lsp.Client`+"`"+`
`+"`"+`wildcat symbol --scope project lsp.Client`+"`"+`

Everything about a symbol in one query:
- Definition location
- Direct callers
- All references
- Implements (for interfaces): types that implement it
- Satisfies (for types): interfaces it implements

Default shows callers in target package only. Use --scope project for all project packages.

### package - Package profile
`+"`"+`wildcat package ./internal/output`+"`"+`

Complete package map in godoc order: constants, variables, functions, types
with methods. Includes imports and imported-by with locations.

### tree - Call graph traversal
`+"`"+`wildcat tree main.main --direction down --depth 3`+"`"+`
`+"`"+`wildcat tree db.Query --direction up --depth 2`+"`"+`

Build call trees in either direction:
- down: what does this function call?
- up: what calls this function?

### channels - Concurrency analysis
`+"`"+`wildcat channels ./internal/lsp`+"`"+`

Channel operations grouped by type: makes, sends, receives, closes, selects.

## Symbol Formats

| Format | Example |
|--------|---------|
| Function | config.Load |
| Method | Server.Start |
| Type | lsp.Client |
| Full path | github.com/user/pkg.Func |

## Common Flags

| Flag | Commands | Description |
|------|----------|-------------|
| --scope | search, symbol | Filter packages: 'project' or comma-separated |
| --exclude-tests | symbol, tree, channels | Exclude test files |
| --exclude-stdlib | package, tree | Exclude standard library |
| --depth N | tree | Traversal depth (default 3) |
| --direction | tree | up or down (default down) |
| -o json/yaml/markdown | all | Output format |

## Scope Filtering

| Command | Default | --scope project |
|---------|---------|-----------------|
| search | all (including deps) | project packages only |
| symbol | target package only | all project packages |

## Workflow Patterns

### Understanding a Symbol
`+"`"+`wildcat symbol Config`+"`"+`

Single query returns callers, references, and interface relationships.

### Exploring Call Flow
`+"`"+`bash
# What does main call?
wildcat tree main.main --direction down

# What leads to this function?
wildcat tree handleRequest --direction up
`+"`"+`

### Package Orientation
`+"`"+`wildcat package ./internal/lsp`+"`"+`

See all symbols, what it imports, and what depends on it.

## JSON Output

All commands return structured JSON with:
- query: What was requested
- target/package: The resolved symbol or package
- results/usage: The data (callers, refs, etc.)
- summary: Counts

Errors include suggestions for typos.
`)
}
