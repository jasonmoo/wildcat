# Wildcat

Code analysis built for AI agents. Go support via gopls.

## The Problem

AI coding assistants need to understand code relationships when refactoring. Language servers (gopls, rust-analyzer, pyright) exist, but they're designed for IDEs, not AI agents.

**LSP friction for AI:**

```bash
# AI wants: "Who calls config.Load?"
# LSP requires:
workspace/symbol("Load")                      # Step 1: Find position
textDocument/prepareCallHierarchy(position)   # Step 2: Prepare hierarchy
callHierarchy/incomingCalls(item)             # Step 3: Get direct callers
# Repeat for transitive callers...
# Read files to extract snippets...
```

- **Position-based API**: Requires `file:line:col`, not symbol names
- **Single-level only**: Call hierarchy shows direct callers, not full trees
- **No snippets**: Returns positions, not code context
- **Multiple round-trips**: Simple queries require many LSP calls
- **IDE-centric**: Designed for incremental, stateful sessions

## The Solution

Wildcat is purpose-built for AI agents. One query, actionable results.

```bash
wildcat symbol config.Load
```

```json
{
  "target": {
    "symbol": "config.Load",
    "file": "/home/user/proj/internal/config/config.go",
    "line": 15
  },
  "usage": {
    "callers": [...],
    "references": [...],
    "satisfies": [...]
  },
  "summary": {
    "total_locations": 12,
    "callers": 3,
    "references": 9
  }
}
```

**What makes this AI-ready:**
- `file`: Absolute path, pass directly to file read/edit tools
- `line`: Exact location for focused reads
- `snippet`: Understand context without reading the file
- Everything about a symbol in one query

## Commands

| Command | Description |
|---------|-------------|
| `wildcat search <query>` | Fuzzy search for symbols |
| `wildcat symbol <symbol>` | Complete analysis: callers, refs, interfaces |
| `wildcat package [path]` | Package profile with all symbols |
| `wildcat tree <symbol>` | Call graph traversal (up/down) |
| `wildcat channels [pkg]` | Channel operations and concurrency |
| `wildcat readme` | AI onboarding instructions |

## Usage

```bash
# Find symbols by name
wildcat search Config

# Everything about a symbol: callers, refs, interfaces
wildcat symbol config.Load

# Package profile: all symbols, imports, dependents
wildcat package ./internal/server

# Call tree: what does main call?
wildcat tree main.main --direction down --depth 3

# Call tree: what calls this function?
wildcat tree db.Query --direction up

# Channel operations in a package
wildcat channels ./internal/worker
```

## Symbol Formats

```bash
wildcat symbol config.Load           # package.Function
wildcat symbol Server.Start          # Type.Method
wildcat symbol internal/server.Start # path/package.Function
```

## Scope Filtering

Control which packages are included in results:

```bash
# search: default is all packages (including dependencies)
wildcat search Config
wildcat search --scope project Config         # only project packages
wildcat search --scope internal/lsp Config    # specific package

# symbol: default is target package only
wildcat symbol lsp.Client                     # callers in lsp package
wildcat symbol --scope project lsp.Client     # callers across project
wildcat symbol --scope cmd,internal/lsp Client # specific packages
```

| Command | Default scope | `--scope project` |
|---------|---------------|-------------------|
| search | all (including deps) | project packages only |
| symbol | target package | all project packages |

## Features

### Complete Symbol Analysis

One query returns everything about a symbol:

```bash
wildcat symbol lsp.Client
```

- Definition location
- Direct callers (who calls this)
- All references (type usage, not just calls)
- Implements (for interfaces): types that implement it
- Satisfies (for types): interfaces it implements

### Call Graph Traversal

See the full picture with the `tree` command:

```bash
wildcat tree main.main --depth 5 --direction down
# main.main → cmd.Execute → server.Start → handler.Serve → db.Query

wildcat tree db.Query --depth 3 --direction up
# What code paths lead to this function?
```

### Package Orientation

Get oriented in any package:

```bash
wildcat package ./internal/lsp
```

Returns all symbols in godoc order, plus imports and imported-by with locations.

### Smart Errors

Self-correcting suggestions:

```json
{
  "error": {
    "code": "symbol_not_found",
    "message": "Cannot resolve 'config.Laod'",
    "suggestions": ["config.Load", "config.LoadFromFile"]
  }
}
```

## Installation

```bash
go install github.com/jasonmoo/wildcat@latest
```

## Why "Wildcat"?

Fast, focused, gets the job done. No ceremony.

## License

MIT
