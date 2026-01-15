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
wildcat callers config.Load
```

```json
{
  "target": {
    "symbol": "config.Load",
    "file": "/home/user/proj/internal/config/config.go",
    "line": 15
  },
  "results": [
    {
      "symbol": "main.main",
      "file": "/home/user/proj/main.go",
      "line": 23,
      "snippet": "func main() {\n\tcfg, err := config.Load(configPath)\n\tif err != nil {",
      "snippet_start": 21,
      "snippet_end": 25,
      "call_expr": "config.Load(configPath)"
    }
  ]
}
```

**What makes this AI-ready:**
- `file`: Absolute path, pass directly to file read/edit tools
- `line`: Exact location for focused reads
- `snippet`: Understand context without reading the file
- `call_expr`: Use directly as match text for edits

## Features

### Symbol-Based Queries

Query by symbol name, not file positions:

```bash
wildcat callers config.Load           # package.Function
wildcat callers Server.Start          # Type.Method
wildcat callers internal/server.Start # path/package.Function
```

### Transitive Call Graphs

See the full picture, not just direct relationships:

```bash
wildcat tree main.main --depth 5 --direction down
# main.main → cmd.Execute → server.Start → handler.Serve → db.Query

wildcat tree db.Query --depth 3 --direction up
# What code paths lead to this function?
```

### Actionable Output

Every result includes what you need to act:

| Field | Purpose |
|-------|---------|
| `file` | Absolute path for read/edit tools |
| `line` | Line of the reference/call site |
| `snippet` | Code context without file read |
| `snippet_start`, `snippet_end` | Line range of the snippet |
| `call_expr` | Exact call expression text (callers only) |
| `in_test` | Filter test vs production code |

### Filtering

Focus on what matters:

```bash
wildcat callers Load --exclude-tests    # Skip test files
wildcat callers Load --package ./...    # Current module only
wildcat callers Load --limit 20         # Cap results
```

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

### Language Support

Currently supports Go via gopls. The LSP-based architecture enables future expansion to other languages.

## Commands

| Command | Description |
|---------|-------------|
| `wildcat callers <symbol>` | Who calls this function? |
| `wildcat callees <symbol>` | What does this function call? |
| `wildcat tree <symbol>` | Full call tree with depth control |
| `wildcat refs <symbol>` | All references to symbol |
| `wildcat impact <symbol>` | What breaks if I change this? |
| `wildcat implements <iface>` | What implements this interface? |
| `wildcat satisfies <type>` | What interfaces does this type satisfy? |
| `wildcat deps [package]` | Package dependency graph (both directions) |
| `wildcat package [path]` | Package profile with all symbols |
| `wildcat symbols <query>` | Fuzzy search for symbols |
| `wildcat readme` | AI onboarding instructions |

## Installation

```bash
go install github.com/jasonmoo/wildcat@latest
```

## Usage

```bash
# Find all callers of a function
wildcat callers config.Load

# Show call tree from main, 4 levels deep
wildcat tree main.main --depth 4

# What breaks if I change this type?
wildcat impact config.Config

# Find what implements an interface
wildcat implements io.Reader

# Find what interfaces a type satisfies
wildcat satisfies MyServer

# Package dependencies (shows imports and imported_by)
wildcat deps ./internal/server

# Search for symbols by name
wildcat symbols Config
```

## Why "Wildcat"?

Fast, focused, gets the job done. No ceremony.

## License

MIT
