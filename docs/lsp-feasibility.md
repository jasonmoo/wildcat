# LSP Feasibility Analysis

## Executive Summary

**Recommendation: LSP-based approach is viable.**

LSP provides the core features we need. Wildcat can act as an intelligent LSP orchestrator - making multiple calls, handling recursion, extracting snippets, and formatting output for AI consumption. The approach enables language-agnostic support.

---

## Feature Mapping

| Wildcat Command | LSP Method(s) | Supported? | Notes |
|-----------------|---------------|------------|-------|
| `callers` | `callHierarchy/incomingCalls` | ✅ Yes | Direct callers; recurse for transitive |
| `callees` | `callHierarchy/outgoingCalls` | ✅ Yes | Direct callees; recurse for transitive |
| `tree` | Recursive call hierarchy | ✅ Yes | Wildcat handles recursion |
| `refs` | `textDocument/references` | ✅ Yes | Returns all reference locations |
| `implements` | `textDocument/implementation` | ✅ Yes | Interface → implementations |
| `satisfies` | `typeHierarchy/supertypes` | ✅ Yes | Type → interfaces it satisfies |
| `impact` | Combination of above | ✅ Yes | Wildcat combines results |
| `deps` | Not in LSP | ⚠️ Partial | Language-specific; could parse imports |

---

## LSP Call Hierarchy Details

### Protocol (LSP 3.16+)

Three-step process:

```
1. textDocument/prepareCallHierarchy(position) → CallHierarchyItem
2. callHierarchy/incomingCalls(item) → CallHierarchyIncomingCall[]
3. callHierarchy/outgoingCalls(item) → CallHierarchyOutgoingCall[]
```

### Response Structure

**CallHierarchyItem:**
```json
{
  "name": "functionName",
  "kind": 12,  // SymbolKind.Function
  "uri": "file:///path/to/file.go",
  "range": { "start": {"line": 10, "character": 0}, "end": {"line": 15, "character": 1} },
  "selectionRange": { "start": {"line": 10, "character": 5}, "end": {"line": 10, "character": 17} },
  "detail": "package.functionName"
}
```

**CallHierarchyIncomingCall:**
```json
{
  "from": { /* CallHierarchyItem of caller */ },
  "fromRanges": [
    { "start": {"line": 25, "character": 4}, "end": {"line": 25, "character": 20} }
  ]
}
```

### What LSP Provides vs What We Add

| Data | LSP Provides | Wildcat Adds |
|------|--------------|--------------|
| Function name | ✅ `name` | - |
| File path | ✅ `uri` | Convert to absolute path |
| Line number | ✅ `range.start.line` | 1-indexed conversion |
| Call location | ✅ `fromRanges` | - |
| Snippet | ❌ | Read file at position |
| Call expression | ❌ | Extract from snippet |
| Arguments | ❌ | Parse from call expression |
| in_test flag | ❌ | Check filename pattern |
| Transitive depth | ❌ | Recursive LSP calls |

---

## Wildcat as LSP Orchestrator

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     Wildcat CLI                          │
├─────────────────────────────────────────────────────────┤
│                  Command Layer                           │
│  (callers, callees, tree, refs, impact, implements)      │
├─────────────────────────────────────────────────────────┤
│                  Orchestration Layer                     │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────────┐│
│  │   Symbol    │ │  Recursive  │ │      Snippet        ││
│  │   Lookup    │ │  Traversal  │ │     Extraction      ││
│  └─────────────┘ └─────────────┘ └─────────────────────┘│
├─────────────────────────────────────────────────────────┤
│                    LSP Client                            │
│           (JSON-RPC over stdio/tcp)                      │
├─────────────────────────────────────────────────────────┤
│                  LSP Servers                             │
│  ┌────────┐ ┌────────────┐ ┌────────┐ ┌───────────────┐ │
│  │ gopls  │ │rust-analyzer│ │pyright │ │typescript-ls │ │
│  └────────┘ └────────────┘ └────────┘ └───────────────┘ │
└─────────────────────────────────────────────────────────┘
```

### Example Flow: `wildcat callers config.Load --depth 2`

```
1. Start/connect to LSP server (if not running)

2. Symbol lookup:
   → workspace/symbol("Load")
   ← [{ name: "Load", uri: "file:///config.go", range: {...} }, ...]
   → Filter to match "config.Load"
   → Get position from result

3. Prepare call hierarchy:
   → textDocument/prepareCallHierarchy({ uri, position })
   ← CallHierarchyItem for config.Load

4. Get direct callers (depth 1):
   → callHierarchy/incomingCalls(item)
   ← [{ from: main.main, fromRanges: [...] }, { from: server.init, ... }]

5. Recurse for depth 2:
   → For each caller, call incomingCalls again
   ← More callers...

6. Extract snippets:
   → Read each file at the call position
   → Extract lines with context

7. Format output:
   → Build our JSON structure with all fields
```

---

## Cross-Language Support Matrix

| Language | Server | Call Hierarchy | References | Implementation | Type Hierarchy |
|----------|--------|----------------|------------|----------------|----------------|
| Go | gopls | ✅ | ✅ | ✅ | ✅ |
| Rust | rust-analyzer | ✅ | ✅ | ✅ | ✅ |
| Python | pyright | ✅ | ✅ | ✅ | ✅ |
| TypeScript/JS | typescript-ls | ✅ | ✅ | ✅ | ✅ |
| C/C++ | clangd | ✅ | ✅ | ✅ | ✅ |
| Java | jdtls | ✅ | ✅ | ✅ | ✅ |

**Call Hierarchy added in LSP 3.16 (2020). All major servers support it.**

---

## What Wildcat Handles Generically

These features work for ANY language with a compliant LSP server:

### 1. Symbol Lookup by Name
```
User: "config.Load"
  → workspace/symbol("Load")
  → Filter results by package/module pattern
  → Return position for LSP calls
