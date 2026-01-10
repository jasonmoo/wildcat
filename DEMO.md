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
./bin/wildcat callers lsp.NewClient --compact
```
> "Every function that calls NewClient - with file paths and line numbers."

```json
{
  "results": [
    {"symbol": "runCallees", "file": ".../cmd/callees.go", "line": 56},
    {"symbol": "runCallers", "file": ".../cmd/callers.go", "line": 57},
    {"symbol": "runImpact",  "file": ".../cmd/impact.go",  "line": 47},
    {"symbol": "runRefs",    "file": ".../cmd/refs.go",    "line": 56},
    {"symbol": "runTree",    "file": ".../cmd/tree.go",    "line": 48}
  ],
  "summary": {"count": 5}
}
```

---

### 2. Impact analysis
```bash
./bin/wildcat impact output.Formatter
```
> "What breaks if I change this interface? Callers, references, implementations - one query."

```json
{
  "target": {"symbol": "output.Formatter", "kind": "interface"},
  "impact": {
    "references": [
      {"file": ".../formatter.go", "line": 30, "reason": "references this symbol"},
      {"file": ".../formatter.go", "line": 36, "reason": "references this symbol"},
      {"file": ".../json.go",      "line": 12, "reason": "references this symbol"}
    ]
  },
  "summary": {"total_locations": 15, "references": 15}
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
  "interface": {"symbol": "output.Formatter", "kind": "interface"},
  "implementations": [
    {"file": ".../formatter.go", "line": 108},  // JSONFormatter
    {"file": ".../formatter.go", "line": 123},  // YAMLFormatter
    {"file": ".../formatter.go", "line": 199},  // DotFormatter
    {"file": ".../formatter.go", "line": 267},  // MarkdownFormatter
    {"file": ".../formatter.go", "line": 351},  // TemplateFormatter
    {"file": ".../formatter.go", "line": 386}   // PluginFormatter
  ],
  "summary": {"count": 6}
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

| Symbol              | File                  | Line |
|---------------------|----------------------|------|
| GetWriter           | .../cmd/server.go    | 21   |
| TestWriter_Write    | .../json_test.go     | 9    |
| TestWriter_WriteError | .../json_test.go   | 61   |

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
