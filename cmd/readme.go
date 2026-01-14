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
- wildcat callers <symbol>     Find all callers of a function
- wildcat callees <symbol>     Find functions called by a function
- wildcat refs <symbol>        Find all references to a symbol
- wildcat tree <symbol>        Build call hierarchy tree
- wildcat impact <symbol>      Analyze change impact
- wildcat implements <iface>   Find types implementing interface
- wildcat satisfies <type>     Find interfaces a type satisfies
- wildcat deps [package]       Show package dependencies
- wildcat package [path]       Show package profile with symbols
- wildcat symbols <query>      Search for symbols (fuzzy match)

## Symbol Formats
- Function               pkg.Function, main.main
- Method                 Type.Method, Server.Start
- Full path              path/to/pkg.Function

## Common Flags
- --compact              Omit code snippets
- --exclude-tests        Exclude test files
- --depth N              Limit traversal depth
- -o, --output FORMAT    json|yaml|markdown|dot
`)
}

func printFullReadme() {
	fmt.Fprint(os.Stdout, `# Wildcat - Static Analysis for AI Agents

Wildcat provides symbol-based code analysis optimized for AI tool integration.
It uses LSP (Language Server Protocol) for language-agnostic support.

## When to Use Wildcat

Use wildcat for **semantic code understanding**:
- Finding all callers of a function before refactoring
- Understanding what a function calls (callees)
- Tracing impact of changing a type or interface
- Finding interface implementations
- Exploring package dependencies

## When to Use Alternatives

- **grep/ripgrep**: Text patterns, non-code files, quick string search
- **gopls directly**: IDE features, diagnostics, formatting, rename
- **go list**: Module info, build flags, dependency versions

## Commands

### callers - Find who calls a function
`+"`"+`wildcat callers config.Load`+"`"+`

Returns all functions that call the target. Useful before changing a function's
signature or behavior.

### callees - Find what a function calls
`+"`"+`wildcat callees main.main --depth 2`+"`"+`

Returns all functions called by the target. Useful for understanding code flow.

### refs - Find all references
`+"`"+`wildcat refs Config`+"`"+`

Returns all references to a symbol including type usage, not just calls.

### tree - Build call hierarchy
`+"`"+`wildcat tree Server.Start --direction up --depth 3`+"`"+`

Builds a visual call tree. Use --direction up for callers, down for callees.

### impact - Change impact analysis
`+"`"+`wildcat impact lsp.Client`+"`"+`

Comprehensive analysis of what would break if you change a symbol. Combines
callers, references, and interface implementations.

### implements - Find implementations
`+"`"+`wildcat implements io.Reader`+"`"+`

Find all types that implement an interface.

### satisfies - Find satisfied interfaces
`+"`"+`wildcat satisfies JSONFormatter`+"`"+`

Find all interfaces that a type satisfies.

### deps - Package dependencies
`+"`"+`wildcat deps ./internal/lsp`+"`"+`

Show what a package imports and what packages import it (both directions in one call).

### package - Package profile
`+"`"+`wildcat package ./internal/output`+"`"+`

Show a complete package profile with all symbols in godoc order (constants,
variables, functions, then types with their methods).

### symbols - Search for symbols
`+"`"+`wildcat symbols Config`+"`"+`

Fuzzy search for symbols across the workspace. Results are ranked by relevance.

## Symbol Formats

| Format | Example | Description |
|--------|---------|-------------|
| Function | main | Function in current context |
| Package.Function | config.Load | Package-qualified function |
| Type.Method | Server.Start | Method on type (value or pointer receiver) |
| Full path | github.com/user/pkg.Func | Fully qualified |

## Output Formats

- --output json: Structured JSON (default)
- --output yaml: YAML format
- --output markdown: Markdown tables
- --output dot: Graphviz DOT for visualization

## Common Flags

| Flag | Description |
|------|-------------|
| --compact | Omit code snippets for smaller output |
| --exclude-tests | Exclude test files from results |
| --depth N | Limit traversal depth |
| --context N | Lines of context in snippets (default 3) |
| -l, --language | Force language (go, python, typescript, rust, c) |

## Workflow Patterns

### Before Refactoring a Function
`+"`"+`bash
# Check what calls this function
wildcat callers old.Function

# Check what it calls (to understand dependencies)
wildcat callees old.Function
`+"`"+`

### Understanding New Code
`+"`"+`bash
# Start from main and go down
wildcat tree main.main --direction down --depth 3

# Find the entry points that reach a function
wildcat tree internal.Function --direction up
`+"`"+`

### Before Changing an Interface
`+"`"+`bash
# Find all implementations
wildcat implements MyInterface

# Full impact analysis
wildcat impact MyInterface
`+"`"+`

## JSON Output Structure

All commands return consistent JSON with:
- query: What was requested
- target: Resolved symbol with file and line
- results: Array of matches with snippets
- summary: Count, packages, test file count

Error responses include:
- error.code: Machine-readable error code
- error.message: Human-readable message
- error.suggestions: Similar symbols if not found
`)
}
