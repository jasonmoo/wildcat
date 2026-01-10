# Wildcat Demo (2 min) - Commands Only

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

---

### 2. Impact analysis
```bash
./bin/wildcat impact output.Formatter
```
> "What breaks if I change this interface? References AND implementations - one query."

---

### 3. Find implementations
```bash
./bin/wildcat implements output.Formatter --compact
```
> "All 6 types implementing Formatter. Try doing that with grep."

---

### 4. Multiple output formats
```bash
./bin/wildcat callers output.NewWriter -o markdown
```
> "JSON for AI tools, Markdown for humans, DOT for visualization."

---

## Curveball (20 sec)

```bash
./bin/wildcat formats
```
> "The curveball was output plugins. 4 built-in formats, plus custom templates and external plugins."

---

## Close (10 sec)

> "Wildcat: code intelligence for AI. It speaks LSP so your agents don't have to."

---

## Pre-Demo Checklist

- [ ] Terminal font size large
- [ ] `./bin/wildcat` built and working
- [ ] Run through commands once
- [ ] gopls responsive
