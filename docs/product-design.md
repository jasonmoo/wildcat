# Wildcat Product Design

## Overview

Wildcat is a Go static analysis CLI built specifically for AI coding agents. It provides structured, actionable output that integrates directly with AI tool workflows (file read, edit, search).

## Target User

**Primary**: AI coding agents (Claude, GPT, etc.) operating via CLI tools

**User characteristics**:
- Queries code relationships during refactoring tasks
- Has file read/write/edit tools with specific input requirements
- Processes JSON natively
- Benefits from complete information in single queries
- Cannot interactively browse or "look around"

## Design Principles

1. **Query by symbol, not position**: Accept `pkg.Func` or `Type.Method`, resolve internally
2. **JSON-first**: All output is structured, parseable JSON
3. **Actionable results**: Include everything needed to act (paths, snippets, line numbers)
4. **Complete in one query**: Minimize round-trips; include context
5. **Fail helpfully**: Errors include suggestions for self-correction

## Commands

### MVP Commands

#### `wildcat callers <symbol>`

Find all callers of a function or method.

**Input**:
```
<symbol> := package.Function | Type.Method | path/to/package.Function
```

**Output**:
```json
{
  "query": {
    "command": "callers",
    "target": "config.Load",
    "resolved": "github.com/user/proj/internal/config.Load"
  },
  "target": {
    "symbol": "config.Load",
    "file": "/absolute/path/to/config/config.go",
    "line": 15,
    "signature": "func Load(path string) (*Config, error)"
  },
  "results": [
    {
      "symbol": "main.main",
      "package": "github.com/user/proj",
      "file": "/absolute/path/to/main.go",
      "line": 23,
      "line_end": 26,
      "snippet": "cfg, err := config.Load(configPath)\nif err != nil {\n    log.Fatal(err)\n}",
      "call_expr": "config.Load(configPath)",
      "args": ["configPath"],
      "in_test": false
    }
  ],
  "summary": {
    "count": 1,
    "packages": ["github.com/user/proj"],
    "in_tests": 0
  }
}
```

**Flags**:
| Flag | Description | Default |
|------|-------------|---------|
| `--exclude-tests` | Exclude `*_test.go` files | false |
| `--package <path>` | Limit to package path pattern | all |
| `--limit <n>` | Maximum results | unlimited |
| `--context <n>` | Lines of context in snippet | 3 |
| `--compact` | Omit snippets, minimal output | false |

---

#### `wildcat callees <symbol>`

Find all functions called by a function or method.

**Output**: Same structure as `callers`, with results showing what the target calls.

---

#### `wildcat tree <symbol>`

Build a call tree from a starting point.

**Flags**:
| Flag | Description | Default |
|------|-------------|---------|
| `--depth <n>` | Maximum tree depth | 3 |
| `--direction` | `up` (callers) or `down` (callees) | down |
| `--exclude-tests` | Exclude test files | false |
| `--exclude-stdlib` | Exclude standard library | false |

**Output**:
```json
{
  "query": {
    "command": "tree",
    "root": "main.main",
    "depth": 3,
    "direction": "down"
  },
  "nodes": {
    "main.main": {
      "file": "/path/to/main.go",
      "line": 10,
      "signature": "func main()",
      "calls": ["cmd.Execute", "os.Exit"]
    },
    "cmd.Execute": {
      "file": "/path/to/cmd/root.go",
      "line": 15,
      "signature": "func Execute() error",
      "calls": ["config.Load", "server.Start"]
    },
    "config.Load": {
      "file": "/path/to/config/config.go",
      "line": 23,
      "signature": "func Load(path string) (*Config, error)",
      "calls": ["os.ReadFile", "json.Unmarshal"]
    }
  },
  "edges": [
    {"from": "main.main", "to": "cmd.Execute", "file": "/path/to/main.go", "line": 11},
    {"from": "main.main", "to": "os.Exit", "file": "/path/to/main.go", "line": 14},
    {"from": "cmd.Execute", "to": "config.Load", "file": "/path/to/cmd/root.go", "line": 18}
  ],
  "summary": {
    "node_count": 3,
    "edge_count": 5,
    "max_depth_reached": 3,
    "truncated": false
  }
}
```

**Why both `nodes` and `edges`**:
- `nodes`: For reading files, understanding each function
- `edges`: For understanding flow, traversing relationships

---

#### `wildcat refs <symbol>`

Find all references to a symbol (not just calls).

**Use cases**:
- Variable references
- Type references
- Constant usage
- Function references (not calls, e.g., passed as value)

**Output**: Same structure as `callers`.

---

#### `wildcat impact <symbol>`

Comprehensive impact analysis: everything affected by changing a symbol.

**Purpose**: Answer "What breaks if I change this?" in one query.

**What it includes**:
- All callers (transitive, not just direct)
- All references (type usage, not just calls)
- Interface implementations (if changing an interface)
- Implementing types (if changing a method signature)
- Dependent packages

**Output**:
```json
{
  "query": {
    "command": "impact",
    "target": "config.Config",
    "resolved": "github.com/user/proj/internal/config.Config"
  },
  "target": {
    "symbol": "config.Config",
    "kind": "type",
    "file": "/path/to/config/config.go",
    "line": 10
  },
  "impact": {
    "callers": [
      {
        "symbol": "server.New",
        "file": "/path/to/server/server.go",
        "line": 25,
        "snippet": "func New(cfg *config.Config) *Server {",
        "reason": "parameter type"
      }
    ],
    "references": [
      {
        "symbol": "handler.Setup",
        "file": "/path/to/handler/handler.go",
        "line": 15,
        "snippet": "var cfg config.Config",
        "reason": "variable declaration"
      }
    ],
    "implementations": [],
    "dependents": [
      {
        "package": "github.com/user/proj/internal/server",
        "import_line": 8,
        "file": "/path/to/server/server.go"
      },
      {
        "package": "github.com/user/proj/internal/handler",
        "import_line": 5,
        "file": "/path/to/handler/handler.go"
      }
    ]
  },
  "summary": {
    "total_locations": 12,
    "callers": 5,
    "references": 4,
    "implementations": 0,
    "dependent_packages": 3,
    "in_tests": 2
  }
}
```

