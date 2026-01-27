# Path to AST Mapping

This document maps path syntax to go/ast node types for implementation reference.

---

## Declaration-Level Paths

| Path Pattern | AST Type | Key Fields |
|--------------|----------|------------|
| `pkg.Func` | `*ast.FuncDecl` | `Name`, `Type`, `Body`, `Doc` |
| `pkg.Type` | `*ast.TypeSpec` (via `*ast.GenDecl`) | `Name`, `Type`, `TypeParams`, `Doc` |
| `pkg.Const` | `*ast.ValueSpec` (via `*ast.GenDecl`, Tok=CONST) | `Names`, `Values`, `Type`, `Doc` |
| `pkg.Var` | `*ast.ValueSpec` (via `*ast.GenDecl`, Tok=VAR) | `Names`, `Values`, `Type`, `Doc` |
| `pkg.Type.Method` | `*ast.FuncDecl` (with `Recv != nil`) | `Recv`, `Name`, `Type`, `Body`, `Doc` |

---

## Type Component Paths

### Struct Fields

| Path | AST Navigation |
|------|----------------|
| `pkg.Struct/fields[Name]` | `TypeSpec.Type.(*ast.StructType).Fields.List[i]` where `Fields.List[i].Names[j].Name == "Name"` |
| `pkg.Struct/fields[0]` | `TypeSpec.Type.(*ast.StructType).Fields.List[0]` |

**AST Type:** `*ast.Field`
- `Names []*ast.Ident` - field names (nil for embedded)
- `Type ast.Expr` - field type
- `Tag *ast.BasicLit` - struct tag (or nil)
- `Doc *ast.CommentGroup` - doc comment
- `Comment *ast.CommentGroup` - line comment

### Embedded Types

| Path | AST Navigation |
|------|----------------|
| `pkg.Struct/embeds[TypeName]` | `StructType.Fields.List[i]` where `Names == nil` and type matches |

**Detection:** A field with `Names == nil` is embedded. Extract type name from `Type` expression.

### Interface Methods

| Path | AST Navigation |
|------|----------------|
| `pkg.Interface.Method` | `TypeSpec.Type.(*ast.InterfaceType).Methods.List[i]` where `Names[0].Name == "Method"` |
| `pkg.Interface/methods[0]` | `InterfaceType.Methods.List[0]` |

**AST Type:** `*ast.Field` (same as struct field)
- In interfaces, `Names` contains method name
- `Type` is `*ast.FuncType` for method signatures
- `Names == nil` for embedded interfaces

---

## Signature Component Paths

### Parameters

| Path | AST Navigation |
|------|----------------|
| `pkg.Func/params[name]` | `FuncDecl.Type.Params.List[i]` where `Names[j].Name == "name"` |
| `pkg.Func/params[0]` | First parameter (considering grouped params) |

**Complexity:** Go groups parameters by type:
```go
func Foo(a, b int, c string)
// Params.List has 2 elements:
//   [0]: Names=["a","b"], Type=int
//   [1]: Names=["c"], Type=string
```

To get "the Nth parameter" requires flattening:
```go
func flattenParams(fl *ast.FieldList) []*ParamInfo {
    var result []*ParamInfo
    for _, field := range fl.List {
        if len(field.Names) == 0 {
            // Unnamed parameter
            result = append(result, &ParamInfo{Field: field})
        } else {
            for _, name := range field.Names {
                result = append(result, &ParamInfo{Name: name, Field: field})
            }
        }
    }
    return result
}
```

### Return Values

| Path | AST Navigation |
|------|----------------|
| `pkg.Func/returns[name]` | `FuncDecl.Type.Results.List[i]` where `Names[j].Name == "name"` |
| `pkg.Func/returns[0]` | First return value (same flattening as params) |

**Note:** `Results` can be nil if function has no return values.

### Receiver

| Path | AST Navigation |
|------|----------------|
| `pkg.Type.Method/receiver` | `FuncDecl.Recv.List[0]` |

**AST Type:** `*ast.Field`
- Always exactly one element in `Recv.List` for methods
- `Names[0]` is receiver name
- `Type` is receiver type (may be `*ast.StarExpr` for pointer receiver)

### Type Parameters (Generics)

| Path | AST Navigation |
|------|----------------|
| `pkg.Func/typeparams[T]` | `FuncDecl.Type.TypeParams.List[i]` where `Names[j].Name == "T"` |
| `pkg.Type/typeparams[T]` | `TypeSpec.TypeParams.List[i]` where `Names[j].Name == "T"` |

**AST Type:** `*ast.Field`
- `Names` contains type parameter names
- `Type` is the constraint (may be nil for `any`)

---

## Documentation Paths

| Path | AST Navigation |
|------|----------------|
| `pkg.Func/doc` | `FuncDecl.Doc` |
| `pkg.Type/doc` | `TypeSpec.Doc` (or parent GenDecl.Doc) |
| `pkg.Const/doc` | `ValueSpec.Doc` (or parent GenDecl.Doc) |
| `pkg.Type/fields[Name]/doc` | `Field.Doc` |

