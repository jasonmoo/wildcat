# Gopls Capabilities Report

## Overview

**Version tested:** v0.20.0
**Protocol:** Language Server Protocol (LSP)
**Primary use case:** IDE integration for real-time editing

Gopls is the official Go language server. It's designed for IDEs and editors, providing real-time analysis, completion, and navigation. It recently added experimental MCP (Model Context Protocol) support for AI assistants.

---

## CLI Commands

Gopls exposes most LSP features via CLI for debugging and scripting:

### Navigation Commands

| Command | Description | Output Format |
|---------|-------------|---------------|
| `definition <file:line:col>` | Go to symbol definition | Location (supports `-json`) |
| `references <file:line:col>` | Find all references to symbol | List of locations |
| `implementation <file:line:col>` | Find interface implementations | List of locations |
| `call_hierarchy <file:line:col>` | Show callers/callees | Hierarchical text |
| `symbols <file>` | List symbols in file | `Name Type line:col` |
| `workspace_symbol <query>` | Fuzzy search all symbols | List of matches |
| `signature <file:line:col>` | Show function signature | Signature text |

### Analysis Commands

| Command | Description | Output Format |
|---------|-------------|---------------|
| `check <file>` | Run diagnostics on file | Diagnostic messages |
| `stats` | Workspace statistics (JSON) | JSON object |

### Transformation Commands

| Command | Description | Output Format |
|---------|-------------|---------------|
| `rename <file:line:col> <name>` | Rename symbol | Diffs (supports `-diff`, `-write`) |
| `format <file>` | Format source code | Formatted source |
| `imports <file>` | Organize imports | Modified source |
| `codeaction <file:line:col>` | List/execute code actions | Actions list or diffs |
| `codelens <file>` | List/execute code lenses | Lenses list |

### Other Commands

| Command | Description |
|---------|-------------|
| `serve` | Run as LSP server |
| `mcp` | Run as MCP server (experimental) |
| `api-json` | Print full API spec as JSON |

---

## Position Specification

All position-based commands use: `file.go:line:column` or `file.go:#offset`

- Line and column are **1-indexed**
- Offset is byte offset from start of file

Example:
```bash
gopls definition cmd/root.go:13:6
gopls definition cmd/root.go:#150
```

---

## Output Formats

### Default (Human-Readable)

```
# symbols
rootCmd Variable 7:5-7:12
Execute Function 13:6-13:13

# references
/path/to/file.go:14:9-16
/path/to/file.go:18:2-9

# call_hierarchy
caller[0]: ranges 10:16-23 in main.go from/to function main
identifier: function Execute in cmd/root.go:13:6-13
callee[0]: ranges 14:17-24 from/to function Execute
```

### JSON (where supported)

Only `definition` and `api-json` have native JSON output:

```json
{
  "span": {
    "uri": "file:///path/to/file.go",
    "start": {"line": 7, "column": 5, "offset": 55},
    "end": {"line": 7, "column": 12, "offset": 62}
  },
  "description": "var rootCmd *cobra.Command"
}
```

---

## Call Hierarchy Details

The `call_hierarchy` command provides:
- **Callers**: What functions call the target
- **Callees**: What functions the target calls
- Position ranges for each call site

**Limitations:**
- Dynamic/interface calls are excluded
- Only shows direct relationships (not transitive)
- Output is text-based, not structured JSON
- Must specify exact position, not symbol name

---

## MCP Integration (Experimental)

Gopls v0.20+ includes MCP server support for AI agents.

### Available MCP Tools

| Tool | Purpose |
|------|---------|
| `go_workspace` | Discover workspace structure |
| `go_search` | Fuzzy symbol search |
| `go_file_context` | Understand file and intra-package deps |
| `go_package_api` | Get package's public API |
| `go_symbol_references` | Find all references to symbol |
| `go_diagnostics` | Check for errors after edits |

### Usage Modes

1. **Attached**: Runs with active LSP session, shares state
   ```bash
   gopls serve -mcp.listen=localhost:8092
   ```

