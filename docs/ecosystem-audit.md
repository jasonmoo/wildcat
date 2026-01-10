# Go Analysis Ecosystem Audit

## Overview

This document audits the Go ecosystem packages available for building Wildcat. We evaluate what to leverage vs build from scratch, following our CLAUDE.md principles: "Composability first" and "Simple over clever".

**Key constraint**: We use packages as libraries only. No shelling out to external tools.

---

## Package Matrix

### Standard Library (go/*)

| Package | Purpose | Our Use Case | Verdict |
|---------|---------|--------------|---------|
| `go/ast` | AST node types and traversal | Read syntax trees from packages | **Use directly** |
| `go/parser` | Parse source files to AST | Handled by go/packages | Skip (use go/packages) |
| `go/types` | Type checking, type info | Core for symbol resolution, references | **Use directly** |
| `go/token` | Positions, file sets | Convert positions to file:line | **Use directly** |
| `go/build` | Package discovery (legacy) | Superseded by go/packages | Skip |
| `go/format` | Source formatting | Not needed | Skip |
| `go/printer` | AST printing | Snippet extraction | Maybe |
| `go/constant` | Constant values | Not needed | Skip |

### golang.org/x/tools/go/*

| Package | Purpose | Our Use Case | Verdict |
|---------|---------|--------------|---------|
| `go/packages` | Modern package loading | **Core foundation** - load packages with types/AST | **Use directly** |
| `go/ssa` | SSA form construction | Required for call graph | **Use directly** |
| `go/ssa/ssautil` | SSA utilities | Helper for building SSA from packages | **Use directly** |
| `go/callgraph` | Call graph types (Graph, Node, Edge) | Core data structure | **Use directly** |
| `go/callgraph/cha` | Class Hierarchy Analysis | Fast, less precise call graph | **Use directly** |
| `go/callgraph/rta` | Rapid Type Analysis | Balanced speed/precision (default) | **Use directly** |
| `go/callgraph/vta` | Variable Type Analysis | Precise, slower call graph | **Use directly** |
| `go/callgraph/static` | Static call graph | Very fast, imprecise | Maybe |
| `go/types/typeutil` | Type utilities (Map, MethodSetCache) | Type comparison, caching | **Use directly** |
| `go/ast/astutil` | AST utilities | PathEnclosingInterval for snippets | **Use directly** |
| `go/analysis` | Analysis framework | For linters, not our use case | Skip |
| `go/pointer` | Pointer analysis | Overkill for our needs | Skip |

### gopls Internals

| Package | Purpose | Verdict |
|---------|---------|---------|
| `gopls/internal/*` | All gopls implementation | **Cannot use** - internal packages |

gopls internals are explicitly internal and cannot be imported. However, we can learn from gopls patterns by reading source code.

### Third-Party Options

| Package | Purpose | Verdict |
|---------|---------|---------|
| `github.com/go-toolsmith/*` | AST utilities | Evaluate if stdlib insufficient |

Recommendation: Start with stdlib/x/tools. Only add third-party if we hit gaps.

---

## Build vs Leverage Decisions

### Use Directly (No Wrapping)

These packages are well-designed and we use their types directly:

```go
import (
    "go/ast"
    "go/token"
    "go/types"
    "golang.org/x/tools/go/packages"
    "golang.org/x/tools/go/ssa"
    "golang.org/x/tools/go/callgraph"
    "golang.org/x/tools/go/callgraph/rta"
)
```

### Wrap/Extend

These need thin wrappers for our use case:

| Package | Wrapper Purpose |
|---------|-----------------|
| `go/packages` | Simplify config, scope control (module/deps/all) |
| `go/callgraph/*` | Unified interface for CHA/RTA/VTA selection |
| `go/types.Info` | Reference finding (iterate Uses map) |

### Build From Scratch

| Component | Why Build |
|-----------|-----------|
| Symbol parsing | Parse `pkg.Func`, `Type.Method` input formats |
| Symbol resolution | Map parsed symbol to `types.Object` |
| Fuzzy matching | Suggest similar symbols on error |
| Snippet extraction | Extract source lines with context |
| JSON output types | Our specific output format |
| Error types | Structured errors with suggestions |

---

## Integration Strategy

