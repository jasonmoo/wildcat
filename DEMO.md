# Wildcat Demo (2 min)

## Opening (15 sec)

> "LSP powers every modern IDE. But it was designed for humans at a cursor, not AI agents trying to understand entire codebases.
>
> Wildcat is an LSP orchestrator that provides code intelligence for AI."

## The Gap (15 sec)

> "When AI asks 'what calls this?' or 'what breaks if I change this?' - grep gives text matches, LSP gives cursor-position answers. Neither is what AI needs.
>
> Wildcat gives structured, complete answers."

---

## Demo (60 sec)

### 1. Find all callers
```bash
./bin/wildcat callers lsp.NewClient
```
> "Every function that calls NewClient - with resolved symbols, file paths, line numbers, and code snippets."

```json
{
  "query": {
    "command": "callers",
    "target": "lsp.NewClient",
    "resolved": "github.com/jasonmoo/wildcat/internal/lsp.NewClient"
  },
  "target": {
    "symbol": "github.com/jasonmoo/wildcat/internal/lsp.NewClient",
    "file": "/home/jason/.../internal/lsp/client.go",
    "line": 18
  },
  "results": [
    {
      "symbol": "runCallees",
      "file": "/home/jason/.../cmd/callees.go",
      "line": 56,
      "snippet": "\tclient, err := lsp.NewClient(ctx, config)\n\tif err != nil {\n\t\treturn writer.WriteError(",
      "call_expr": "NewClient",
      "in_test": false
    },
    {
      "symbol": "runCallers",
      "file": "/home/jason/.../cmd/callers.go",
      "line": 57,
      "snippet": "...",
      "in_test": false
    },
    // ... 6 more callers
  ],
  "summary": {"count": 8, "in_tests": 0}
}
```

---

### 2. Impact analysis
```bash
./bin/wildcat impact output.Formatter
```
> "What breaks if I change this interface? References AND implementations - one query."

```json
{
  "query": {
    "command": "impact",
    "target": "output.Formatter",
    "resolved": "github.com/jasonmoo/wildcat/internal/output.Formatter"
  },
  "target": {
    "symbol": "github.com/jasonmoo/wildcat/internal/output.Formatter",
    "kind": "interface",
    "file": "/home/jason/.../internal/output/formatter.go",
    "line": 16
  },
  "impact": {
    "references": [
      {"file": ".../formatter.go", "line": 30, "reason": "references this symbol"},
      {"file": ".../formatter.go", "line": 36, "reason": "references this symbol"},
      {"file": ".../json.go", "line": 12, "reason": "references this symbol"},
      // ... 4 more references
    ],
    "implementations": [
      {"file": ".../formatter.go", "line": 108},
      {"file": ".../formatter.go", "line": 123},
      // ... 4 more implementations
    ]
  },
  "summary": {"total_locations": 13, "references": 7, "implementations": 6}
}
```

---

### 3. Find implementations
```bash
./bin/wildcat implements output.Formatter --compact
```
> "All 6 types implementing Formatter. Try doing that with grep."

```json
{
  "query": {
    "command": "implements",
    "target": "output.Formatter",
    "resolved": "github.com/jasonmoo/wildcat/internal/output.Formatter"
  },
  "interface": {
    "symbol": "github.com/jasonmoo/wildcat/internal/output.Formatter",
    "kind": "interface",
    "file": "/home/jason/.../internal/output/formatter.go",
    "line": 16
  },
  "implementations": [
    {"file": ".../formatter.go", "line": 108, "in_test": false},
    {"file": ".../formatter.go", "line": 123, "in_test": false},
    {"file": ".../formatter.go", "line": 199, "in_test": false},
    {"file": ".../formatter.go", "line": 267, "in_test": false},
    {"file": ".../formatter.go", "line": 351, "in_test": false},
    {"file": ".../formatter.go", "line": 386, "in_test": false}
  ],
  "summary": {"count": 6, "in_tests": 0, "truncated": false}
}
```

---

### 4. Multiple output formats
```bash
./bin/wildcat callers output.NewWriter -o markdown
```
> "JSON for AI tools, Markdown for humans, DOT for visualization."

```markdown
# Callers: output.NewWriter

| Symbol | File | Line |
|--------|------|------|
| GetWriter | .../cmd/server.go | 21 |
| TestWriter_Write | .../internal/output/json_test.go | 9 |
| TestWriter_WriteError | .../internal/output/json_test.go | 61 |
| TestWriter_PrettyPrint | .../internal/output/json_test.go | 88 |

## Summary

- **count**: 4
- **in_tests**: 3
```

---

## Curveball (20 sec)

```bash
./bin/wildcat formats
```
> "The curveball was output plugins. 4 built-in formats, plus custom templates and external plugins."

```
Available output formats:

  dot           Graphviz DOT format (for call trees)
  json          JSON output (default)
  markdown      Markdown tables and lists
  yaml          YAML output

Custom formats:
  template:<path>  Use a Go template file
  plugin:<name>    External plugin (wildcat-format-<name>)
```

---

## Close (10 sec)

> "Wildcat: code intelligence for AI. It speaks LSP so your agents don't have to."

---

## Pre-Demo Checklist

- [ ] Terminal font size large
- [ ] `./bin/wildcat` built and working
- [ ] Run through commands once
- [ ] gopls responsive
