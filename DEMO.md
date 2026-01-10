# Wildcat Demo (2 min)

## Opening (15 sec)

> "LSP - Language Server Protocol - powers every modern IDE. But it was designed for humans at a cursor, not AI agents trying to understand entire codebases.
>
> Wildcat is an LSP orchestrator that provides code intelligence for AI."

## The Gap (15 sec)

> "When an AI agent asks 'what calls this function?' or 'what breaks if I change this?' - grep gives you text matches, LSP gives you cursor-position answers. Neither is what AI needs.
>
> Wildcat gives you structured, complete answers."

## Demo (60 sec)

### 1. Find all callers
```bash
./bin/wildcat callers lsp.NewClient --compact
```
> "Every function that calls NewClient - with file paths and line numbers."

### 2. Impact analysis
```bash
./bin/wildcat impact output.Formatter
```
> "What breaks if I change this interface? Callers, references, implementations - one query."

### 3. Type system queries
```bash
./bin/wildcat implements output.Formatter --compact
```
> "All 6 types implementing Formatter. Try doing that with grep."

### 4. Multiple output formats
```bash
./bin/wildcat callers output.NewWriter -o markdown
```
> "JSON for AI tools, Markdown for humans, DOT for visualization."

## Curveball (20 sec)

```bash
./bin/wildcat formats
```
> "The curveball was output plugins. We have 4 built-in formats, plus support for custom Go templates and external plugins. Extensible by design."

## Close (10 sec)

> "Wildcat: code intelligence for AI. It speaks LSP so your agents don't have to."

---

## Backup Commands (if time or questions)

```bash
# Reverse dependencies
./bin/wildcat deps ./internal/lsp --reverse

# What interfaces does a type satisfy?
./bin/wildcat satisfies JSONFormatter

# Self-documenting
./bin/wildcat readme --compact
```

## Pre-Demo Checklist

- [ ] Terminal font size large
- [ ] `./bin/wildcat` built and working
- [ ] Commands tested (run through once)
- [ ] gopls installed and responsive
