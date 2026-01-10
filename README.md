# Wildcat

Go static analysis built for AI agents.

## The Problem

AI coding assistants need to understand code relationships when refactoring Go. The standard tool is `gopls`, but it's designed for IDEs, not AI agents.

**gopls friction for AI:**

```bash
# AI wants: "Who calls config.Load?"
# gopls requires:
gopls workspace_symbol "Load"              # Step 1: Find position
# ... parse text output ...
gopls references config/config.go:15:6     # Step 2: Query by position
# ... parse more text output ...
```

- **Position-based API**: Requires `file:line:col`, not symbol names
- **Text output**: Most commands lack JSON, require parsing
- **Single-level only**: `call_hierarchy` shows direct callers, not full trees
- **No batch queries**: One symbol at a time
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
    "line": 15,
    "signature": "func Load(path string) (*Config, error)"
  },
  "results": [
    {
      "symbol": "main.main",
      "file": "/home/user/proj/main.go",
      "line": 23,
      "snippet": "cfg, err := config.Load(configPath)",
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
| `line`, `line_end` | Line range for focused reads |
| `snippet` | Code context without file read |
| `call_expr` | Exact text for find/replace |
| `args` | Arguments at call site |
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

## Commands

| Command | Description |
|---------|-------------|
| `wildcat callers <symbol>` | Who calls this function? |
| `wildcat callees <symbol>` | What does this function call? |
| `wildcat tree <symbol>` | Full call tree with depth control |
| `wildcat refs <symbol>` | All references to symbol |
| `wildcat implements <type>` | What implements this interface? |
| `wildcat deps <package>` | Package dependency graph |

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

# Find what implements an interface
wildcat implements io.Reader

# Package dependencies
wildcat deps ./internal/server
```

## Why "Wildcat"?

Fast, focused, gets the job done. No ceremony.

## License

MIT