**Flags**:
| Flag | Description | Default |
|------|-------------|---------|
| `--exclude-tests` | Exclude test files | false |
| `--depth <n>` | Max depth for transitive callers | unlimited |
| `--include-deps` | Include impact in dependencies | false |

**Use case**: Before any refactoring, run `impact` to understand the full scope of changes needed.

---

### Future Commands

#### `wildcat implements <interface>`

Find all types implementing an interface.

```json
{
  "interface": "io.Reader",
  "implementations": [
    {
      "type": "*bytes.Buffer",
      "package": "bytes",
      "file": "/path/to/file.go",
      "line": 45
    }
  ]
}
```

#### `wildcat satisfies <type>`

Find all interfaces a type satisfies.

#### `wildcat deps <package>`

Package dependency graph.

```bash
wildcat deps ./internal/server              # What does server import?
wildcat deps ./internal/server --reverse    # What imports server?
```

---

## Output Format Specification

### Common Fields

All results include these fields where applicable:

| Field | Type | Description |
|-------|------|-------------|
| `symbol` | string | Fully qualified symbol name |
| `package` | string | Import path |
| `file` | string | **Absolute** file path |
| `line` | int | Start line (1-indexed) |
| `line_end` | int | End line (1-indexed) |
| `signature` | string | Function/method signature |
| `snippet` | string | Source code at location |
| `call_expr` | string | The call expression text |
| `args` | []string | Arguments at call site |
| `in_test` | bool | Whether in a test file |

### Path Resolution

Paths are always absolute. This allows direct use with file tools:

```json
{"file": "/home/user/proj/internal/config/config.go"}
```

Not:
```json
{"file": "internal/config/config.go"}
```

### Snippets

Snippets include the call site with configurable context:

- Default: 3 lines (1 before, call, 1 after)
- `--context 0`: Just the call expression
- `--context 5`: 5 lines before and after

Snippets preserve original indentation.

### Summary Block

Every response includes a summary for quick assessment:

```json
{
  "summary": {
    "count": 15,
    "packages": ["pkg1", "pkg2"],
    "in_tests": 3,
    "truncated": false
  }
}
```

---

## Error Handling

### Error Response Format

```json
{
  "error": {
    "code": "symbol_not_found",
    "message": "Cannot resolve symbol 'config.Laod'",
    "suggestions": ["config.Load", "config.LoadFromFile", "config.LoadDefault"],
    "context": {
      "searched": ["github.com/user/proj/..."],
      "similar_symbols": 3
    }
  }
}
```

### Error Codes

| Code | Description |
|------|-------------|
| `symbol_not_found` | Symbol doesn't exist; includes suggestions |
| `ambiguous_symbol` | Multiple matches; includes candidates |
| `package_not_found` | Package path doesn't exist |
| `parse_error` | Source code has syntax errors |
| `load_error` | Failed to load packages |

### Self-Correction

Errors include `suggestions` array with likely intended symbols. AI can retry with corrected input without user intervention.

---

## Analysis Backend

### Call Graph Algorithms

Wildcat uses Go's `golang.org/x/tools/go/callgraph` package.

| Algorithm | Speed | Precision | Use Case |
|-----------|-------|-----------|----------|
| CHA | Fast | Low | Quick exploration, large codebases |
| RTA | Medium | Medium | General use (default) |
| VTA | Slow | High | Precise refactoring |

**Default**: RTA (reasonable precision, acceptable speed)

**Flag**: `--algorithm cha|rta|vta`

### Scope Control

| Scope | Description | Flag |
|-------|-------------|------|
| Module | Current go.mod only | default |
| Direct deps | Module + direct dependencies | `--include-deps` |
| All deps | Full transitive closure | `--include-all-deps` |

---

## Performance Considerations

### Caching

- First query builds package/type index
- Subsequent queries reuse index
- Index invalidated on file modification time change
- Optional: `wildcat index` to pre-build

### Large Codebases

- Default limit: 100 results (override with `--limit`)
- `summary.truncated: true` indicates more results available
- Use `--package` to narrow scope

### Memory

- Stream results where possible
- Don't load entire call graph into memory for simple queries

---

## Integration Examples

### AI Refactoring Workflow

```bash
# 1. Find what to change
CALLERS=$(wildcat callers config.Load)

# 2. AI reads the output, understands each call site via snippets

# 3. For each result, AI can:
#    - Read file at exact line: Read(file, offset=line)
#    - Edit using call_expr: Edit(file, old=call_expr, new=updated_call)

# 4. Verify no breaks
wildcat callers config.Load --compact  # Quick recheck
```

### Impact Analysis

```bash
# Before changing a function, understand impact:
wildcat tree myFunc --direction up --depth 5

# AI sees all code paths that lead to myFunc
# Can assess risk and identify test coverage
```

---

## Open Questions

1. **Receiver types**: How to specify `(*Server).Start` vs `Server.Start`?
2. **Generics**: How to handle generic functions/types?
3. **Build tags**: Honor build constraints? Flag to override?
4. **Vendored deps**: Include in analysis by default?

---

## References

- [gopls Gap Analysis](./gopls-report.md)
- [Initial Research](./research.md)
- [golang.org/x/tools/go/callgraph](https://pkg.go.dev/golang.org/x/tools/go/callgraph)
