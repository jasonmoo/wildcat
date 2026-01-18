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

# Call tree: what does main call? (dense markdown by default)
wildcat tree main.main --down 3 --up 0

# Call tree: what calls this function?
wildcat tree db.Query --up 3

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
# search: default is project packages
wildcat search Config                         # project packages (default)
wildcat search --scope all Config             # include external dependencies
wildcat search --scope internal/lsp Config    # specific package

# symbol: default is project packages
wildcat symbol lsp.Client                     # callers across project (default)
wildcat symbol --scope package lsp.Client     # callers in target package only
wildcat symbol --scope cmd,internal/lsp Client # specific packages
```

| Command | Default | Other scopes |
|---------|---------|--------------|
| search | project | `all`, comma-separated packages |
| symbol | project | `package` (target only), comma-separated |

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
# Callees: what does main.main call? (5 levels down)
wildcat tree main.main --down 5 --up 0

# Callers: what calls db.Query? (3 levels up)
wildcat tree db.Query --up 3 --down 0

# Both directions (default: 2 up, 2 down)
wildcat tree server.Handle
```

Output defaults to markdown (dense, ~50% smaller). Use `-o json` for field extraction.

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