2. **Detached**: Standalone, only sees saved files
   ```bash
   gopls mcp  # over stdio
   ```

---

## Strengths for AI Agents

1. **MCP support** - Purpose-built AI integration
2. **Symbol search** - Fuzzy workspace-wide search
3. **References** - Find all uses across codebase
4. **Diagnostics** - Type checking and static analysis
5. **Battle-tested** - Stable, well-maintained

---

## Weaknesses for AI Agents

### 1. Call Graph Limitations
- `call_hierarchy` only shows direct callers/callees
- No transitive call tree (N levels deep)
- Excludes dynamic calls (interfaces, function values)
- No JSON output for call hierarchy

### 2. Position-Based API
- Requires exact `file:line:col` positions
- Can't query by symbol name directly (e.g., `pkg.FuncName`)
- Two-step process: search → get position → query

### 3. Output Format Issues
- Most commands lack JSON output
- Text output requires parsing
- Inconsistent formats across commands

### 4. No Batch Operations
- One query at a time
- Can't ask "all callers of all functions in package X"
- No graph traversal queries

### 5. IDE-Centric Design
- Assumes incremental, interactive use
- State tied to open files/workspace
- Not optimized for one-shot queries

### 6. Missing Capabilities
- No "what packages depend on X"
- No "show call tree to depth N"
- No impact analysis ("what breaks if I change X")
- No type hierarchy as JSON

---

## Comparison: What We Need vs What Gopls Provides

| Need | Gopls Support | Gap |
|------|---------------|-----|
| "Who calls function X?" | `call_hierarchy` | Position-based, no JSON |
| "What does X call?" | `call_hierarchy` | Position-based, no JSON |
| "Call tree N levels deep" | ❌ | Not available |
| "What implements interface Y?" | `implementation` | Position-based only |
| "Package dependencies" | ❌ | Not available |
| "Symbol by name" | `workspace_symbol` | Returns positions, then query again |
| "References to X" | `references` | Position-based, MCP has by-name |
| "Batch queries" | ❌ | Not available |
| "JSON output everywhere" | Partial | Most commands text-only |

---

## Recommendations for Wildcat

Based on this analysis, Wildcat should focus on:

### 1. Symbol-Based Queries
Accept `package.Function` or `Type.Method` directly, not file positions.

### 2. Transitive Call Graphs
Build full call trees using `go/callgraph` (CHA/RTA/VTA algorithms).

### 3. Structured JSON Output
All commands return parseable JSON by default.

### 4. Depth-Limited Traversal
`wildcat tree <func> --depth 3` - show call tree to N levels.

### 5. Batch Operations
Query multiple symbols at once, aggregate results.

### 6. Package-Level Analysis
- What packages import X?
- What does package X depend on?
- Impact of changing package X?

### 7. Interface Analysis
- What types implement interface X?
- What interfaces does type Y satisfy?

---

## Appendix: Sample Outputs

### gopls symbols
```
$ gopls symbols cmd/root.go
rootCmd Variable 7:5-7:12
Execute Function 13:6-13:13
init Function 17:6-17:10
```

### gopls references
```
$ gopls references cmd/root.go:7:5
/home/user/wildcat/cmd/root.go:14:9-16
/home/user/wildcat/cmd/root.go:18:2-9
/home/user/wildcat/cmd/version.go:24:2-9
```

### gopls call_hierarchy
```
$ gopls call_hierarchy cmd/root.go:13:6
caller[0]: ranges 10:16-23 in main.go from/to function main in main.go:9:6-10
identifier: function Execute in cmd/root.go:13:6-13
callee[0]: ranges 14:17-24 in cmd/root.go from/to function Execute in cobra/command.go:1070:19-26
```

### gopls definition -json
```json
{
  "span": {
    "uri": "file:///home/user/wildcat/cmd/root.go",
    "start": {"line": 7, "column": 5, "offset": 55},
    "end": {"line": 7, "column": 12, "offset": 62}
  },
  "description": "var rootCmd *cobra.Command"
}
```
