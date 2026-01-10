# Wildcat Research: Go Static Analysis for AI Agents

## Problem Statement

Claude Code often needs to answer questions about call trees and function relationships when refactoring Go code. It currently uses gopls and reads code directly. This works but isn't optimal.

**gopls limitations for AI agents:**
- Built for IDE workflows (real-time, incremental)
- LSP protocol overhead
- Designed for interactive use, not batch queries
- Returns data formatted for human consumption

## What AI Agents Actually Need

When refactoring, an AI agent asks questions like:
- "What calls this function?" (impact analysis)
- "What does this function call?" (dependency analysis)
- "Show me the call tree N levels deep" (understanding scope)
- "What implements this interface?" (polymorphism tracking)
- "What are the dependencies of this package?" (module boundaries)

**Key requirements:**
- Structured JSON output for parsing
- Fast, targeted queries
- No protocol overhead
- Call graph awareness

## Go Tooling Building Blocks

### Standard Library
- `go/ast` - parsing source into AST
- `go/types` - type checking, type information
- `go/parser` - parsing files and packages
- `go/token` - token positions, file sets

### golang.org/x/tools
- `go/packages` - load packages with full type info (replaces go/loader)
- `go/callgraph` - call graph construction
- `go/callgraph/cha` - Class Hierarchy Analysis (fast, imprecise)
- `go/callgraph/rta` - Rapid Type Analysis (medium)
- `go/callgraph/vta` - Variable Type Analysis (slower, precise)
- `go/ssa` - Static Single Assignment form

## Proposed Commands

```
wildcat callers <func>       # who calls this function?
wildcat callees <func>       # what does this function call?
wildcat tree <func>          # call tree to depth N
wildcat implements <type>    # what implements this interface?
wildcat refs <symbol>        # all references to symbol
wildcat deps <pkg>           # package dependency graph
```

## Design Questions

### 1. Target Identification
How do users specify what they're asking about?

Options:
- `package.Function` - fully qualified name
- `file.go:line` - file and line number
- `file.go:Function` - file and symbol name
- All of the above with smart detection

### 2. Call Graph Precision

| Algorithm | Speed | Precision | Use Case |
|-----------|-------|-----------|----------|
| CHA | Fast | Low | Quick exploration |
| RTA | Medium | Medium | General use |
| VTA | Slow | High | Precise refactoring |

Should we default to one and allow override? Or auto-select based on query?

### 3. Analysis Scope

Options:
- Current module only (fast, limited)
- Module + direct dependencies
- Full transitive dependencies (slow, complete)

### 4. Caching Strategy

Options:
- No caching (simple, always fresh)
- Index on first query, invalidate on file change
- Explicit `wildcat index` command
- Watch mode with incremental updates

### 5. Output Format

Primary: JSON for programmatic use
```json
{
  "query": "callers",
  "target": "pkg.Function",
  "results": [
    {"function": "main.main", "file": "main.go", "line": 42},
    {"function": "pkg.Helper", "file": "helper.go", "line": 17}
  ]
}
```

Secondary: Human-readable table/tree for debugging

## Open Questions

1. How do we handle methods vs functions? `Type.Method` syntax?
2. Should we support regex/glob patterns for bulk queries?
3. How do we represent indirect calls (interfaces, function values)?
4. What about test files - include by default or flag?
5. Cross-module analysis - how deep do we go?

## Next Steps

- [ ] Prototype basic package loading with `go/packages`
- [ ] Experiment with call graph algorithms
- [ ] Define target specification syntax
- [ ] Build minimal `callers` command as proof of concept
