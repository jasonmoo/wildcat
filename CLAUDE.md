# Wildcat CLI

## What This Is

A code intelligence CLI for AI agents working on Go codebases. Built with Go's native analysis packages (`go/packages`, `go/types`, `go/ast`) to provide accurate, structured information about code.

**The user it serves:** AI agents that need to understand, navigate, and modify Go code.

**The problem it solves:** AIs need trustworthy answers to questions like "who calls this function?" and "what would break if I changed this?" Traditional tools (grep, go doc) give unstructured output. Wildcat gives structured, complete answers.

## Core Goals

1. **Support AIs in developing Go code**: Every feature should ask: "Does this help an AI make better decisions about code?"

2. **Be a source of truth**: Accurate and complete output. AIs rely on this to reason - incorrect or missing data leads to broken code.

## North Star

**Be a source of truth that AIs can trust.**

Every piece of output should be accurate and complete. When wildcat says "these are all the callers," it means ALL the callers. When it can't provide complete information, it says so explicitly.

This matters because AIs make decisions based on wildcat's output. Incomplete information presented as complete leads to broken code.

## The Cardinal Rule

**Never silently fail.**

- Don't discard errors with `_`
- Don't skip items without explanation
- Don't return partial results as if they were complete
- If something fails, include it in output with an error explaining why

An AI that knows it's missing information can adapt. An AI that doesn't know is operating blind.

## Error Handling

Two categories, one principle: **the AI must always know what happened.**

**System errors** — things the AI cannot fix:
- Filesystem failures
- Network errors
- Invalid configuration
- Missing dependencies

These stop execution. Return the error idiomatically (`return err`). There's no point continuing if the environment is broken.

**Operational issues** — things that went wrong during analysis:
- A signature couldn't be formatted
- Type info was unavailable for a symbol
- A package had parse errors but partially loaded

These don't stop execution. Report them via:
- **Diagnostics**: for issues affecting result completeness ("3 packages had type errors")
- **Inline tokens**: for issues at a specific point (`func Foo(...) <format error: nil receiver>`)

The AI can still use partial results and knows exactly what's degraded.

**The test:** Can the AI do something about this? If yes (or if the info is still useful), continue and report. If no, stop and error.

## Design Principles

- **Composability first**: Use interfaces to decouple components. Design for change.
- **Simple over clever**: Clear, readable code beats complex abstractions.
- **Small, focused packages**: Each package should do one thing well.
- **No global state**: Pass dependencies explicitly.
- **Wrap errors with context**: `fmt.Errorf("context: %w", err)`

## Architecture

```
main.go                    # CLI entry point, cobra root command
internal/
  commands/                # CLI commands (search, symbol, package, tree, deadcode, readme)
    commands.go            # Command[T] interface, ErrorResult, Result interface
    wildcat.go             # Wildcat struct (Project + Index), shared helpers
    scope.go               # ScopeFilter for package filtering
    search/                # search command
    symbol/                # symbol command
    package/               # package command
    tree/                  # tree command
    deadcode/              # deadcode command
    readme/                # readme command
  golang/                  # Pure Go analysis (no CLI concerns)
    pkgpath.go             # Project, Package, loading
    search.go              # SymbolIndex, fuzzy search, regex search
    refs.go                # Reference walking
    calls.go               # Call graph walking
    interfaces.go          # Interface satisfaction checking
    deadcode.go            # RTA-based reachability analysis
    format.go              # AST formatting helpers
    channels.go            # Channel operation detection
    embed.go               # go:embed directive parsing
  output/                  # Output formatting
    types.go               # Shared output types
    snippet.go             # Code snippet extraction
```

**Key types:**
- `commands.Wildcat`: Holds loaded project, stdlib, and symbol index
- `golang.Project`: Module info + all packages
- `golang.Package`: Single package with AST and type info
- `golang.Symbol`: A named entity (func, type, var, const, method)
- `golang.SymbolIndex`: Searchable index of all symbols

**Key patterns:**
- Commands implement `Command[T]` interface with `Execute()` and `Cmd()`
- Analysis functions are in `internal/golang/`, CLI wiring in `internal/commands/`
- Visitor patterns for walking (`RefVisitor`, `CallVisitor`, `ChannelOpVisitor`)
- `ScopeFilter` for include/exclude package patterns

## Developing on Wildcat

### Orient yourself

```bash
bd ready                     # See unblocked tickets ready to claim
wildcat package internal/golang   # Understand a package
wildcat search "YourTopic"   # Find relevant symbols
```

### Use wildcat to develop wildcat

This is critical. Every time you need to understand code, find symbols, or explore - use wildcat instead of grep. This surfaces bugs and UX issues.

```bash
wildcat search DeadCode                             # find symbols
wildcat symbol golang.WalkReferences                # deep dive on a symbol
wildcat package internal/commands/symbol            # understand a package
wildcat tree commands.LoadWildcat                   # trace call graphs
```

A stable `wildcat` is in PATH. After changes, build with `go build -o wildcat .` and test with `./wildcat`.

### bd - Issue Tracker

Issues are in `.beads/issues.jsonl`. Use `bd` to manage them.

**Note for future Claudes:** bd is your memory across sessions. Track work, insights, and discoveries here. Your context won't persist, but `.beads/` will.

#### Working on tickets

The workflow is atomic: close the ticket and commit the code together.

```bash
bd show wc-XXXX              # Read the ticket
bd update wc-XXXX --status in_progress
# ... do the work ...
bd close wc-XXXX -r "Brief description of what was done"
git add . && git commit      # Code + .beads/ in same commit
```

**Why this order:** The ticket closure and code change belong together for traceability. Close the ticket first (updates `.beads/`), then commit everything atomically. Don't commit the ticket closure separately from the code it describes.

### Code patterns to follow

## Build & Test

```bash
go build -o wildcat .        # Build
go test ./...                # Run tests
./wildcat <command>          # Test locally
```

**Adhoc testing approach:**
- Prefer writing `*_test.go` files - they integrate with package context
- Table-driven tests preferred
- Test interfaces, not implementations
- **For quick experiments**: add print statements, build, run, remove them
- NEVER use `go run -` or `cat <<EOF | go run` - they don't work and require approval

**Why test files over ad-hoc:** Shell patterns like `cat > /tmp/test.go` require approval 
every time and fail due to import path issues. A proper test file in the package directory 
just works.

---

## Tools

Use built-in tools (Read, Write, Edit) instead of cat/echo/sed for file operations.
