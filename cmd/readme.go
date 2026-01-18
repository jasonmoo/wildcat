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

## Scope Filtering (default: project)
- search: --scope all (include deps), --scope internal/lsp
- symbol: --scope package (target only), --scope cmd,lsp
- exclude: --scope -internal/lsp (project minus package)

## Common Flags
- --scope SCOPE          Filter packages (project, packages, -pkg to exclude)
- --exclude-tests        Exclude test files
- --up N                 Tree: caller depth (default 2)
- --down N               Tree: callee depth (default 2)
- -o, --output FORMAT    json|yaml|markdown (tree defaults to markdown)
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
`+"`"+`wildcat search --scope all Config`+"`"+`

Fuzzy search across the workspace. Returns functions, types, methods, constants.
Default shows project packages. Use --scope all to include dependencies.

### symbol - Complete symbol analysis
`+"`"+`wildcat symbol lsp.Client`+"`"+`
`+"`"+`wildcat symbol --scope package lsp.Client`+"`"+`

Everything about a symbol in one query:
- Definition location
- Direct callers
- All references
- Implements (for interfaces): types that implement it
- Satisfies (for types): interfaces it implements

Default shows callers across project. Use --scope package for target package only.

### package - Package profile
`+"`"+`wildcat package ./internal/output`+"`"+`

Complete package map in godoc order: constants, variables, functions, types
with methods. Includes imports and imported-by with locations.

### tree - Call graph traversal
`+"`"+`wildcat tree main.main --down 3 --up 0`+"`"+`
`+"`"+`wildcat tree db.Query --up 3`+"`"+`

Build bidirectional call trees:
- --up N: show N levels of callers (what calls this)
- --down N: show N levels of callees (what this calls)
- Default: 2 levels each direction

Output defaults to markdown (dense, ~50% smaller). Use -o json for field extraction.

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
| --scope | search, symbol, tree | Filter packages: 'project' or comma-separated |
| --exclude-tests | symbol, tree, channels | Exclude test files |
| --up N | tree | Caller depth (default 2) |
| --down N | tree | Callee depth (default 2) |
| -o FORMAT | all | json, yaml, markdown (tree defaults to markdown) |

## Scope Filtering

| Command | Default | Other scopes |
|---------|---------|--------------|
| search | project | all (include deps), comma-separated |
| symbol | project | package (target only), comma-separated |

## Workflow Patterns

### Understanding a Symbol
`+"`"+`wildcat symbol Config`+"`"+`

Single query returns callers, references, and interface relationships.

### Exploring Call Flow
`+"`"+`bash
# What does main call? (callees only)
wildcat tree main.main --down 3 --up 0

# What calls this function? (callers only)
wildcat tree handleRequest --up 3 --down 0

# Both directions (default)
wildcat tree server.Handle
`+"`"+`

### Package Orientation
`+"`"+`wildcat package ./internal/lsp`+"`"+`

See all symbols, what it imports, and what depends on it.

## Output Formats

**Markdown** (tree default): Dense, AI-readable, ~50% smaller than JSON.
Preferred for understanding code structure and relationships.

**JSON** (-o json): Structured output for programmatic field extraction.
Use when you need to parse specific values from results.

All commands return structured data with:
- query: What was requested
- target/package: The resolved symbol or package
- results/usage: The data (callers, refs, etc.)
- summary: Counts

Errors include suggestions for typos.
`)
}
