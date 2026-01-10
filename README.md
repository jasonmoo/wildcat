# Wildcat

Code analysis built for AI agents. Language-agnostic via LSP.

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

Wildcat is purpose-built for AI agents. One query, actionable results. Works with any language that has an LSP server.

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

### Multi-Language Support

Wildcat works with any language that has an LSP server supporting call hierarchy (LSP 3.16+):

| Language | Server | Status |
|----------|--------|--------|
| Go | gopls | ✅ Full support |
| Python | pyright | ✅ Full support |
| TypeScript/JavaScript | typescript-language-server | ✅ Full support |
| Rust | rust-analyzer | ✅ Full support |
| C/C++ | clangd | ✅ Full support |
| Java | jdtls | ✅ Full support |

Wildcat auto-detects the language and starts the appropriate server.

## Commands

| Command | Description |
|---------|-------------|
| `wildcat callers <symbol>` | Who calls this function? |
| `wildcat callees <symbol>` | What does this function call? |
| `wildcat tree <symbol>` | Full call tree with depth control |
| `wildcat refs <symbol>` | All references to symbol |
| `wildcat impact <symbol>` | What breaks if I change this? |
| `wildcat implements <type>` | What implements this interface? |
| `wildcat deps <package>` | Package dependency graph |
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

# Package dependencies
wildcat deps ./internal/server
```

## Why "Wildcat"?

Fast, focused, gets the job done. No ceremony.

## License

MIT