### Recommended Package Stack

```
┌─────────────────────────────────────────────────────────┐
│                    CLI Commands                          │
│              (callers, callees, tree, refs, impact)      │
├─────────────────────────────────────────────────────────┤
│                   Wildcat Internal                       │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────────┐│
│  │   symbols   │ │  callgraph  │ │       output        ││
│  │  (resolve)  │ │  (wrapper)  │ │   (JSON, snippets)  ││
│  └─────────────┘ └─────────────┘ └─────────────────────┘│
├─────────────────────────────────────────────────────────┤
│                   x/tools/go/*                           │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────────┐│
│  │  packages   │ │     ssa     │ │     callgraph/*     ││
│  └─────────────┘ └─────────────┘ └─────────────────────┘│
├─────────────────────────────────────────────────────────┤
│                    Standard Library                      │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────────┐│
│  │   go/ast    │ │  go/types   │ │      go/token       ││
│  └─────────────┘ └─────────────┘ └─────────────────────┘│
└─────────────────────────────────────────────────────────┘
```

### How Packages Compose

#### 1. Package Loading Flow

```go
// Load packages with full type info and syntax
cfg := &packages.Config{
    Mode: packages.LoadAllSyntax,
    Dir:  workDir,
}
pkgs, err := packages.Load(cfg, "./...")

// Each pkg has:
// - pkg.Types      (*types.Package)
// - pkg.TypesInfo  (*types.Info) - has Uses/Defs maps
// - pkg.Syntax     ([]*ast.File)
// - pkg.Fset       (*token.FileSet)
```

#### 2. SSA Construction Flow

```go
// Build SSA from loaded packages
prog, ssaPkgs := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
prog.Build()

// Now we have SSA functions for call graph
```

#### 3. Call Graph Construction Flow

```go
// Pick algorithm based on flag
var cg *callgraph.Graph
switch algorithm {
case "cha":
    cg = cha.CallGraph(prog)
case "rta":
    roots := ssautil.MainPackages(ssaPkgs)
    result := rta.Analyze(roots, true)
    cg = result.CallGraph
case "vta":
    funcs := ssautil.AllFunctions(prog)
    cg = vta.CallGraph(funcs, cha.CallGraph(prog))
}

// Query the graph
for _, edge := range cg.Nodes[targetFunc].In {
    caller := edge.Caller.Func
    callSite := edge.Site
    // Extract position, build result
}
```

#### 4. Reference Finding Flow

```go
// Find references using types.Info.Uses
for _, pkg := range pkgs {
    for ident, obj := range pkg.TypesInfo.Uses {
        if obj == targetObject {
            pos := pkg.Fset.Position(ident.Pos())
            // Found a reference at pos
        }
    }
}
```

#### 5. Snippet Extraction Flow

```go
// Get source lines around a position
func extractSnippet(fset *token.FileSet, pos token.Position, context int) string {
    // Read file
    content, _ := os.ReadFile(pos.Filename)
    lines := strings.Split(string(content), "\n")

    // Extract lines around pos.Line
    start := max(0, pos.Line - context - 1)
    end := min(len(lines), pos.Line + context)

    return strings.Join(lines[start:end], "\n")
}
```

---

## Key Code Patterns

### Loading Packages with Scope Control

```go
type LoadScope int

const (
    ScopeModule LoadScope = iota  // Current module only
    ScopeDeps                      // + direct dependencies
    ScopeAll                       // + transitive dependencies
)

func LoadPackages(dir string, scope LoadScope) ([]*packages.Package, error) {
    mode := packages.NeedName | packages.NeedFiles |
            packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo

    if scope >= ScopeDeps {
        mode |= packages.NeedImports | packages.NeedDeps
    }

    cfg := &packages.Config{
        Mode: mode,
        Dir:  dir,
    }

    return packages.Load(cfg, "./...")
}
```

### Symbol Resolution

