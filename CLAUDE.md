# Wildcat CLI

## What This Is

A code intelligence CLI for AI agents working on Go codebases. Built with Go's native analysis packages (`go/packages`, `go/types`, `go/ast`) to provide accurate, structured information about code.

**The user it serves:** AI agents that need to understand, navigate, and modify Go code.

**The problem it solves:** AIs need trustworthy answers to questions like "who calls this function?" and "what would break if I changed this?" Traditional tools (grep, go doc) give unstructured output. Wildcat gives structured, complete answers.

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

## Current State

The tool works and is useful. The main commands (`search`, `symbol`, `package`, `tree`, `deadcode`) provide real value.

**Current priority:** Error handling consistency. A recent audit found ~18 places where errors are silently discarded or analysis fails without indication. These are tracked as tickets. Check `bd list -s open -p 1` for the high-priority work.

**Key architectural tickets:**
- `wc-f06a`: Add diagnostics channel for non-fatal issues
- `wc-79d6`: Format functions should embed errors inline, not return them
- `wc-c79f`: Error handling philosophy documentation

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

## Bigger Picture

Wildcat is intended to be a reference implementation for a broader specification: **AIDE (AI Development Environment)**. The idea is that if we get the command surface area and output semantics right for Go, we can write a spec that other languages can implement.

This means command names and output structures should be thoughtfully designed - they're not just implementation details, they're potentially part of a standard.

See `docs/AIDE-SPEC-SEED.md` for the early thinking on this.

## Developing on Wildcat

### Orient yourself

```bash
bd list -s open              # See what needs work
bd ready                     # See unblocked tickets ready to claim
wildcat package internal/golang   # Understand a package
wildcat search "YourTopic"   # Find relevant symbols
```

### Use wildcat to develop wildcat

This is critical. Every time you need to understand code, find symbols, or explore - use wildcat instead of grep. This surfaces bugs and UX issues.

```bash
wildcat search "DeadCode"                           # find symbols
wildcat symbol golang.WalkReferences                # deep dive on a symbol
wildcat package internal/commands/symbol            # understand a package
wildcat tree commands.LoadWildcat                   # trace call graphs
```

A stable `wildcat` is in PATH. After changes, build with `go build -o wildcat .` and test with `./wildcat`.

### Working on tickets

```bash
bd show wc-XXXX              # Read the ticket
bd update wc-XXXX --status in_progress
# ... do the work ...
bd close wc-XXXX -r "Brief description of what was done"
git add . && git commit      # Include .beads/ in commit
```

### Code patterns to follow

**Adding a new analysis function** (in `internal/golang/`):
- Return errors, don't panic
- Use visitor pattern if walking AST/packages
- Consider what happens when type info is unavailable

**Adding error handling:**
- For fatal errors: return `error` or `*commands.ErrorResult` with suggestions
- For partial failures: emit diagnostic (once `wc-f06a` is implemented)
- For format failures: embed error inline in output string (per `wc-79d6`)

**Adding a new command:**
- Implement `Command[T]` interface
- Put in `internal/commands/<name>/`
- Wire up in `main.go`

## Build & Test

```bash
go build -o wildcat .        # Build
go test ./...                # Run tests
./wildcat <command>          # Test locally
```

Prefer writing `*_test.go` files over ad-hoc testing. For quick experiments, add print statements, build, run, then remove them.

---

## Tools

### bd - Issue Tracker

Issues are in `.beads/issues.jsonl`. Use `bd` to manage them.

```bash
bd ready                     # What's ready to work on
bd list -s open -p 1         # High priority open tickets
bd show wc-XXXX              # Ticket details
bd create "Title" -d "Description" -p 2
bd close wc-XXXX -r "Reason"
bd dep tree wc-XXXX          # See dependencies
```

**Workflow:** Create ticket → commit .beads/ → work → close ticket → commit code + .beads/ together

### Allowed Commands

These don't require approval:
- `bd`, `wildcat`, `go build`, `go test`, `go doc`, `jq`
- `git status/add/commit/log/diff/show/checkout/branch/...`
- `tree`, `find`, `ls`, `grep`

Avoid: `go run -`, `cat <<EOF | go run`, `-C <dir>` flags (cd instead)

Use built-in tools (Read, Write, Edit) instead of cat/echo/sed for file operations.