**AST Type:** `*ast.CommentGroup`
- `List []*ast.Comment` - individual comment lines
- Each `Comment` has `Text` field with `//` or `/*` prefix

**Complexity:** For grouped declarations:
```go
// Package-level doc (on GenDecl)
const (
    // A doc (on ValueSpec)
    A = 1
    // B doc
    B = 2
)
```

The doc may be on `GenDecl.Doc` or `ValueSpec.Doc` depending on structure.

---

## Struct Tag Paths

| Path | AST Navigation |
|------|----------------|
| `pkg.Type/fields[Name]/tag` | `Field.Tag.Value` |
| `pkg.Type/fields[Name]/tag[json]` | Parse tag, extract `json` key |

**AST Type:** `*ast.BasicLit` with `Kind == token.STRING`
- `Value` includes backticks: `` `json:"name"` ``
- Parse with `reflect.StructTag` after stripping backticks

```go
func getTagValue(field *ast.Field, key string) string {
    if field.Tag == nil {
        return ""
    }
    // Remove backticks
    raw := strings.Trim(field.Tag.Value, "`")
    tag := reflect.StructTag(raw)
    return tag.Get(key)
}
```

---

## Body Paths

| Path | AST Navigation |
|------|----------------|
| `pkg.Func/body` | `FuncDecl.Body` |
| `pkg.Type.Method/body` | `FuncDecl.Body` |

**AST Type:** `*ast.BlockStmt`
- `List []ast.Stmt` - statements in body
- `Lbrace`, `Rbrace` - positions of braces

**For V1:** Treat body as atomic. Full body replacement only.

---

## Import Paths (Deferred)

| Path | AST Navigation |
|------|----------------|
| `pkg/imports[fmt]` | `File.Imports[i]` where name matches |

**Complexity:**
- Imports are file-scoped, not package-scoped
- Name can be: explicit alias, inferred from path, `.`, `_`
- Multiple files can import same package differently

---

## Implementation Helpers

### Resolving a Path

```go
type PathResolver struct {
    pkg *packages.Package
}

func (r *PathResolver) Resolve(path string) (ast.Node, error) {
    parts := parsePath(path)

    // 1. Find package
    // 2. Find symbol in package
    symbol := r.findSymbol(parts.Package, parts.Symbol)

    // 3. Navigate sub-paths
    node := symbol
    for _, sub := range parts.SubPaths {
        node = r.navigateSubPath(node, sub)
    }

    return node, nil
}

func (r *PathResolver) navigateSubPath(node ast.Node, sub SubPath) ast.Node {
    switch sub.Category {
    case "params":
        fd := node.(*ast.FuncDecl)
        return r.selectFromFieldList(fd.Type.Params, sub.Selector)
    case "returns":
        fd := node.(*ast.FuncDecl)
        return r.selectFromFieldList(fd.Type.Results, sub.Selector)
    case "fields":
        ts := node.(*ast.TypeSpec)
        st := ts.Type.(*ast.StructType)
        return r.selectFromFieldList(st.Fields, sub.Selector)
    case "body":
        fd := node.(*ast.FuncDecl)
        return fd.Body
    case "doc":
        return r.getDocComment(node)
    case "tag":
        f := node.(*ast.Field)
        return f.Tag
    // ... etc
    }
}
```

### Extracting Source for a Node

```go
func extractSource(fset *token.FileSet, node ast.Node, src []byte) string {
    start := fset.Position(node.Pos())
    end := fset.Position(node.End())
    return string(src[start.Offset:end.Offset])
}
```

### Byte-Accurate Positions

The `token.Pos` values from AST nodes map directly to byte offsets:
- `node.Pos()` - start position
- `node.End()` - end position (exclusive)
- Convert via `fset.Position(pos).Offset`

---

## Edge Cases

### Grouped Parameters

```go
func Foo(a, b int)
// /params[0] → "a int" (first param)
// /params[1] → "b int" (second param)
// /params[a] → "a int"
// /params[b] → "b int"
```

Must flatten `FieldList` for positional access.

### Unnamed Parameters

```go
func Read([]byte) (int, error)
// /params[0] → "[]byte" (no name)
// /params[data] → error (no such name)
```

Named access fails; only positional works.

### Anonymous Embedded Fields

```go
type Foo struct {
    *Bar       // embedded
    io.Reader  // embedded interface
}
// /embeds[Bar] → "*Bar"
// /embeds[Reader] → "io.Reader"
// /fields[0] → "*Bar" (also works)
```

### Method on Pointer vs Value

```go
func (p Point) Foo()   // value receiver
func (p *Point) Bar()  // pointer receiver
```

Both are `pkg.Point.Foo` and `pkg.Point.Bar`. The receiver type is accessible via `/receiver`.

### Multiple Init Functions

```go
// file1.go
func init() { ... }

// file2.go
func init() { ... }
func init() { ... }
```

Need file-qualified or positional addressing: `pkg.init[0]`, `pkg.init[1]`, etc.
Or: `pkg/file1.go.init`, `pkg/file2.go.init[0]`, `pkg/file2.go.init[1]`