```go
func ResolveSymbol(pkgs []*packages.Package, name string) (types.Object, error) {
    // Parse: "pkg.Func" or "Type.Method"
    parts := strings.Split(name, ".")

    for _, pkg := range pkgs {
        // Check package-level scope
        obj := pkg.Types.Scope().Lookup(parts[0])
        if obj != nil {
            if len(parts) == 1 {
                return obj, nil
            }
            // Handle method lookup on type
            if named, ok := obj.Type().(*types.Named); ok {
                for i := 0; i < named.NumMethods(); i++ {
                    m := named.Method(i)
                    if m.Name() == parts[1] {
                        return m, nil
                    }
                }
            }
        }
    }
    return nil, fmt.Errorf("symbol not found: %s", name)
}
```

### Call Graph Querying

```go
func FindCallers(cg *callgraph.Graph, fn *ssa.Function) []*callgraph.Edge {
    node := cg.Nodes[fn]
    if node == nil {
        return nil
    }
    return node.In  // All incoming edges = callers
}

func FindCallees(cg *callgraph.Graph, fn *ssa.Function) []*callgraph.Edge {
    node := cg.Nodes[fn]
    if node == nil {
        return nil
    }
    return node.Out  // All outgoing edges = callees
}
```

---

## Risk Assessment

### API Stability

| Package | Stability | Risk |
|---------|-----------|------|
| `go/ast`, `go/types`, `go/token` | Stable (stdlib) | Low |
| `golang.org/x/tools/go/packages` | Pre-v1 (v0.x) | Medium - known issues documented |
| `golang.org/x/tools/go/ssa` | Pre-v1 | Medium - API may change |
| `golang.org/x/tools/go/callgraph` | Pre-v1 | Medium - API may change |

**Mitigation**: Pin x/tools version in go.mod. Monitor for breaking changes.

### Performance Considerations

| Algorithm | Speed | Memory | Best For |
|-----------|-------|--------|----------|
| CHA | Fast | Low | Quick exploration, large codebases |
| RTA | Medium | Medium | General use (recommended default) |
| VTA | Slow | High | Precise refactoring |

**Mitigation**: Default to RTA, allow algorithm selection via flag.

### Known Issues

1. **go/packages LoadMode interactions** - Some mode combinations have bugs
   - Mitigation: Use LoadAllSyntax for simplicity

2. **Type identity across Load calls** - Can't mix objects from different loads
   - Mitigation: Load once, reuse

3. **Generics in SSA** - Need `ssa.InstantiateGenerics` mode
   - Mitigation: Always use this mode

---

## Recommendations

### Phase 1: Core Infrastructure

1. **Use `go/packages`** with `LoadAllSyntax` mode
2. **Use `go/ssa` + `ssautil`** for SSA construction
3. **Default to `rta`** for call graph, support all three algorithms
4. **Use `types.Info.Uses`** for reference finding

### Phase 2: Our Code

1. **Build symbol parser** - Parse `pkg.Func`, `Type.Method` formats
2. **Build symbol resolver** - Map to `types.Object` with fuzzy suggestions
3. **Build snippet extractor** - Read source lines with context
4. **Build output types** - JSON structures per product-design.md
5. **Build error types** - Structured errors with suggestions

### Dependency List

```go
// go.mod additions
require (
    golang.org/x/tools v0.20.0  // or latest stable
)
```

Core imports:
```go
import (
    "go/ast"
    "go/token"
    "go/types"

    "golang.org/x/tools/go/packages"
    "golang.org/x/tools/go/ssa"
    "golang.org/x/tools/go/ssa/ssautil"
    "golang.org/x/tools/go/callgraph"
    "golang.org/x/tools/go/callgraph/cha"
    "golang.org/x/tools/go/callgraph/rta"
    "golang.org/x/tools/go/callgraph/vta"
    "golang.org/x/tools/go/ast/astutil"
    "golang.org/x/tools/go/types/typeutil"
)
```

---

## Summary

| Category | Packages | Count |
|----------|----------|-------|
| Use directly | go/ast, go/types, go/token, go/packages, go/ssa, go/callgraph/* | 10+ |
| Wrap lightly | go/packages (scope), callgraph (algorithm selection) | 2 |
| Build from scratch | Symbol parsing, resolution, suggestions, snippets, output, errors | 6 |
| Skip | go/analysis, go/pointer, gopls internals, go/build | 4+ |

The Go ecosystem provides excellent foundations. We leverage battle-tested packages for the hard parts (type checking, SSA, call graphs) and build only what's specific to our AI-optimized output format.