```

### 2. Recursive Traversal
```
User: "--depth 3"
  → Call incomingCalls/outgoingCalls
  → For each result, recursively call again
  → Track visited nodes (avoid cycles)
  → Stop at depth limit
```

### 3. Snippet Extraction
```
LSP returns: { uri: "file:///foo.go", range: { start: {line: 25} } }
Wildcat:
  → Read file at uri
  → Extract lines 23-27 (with context)
  → Return as snippet field
```

### 4. Output Formatting
```
LSP returns: Multiple responses with different structures
Wildcat:
  → Normalize to our JSON format
  → Add computed fields (in_test, call_expr, args)
  → Include summary block
```

---

## Gap Analysis

### Fully Solved by LSP

| Feature | Solution |
|---------|----------|
| Find callers | `callHierarchy/incomingCalls` |
| Find callees | `callHierarchy/outgoingCalls` |
| Call tree | Recursive calls |
| References | `textDocument/references` |
| Implementations | `textDocument/implementation` |
| Type hierarchy | `typeHierarchy/*` |

### Solved by Wildcat (Language-Agnostic)

| Feature | Solution |
|---------|----------|
| Symbol by name | `workspace/symbol` + filtering |
| Snippets | Read file at position |
| Call expression | Parse snippet |
| in_test detection | Filename pattern matching |
| Transitive queries | Recursive orchestration |
| JSON formatting | Build our output structure |

### Requires Language-Specific Handling

| Feature | Challenge | Mitigation |
|---------|-----------|------------|
| Package dependencies | Not in LSP spec | Parse import statements (simple AST) |
| Argument extraction | Language syntax varies | Regex per language OR skip |

---

## Performance Considerations

### LSP Server Lifecycle
- **Cold start**: 1-5 seconds for most servers
- **Warm queries**: Milliseconds
- **Strategy**: Keep server running, reuse connection

### Query Complexity
| Query Type | LSP Calls | Estimated Time |
|------------|-----------|----------------|
| Direct callers | 2 | ~100ms |
| Callers depth 3 | 2 + (N × depth) | ~500ms-2s |
| Full impact | 5-10 | ~1-3s |

### Optimization Strategies
1. **Server pooling**: Keep LSP servers running between queries
2. **Parallel queries**: Call multiple LSP methods concurrently
3. **Result caching**: Cache call hierarchy items for reuse
4. **Lazy snippets**: Only extract snippets when `--compact` not set

---

## Implementation Approach

### Phase 1: LSP Client Core
- JSON-RPC client over stdio
- Server lifecycle management (start, keep-alive, shutdown)
- Request/response handling with timeouts

### Phase 2: Generic Features
- workspace/symbol with filtering
- Call hierarchy orchestration (recursive)
- Snippet extraction
- Output formatting

### Phase 3: Server Configurations
- Server detection (gopls, rust-analyzer, etc.)
- Per-language startup commands
- Capability detection

### Phase 4: Language-Specific Plugins (if needed)
- Import/dependency parsing
- Argument extraction
- Custom symbol resolution

---

## Comparison: Go-Specific vs LSP

| Aspect | Go-Specific | LSP-Based |
|--------|-------------|-----------|
| Language support | Go only | Any with LSP server |
| Precision | Very high (VTA algorithm) | Server-dependent |
| Performance | In-process, fast | IPC overhead, still fast |
| Snippet extraction | Direct file access | Same (read files) |
| Package deps | Full support | Needs language plugin |
| Implementation effort | Medium | Medium |
| Maintenance | Go-only updates | Multi-server updates |
| User value | Go developers | All developers |

---

## Recommendation

### Go with LSP-Based Approach

**Reasons:**
1. **Broader impact**: Works with Go, Python, TypeScript, Rust, C++, Java
2. **LSP is mature**: Call hierarchy (LSP 3.16) supported by all major servers
3. **Gaps are small**: Only `deps` command needs language-specific work
4. **Orchestration is tractable**: Recursive traversal, snippet extraction are straightforward
5. **Performance is acceptable**: Sub-second for most queries

### Suggested Architecture

```
wildcat (Go binary)
├── cmd/          # CLI commands
├── internal/
│   ├── lsp/      # LSP client (JSON-RPC)
│   ├── servers/  # Server configs (gopls, rust-analyzer, etc.)
│   ├── symbols/  # Symbol lookup and filtering
│   ├── traverse/ # Recursive call hierarchy
│   ├── snippets/ # File reading, snippet extraction
│   └── output/   # JSON formatting
└── configs/      # Per-language server configurations
```

### Next Steps

1. Update tickets to reflect LSP architecture
2. Create LSP client infrastructure ticket
3. Prototype with gopls first (we know it well)
4. Expand to other languages

---

## References

- [LSP 3.17 Specification](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/)
- [Call Hierarchy in rust-analyzer](https://github.com/rust-lang/rust-analyzer/pull/2698)
- [Pyright LSP Support](https://github.com/emacs-lsp/lsp-pyright)
- [clangd Call Hierarchy](https://github.com/clangd/clangd/issues/162)
- [typescript-language-server](https://github.com/typescript-language-server/typescript-language-server)
