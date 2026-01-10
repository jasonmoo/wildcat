# AI-Consumable Output Format Research

Research for optimal CLI output formats for call trees, code graphs, and related information optimized for LLM/AI consumption.

## Problem Statement

We need a CLI output format that:
1. Is easy for AI agents (Claude Code) to parse and reason about
2. Minimizes token usage (every token counts in context windows)
3. Supports streaming for large codebases
4. Represents hierarchical call trees and graph relationships clearly

## Format Comparison

| Format | Token Efficiency | Parseability | Nested Data | Streaming |
|--------|------------------|--------------|-------------|-----------|
| JSON | Poor (~2x TSV) | Excellent | Excellent | No |
| JSONL | Good | Excellent | Good | Yes |
| YAML | Poor | Good | Excellent | No |
| TSV/CSV | Excellent | Good | Poor | Yes |
| DOT | Medium | Fair | N/A (graphs) | No |
| TOON | Excellent (30-60% < JSON) | Good | Good | Yes |

**Key insight:** JSON uses ~2x the tokens of TSV due to quotes, colons, braces. For LLM consumption, every token counts.

## Existing Tool Formats

### 1. LSP Call Hierarchy (gopls uses this)

```json
{
  "name": "FunctionName",
  "kind": 12,
  "uri": "file:///path/to/file.go",
  "range": {"start": {"line": 10, "character": 0}, "end": {"line": 20, "character": 1}},
  "selectionRange": {"start": {"line": 10, "character": 5}, "end": {"line": 10, "character": 17}}
}
```

- **Pros:** Standardized, well-documented, precise positions
- **Cons:** Verbose, designed for IDEs not batch consumption

### 2. Universal Ctags JSON

```jsonl
{"_type": "tag", "name": "Function", "path": "file.go", "line": 42, "kind": "function", "scope": "package", "scopeKind": "package"}
```

- **Pros:** JSONL streaming, compact, well-documented schema
- **Cons:** Tag-focused, not call-graph aware

### 3. Aider Repo Map Format

```
pkg/module.go:
├─ class Module
│   ├─ method Process(input Input) Output
│   └─ method Validate() error
└─ func NewModule(config Config) *Module
```

- **Pros:** Very compact, tree-structured, token-efficient
- **Cons:** Custom format, parsing complexity

### 4. Tree-sitter Node JSON

```json
{"type": "function_declaration", "startPosition": {"row": 10, "column": 0}, "endPosition": {"row": 20, "column": 1}, "children": [...]}
```

- **Pros:** Precise AST positions, language-agnostic
- **Cons:** Too low-level for call graphs

## Recommendations for Wildcat

### Primary Format: Compact JSONL

Design principles:
1. **One record per line** - enables streaming, grep-friendly
2. **Short keys** - `fn` not `functionName`, `loc` not `location`
3. **Colon-delimited positions** - `file.go:42:5` not `{"file": "file.go", "line": 42, "col": 5}`
4. **Flat when possible** - avoid nesting unless necessary
5. **Include depth/level** - for tree traversal context

#### Proposed Schema

```jsonl
{"t":"node","fn":"pkg.Function","loc":"file.go:42:5","sig":"func(x int) error"}
{"t":"edge","from":"main.main","to":"pkg.Function","loc":"main.go:15:8","kind":"call"}
{"t":"edge","from":"pkg.Function","to":"fmt.Println","loc":"file.go:45:3","kind":"call"}
```

Record types:
- `node` - function/method definition
- `edge` - call relationship
- `impl` - interface implementation
- `ref` - symbol reference

#### Example: Call Tree Output

```jsonl
{"t":"caller","fn":"main.main","loc":"main.go:42:5","depth":1}
{"t":"caller","fn":"pkg.Helper","loc":"helper.go:17:2","depth":1}
{"t":"caller","fn":"pkg.Init","loc":"init.go:8:10","depth":2}
```

### Secondary Format: Tree Visualization

For human debugging and quick orientation:

```
pkg.Function (file.go:42)
├── [calls] fmt.Println (stdlib)
├── [calls] pkg.helper (helper.go:10)
│   └── [calls] pkg.util (util.go:5)
└── [called by] main.main (main.go:15)
```

Or Aider-style for definitions:

```
main.go:
├─ func main()
└─ func init()

pkg/handler.go:
├─ type Handler struct
│   ├─ method ServeHTTP(w, r)
│   └─ method Close() error
└─ func NewHandler(cfg Config) *Handler
```

## Token Optimization Techniques

### 1. Abbreviate Common Patterns

- `stdlib` instead of full paths for standard library
- Relative paths from module root
- Short type aliases: `s` for string, `e` for error

### 2. Omit Defaults

- Don't include `"kind": "call"` if calls are the default
- Don't include `"depth": 0` for root nodes
- Skip empty arrays/objects

### 3. Consider TOON Format

For very large outputs, TOON (Token-Optimized Object Notation) saves 30-60% tokens:

```
t:node fn:pkg.Function loc:file.go:42:5
t:edge from:main.main to:pkg.Function loc:main.go:15:8
```

## Graph-Specific Considerations

### Adjacency List Format (compact)

```jsonl
{"fn":"main.main","calls":["pkg.A","pkg.B"],"calledBy":[]}
{"fn":"pkg.A","calls":["pkg.C"],"calledBy":["main.main"]}
```

### Edge List Format (flexible)

```jsonl
{"from":"main.main","to":"pkg.A"}
{"from":"main.main","to":"pkg.B"}
{"from":"pkg.A","to":"pkg.C"}
```

### Include Metadata Flags

```jsonl
{"from":"main.main","to":"pkg.Handler","kind":"interface","conf":"static"}
{"from":"pkg.Run","to":"unknown","kind":"dynamic","conf":"low"}
```

## Relevance Ranking

For large codebases, not all information is equally useful. Aider's repo map approach:

1. **PageRank on file graph** - files with more cross-references rank higher
2. **Token budget awareness** - binary search to fit within context limits
3. **Tree-sitter extraction** - pull only definitions, not implementations

Consider adding a `rank` or `score` field:

```jsonl
{"fn":"pkg.CoreFunction","loc":"core.go:10","score":0.95}
{"fn":"pkg.UtilHelper","loc":"util.go:100","score":0.12}
```

## Format Decision Matrix

| Use Case | Recommended Format | Rationale |
|----------|-------------------|-----------|
| Single function callers | Compact JSONL | Fast parsing, low tokens |
| Full call graph | Edge list JSONL | Flexible for analysis |
| Codebase overview | Aider-style tree | Most compact, scannable |
| Debug output | Tree visualization | Human readable |
| Piping to other tools | JSONL | Standard, streamable |
| Very large output | TOON | Maximum token savings |

## Implementation Flags

Proposed CLI flags:

```
--format json      # Standard JSON (pretty, one object)
--format jsonl     # JSON Lines (default, streaming)
--format tree      # Human-readable tree
--format dot       # Graphviz DOT format
--format compact   # TOON or minimal format

--depth N          # Limit traversal depth
--limit N          # Limit number of results
--score            # Include relevance scores
--no-stdlib        # Exclude standard library
```

## Sources

- [LSP 3.17 Specification](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/)
- [Universal Ctags JSON Output](https://docs.ctags.io/en/latest/man/ctags-json-output.5.html)
- [Aider Repository Map](https://aider.chat/docs/repomap.html)
- [Building a Better Repo Map with Tree-sitter](https://aider.chat/2023/10/22/repomap.html)
- [LLM Output Formats: Why JSON Costs More Than TSV](https://david-gilbertson.medium.com/llm-output-formats-why-json-costs-more-than-tsv-ebaf590bd541)
- [TOON vs JSON: Token-Optimized Format](https://www.tensorlake.ai/blog/toon-vs-json)
- [Graphviz JSON Output](https://graphviz.org/docs/outputs/json/)
- [RepoMapper MCP Server](https://github.com/pdavis68/RepoMapper)
- [gopls Call Hierarchy](https://go.dev/gopls/features/navigation)
