# Symbol Naming Patterns Audit

## The Problem

The codebase has ~55 places where names are constructed using `+ "." +` patterns. The same pattern often means different things, and it's hard to understand what output a given expression produces.

## Fields and Their Actual Values

| Field | Example Value | Description |
|-------|---------------|-------------|
| `PackageIdentifier.Name` | `golang` | Short package name |
| `PackageIdentifier.PkgPath` | `github.com/jasonmoo/wildcat/internal/golang` | Full import path |
| `PackageIdentifier.PkgShortPath` | `internal/golang` | Module-relative path |
| `Symbol.Name` | `Symbol` | Symbol name only |
| `indexedSymbol.Name` | `Symbol` or `Symbol.Signature` | May include type for methods |
| `SearchResult.Name` | `Symbol` or `Symbol.Signature` | Same as indexedSymbol.Name |

**Key confusion:** `Name` means different things:
- `Symbol.Name` = just the symbol (`LoadSymbols`)
- `PackageIdentifier.Name` = package name (`golang`)
- `indexedSymbol.Name` = search name, includes type for methods (`Symbol.Signature`)

## Pattern Categories

### 1. Display Names (Short, Human-Readable) - 28 occurrences

**Pattern:** `PackageIdentifier.Name + "." + sym.Name`
**Output:** `golang.Symbol`
**Intent:** Human-readable display in output

Locations:
- `search/search.go:185` - output Symbol field
- `deadcode/deadcode.go:239` - dead symbol name
- `tree/tree.go:156` - qualifiedSymbol for display
- `wildcat.go:132,146` - display names
- `symbol/symbol.go:228,259,379,700` - display names
- `package/package.go:193,201,213,264` - symbolKey for display

### 2. Unique Identifiers (Lookup Keys) - 16 occurrences

**Pattern:** `PackageIdentifier.PkgPath + "." + sym.Name`
**Output:** `github.com/jasonmoo/wildcat/internal/golang.Symbol`
**Intent:** Globally unique key for deduplication or lookup

Locations:
- `symbol.go:112` - SearchName() method
- `search.go:212` - fullSource for fuzzy search
- `wildcat.go:106,112` - deduplication keys
- `symbol/symbol.go:54,76,193` - candidates, qualifiedSymbol
- `tree/tree.go:424,441` - deduplication keys
- `interfaces.go:142` - type symbol lookup

### 3. Method Display Names - 5 occurrences

**Pattern:** `PackageIdentifier.Name + "." + typeName + "." + methodName`
**Output:** `golang.Symbol.Signature`
**Intent:** Display name for methods showing receiver type

Locations:
- `symbol/symbol.go:218` - methodSymbol display
- `package/package.go:218` - mSymbolKey
- `refs.go:448-450` - scope keys including methods

### 4. Containing Symbol (Reference Tracking) - 6 occurrences

**Pattern:** `pkg.Identifier.Name + "." [+ receiverType + "."] + funcName`
**Output:** `golang.LoadSymbols` or `golang.Symbol.Signature`

Locations:
- `refs.go:52-79, 245-272` - containing field
- `calls.go:114-131` - CallerName(), CalledName()

## Key Inconsistencies

### Same Variable Name, Different Values

| Location | Variable | Pattern | Output |
|----------|----------|---------|--------|
| `symbol/symbol.go:193` | `qualifiedSymbol` | `PkgPath + "." + Name` | `github.com/.../golang.Symbol` |
| `tree/tree.go:156` | `qualifiedSymbol` | `Name + "." + Name` | `golang.Symbol` |

### Package vs PackageIdentifier

Code inconsistently uses `impl.Package.Name` vs `impl.PackageIdentifier.Name` for the same values.

## Existing Helper

Only `SearchName()` exists (returns `PkgPath.Name`).

## Proposed Helpers

```go
// DisplayName: "golang.Symbol"
func (ps *Symbol) DisplayName() string

// QualifiedDisplayName: "golang.Symbol.Signature" for methods
func (ps *Symbol) QualifiedDisplayName() string
```

## Summary

| Pattern | Count | Use |
|---------|-------|-----|
| `Name + "." + Name` | 28 | Display |
| `PkgPath + "." + Name` | 16 | Keys |
| `Name + "." + Type + "." + Method` | 5 | Method display |
| `Name + "." prefix` | 6 | Containing |
