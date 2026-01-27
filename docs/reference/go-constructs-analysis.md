# Go Language Constructs: Path Addressability Analysis

This document systematically analyzes every Go language construct from the spec to determine what can and should be addressable via paths.

Reference: [Go 1.25 Spec](go-spec-1.25.md)

---

## Design Principle: Canonical vs Accepted Forms

The AI agent is the primary user of this path system. AIs guess based on patterns they've seen, and rigid syntax creates friction. Therefore:

**Multiple input forms, unambiguous resolution, canonical output.**

- **Canonical form**: What wildcat generates/outputs. Consistent, predictable, learnable.
- **Accepted forms**: What wildcat parses/accepts. Flexible, forgiving of reasonable guesses.

The spec defines both. All accepted forms for an element resolve to the same AST node. Wildcat always outputs canonical form.

### Example: Interface Methods

```
# All resolve to the same entity:
pkg.Reader.Read           # canonical (dot notation)
pkg.Reader/methods[Read]  # accepted (slash + name)
pkg.Reader/methods[0]     # accepted (positional)

# Wildcat outputs:
pkg.Reader.Read
```

### Why This Matters

The AI learns from wildcat's output (canonical form). But we don't punish reasonable guesses based on:
- Go syntax patterns (`Type.Method`)
- AST structure patterns (`/methods[Name]`)
- Positional patterns (`/methods[0]`)

The flexibility also provides precision when needed - slashes disambiguate when dots would be ambiguous.

---

## Notation Rules

**Dots (`.`) - Methods only:**
- Concrete methods: `pkg.Type.Method`
- Interface methods: `pkg.Interface.Method`
- Methods are "named members with behavior" in Go's method set sense

**Slashes (`/`) - Structural components:**
- Fields: `pkg.Type/fields[Name]`
- Parameters: `pkg.Func/params[name]`
- Returns: `pkg.Func/returns[0]`
- Embedded types: `pkg.Type/embeds[Type]`
- Body, doc, tag, etc.

**Why this distinction:**
- Explored using dots for fields (`.Field`) - discarded due to complexity in reasoning about outputs
- Methods are the only dot-accessible sub-elements of types
- Everything else uses slash notation for structural access

---

## Analysis Framework

For each construct, we evaluate:
1. **Is it addressable?** - Can we uniquely identify it?
2. **How is it named?** - Named, positional, or anonymous?
3. **What contains it?** - Its parent in the hierarchy
4. **What can it contain?** - Its children
5. **Path recommendation** - Proposed path syntax (canonical form)
6. **Accepted alternatives** - Other forms that resolve to same entity

---

## 1. Package Level

### Package

| Property | Value |
|----------|-------|
| Addressable | Yes |
| Named | Yes (import path) |
| Container | Module / workspace |
| Contains | Imports, declarations (const, var, type, func) |
| Path | `package/path` |

**Examples:**
```
golang                           # local package
io                               # stdlib package
github.com/user/repo/pkg         # external package
```

**Notes:** Package is the root of all symbol paths. The package path itself is the identity.

---

### Import

| Property | Value |
|----------|-------|
| Addressable | Yes |
| Named | By alias or inferred name |
| Container | Package (file-scoped) |
| Contains | Nothing (reference only) |
| Path | `pkg/imports[name]` or `pkg/imports[0]` |

**Examples:**
```go
import "fmt"                     // pkg/imports[fmt]
import f "fmt"                   // pkg/imports[f]
import . "fmt"                   // pkg/imports[.]
import _ "image/png"             // pkg/imports[_] (ambiguous if multiple)
```

**Challenges:**
- Imports are file-scoped, not package-scoped
- Multiple files can have same import with different aliases
- Blank imports (`_`) have no unique name
- Dot imports (`.`) are special

**Recommendation:** Consider `pkg/file.go/imports[name]` for file-scoped precision, or defer import addressing entirely (low value for AI editing use case).

---

## 2. Declarations

### Constant Declaration

| Property | Value |
|----------|-------|
| Addressable | Yes |
| Named | Yes |
| Container | Package |
| Contains | Value expression, optional type |
| Path | `pkg.ConstName` |

**Examples:**
```go
const Pi = 3.14159              // pkg.Pi
const (
    A = iota                    // pkg.A
    B                           // pkg.B
    C                           // pkg.C
)
```

**Sub-elements:**
- `/value` - the constant expression
- `/doc` - godoc comment

**Notes:** Constants in a const block share an implicit relationship (iota), but each is independently addressable.

---

### Variable Declaration

| Property | Value |
|----------|-------|
| Addressable | Yes |
| Named | Yes |
| Container | Package (or function body for locals) |
| Contains | Type, optional initializer |
| Path | `pkg.VarName` |

**Examples:**
```go
var ErrNotFound = errors.New("not found")   // pkg.ErrNotFound
var (
    mu    sync.Mutex            // pkg.mu
    cache map[string]string     // pkg.cache
)
```

**Sub-elements:**
- `/type` - declared type (if explicit)
- `/value` - initializer expression
- `/doc` - godoc comment

---

### Type Declaration

| Property | Value |
|----------|-------|
| Addressable | Yes |
| Named | Yes |
| Container | Package |
| Contains | Underlying type definition |
| Path | `pkg.TypeName` |

**Types of type declarations:**
1. **Type definition** - creates new type: `type Point struct{...}`
2. **Type alias** - creates alias: `type Byte = uint8`

**Sub-elements (struct):**
- `/fields[Name]` or `/fields[0]` - struct fields
- `/embeds[TypeName]` - embedded types
- `.Method` - methods (dot notation - methods are the only dot-accessible sub-elements)

**Sub-elements (interface):**
- `.Method` - interface methods (canonical)
- `/methods[Name]` or `/methods[0]` - interface methods (accepted alternative)
- `/embeds[InterfaceName]` - embedded interfaces
- `/types` - type constraints (Go 1.18+)

**Sub-elements (all types):**
- `/doc` - godoc comment
- `/typeparams[T]` - type parameters (generics)
- `/underlying` - underlying type (for analysis, not editing)

---

### Function Declaration

| Property | Value |
|----------|-------|
| Addressable | Yes |
| Named | Yes |
| Container | Package |
| Contains | Signature, body |
| Path | `pkg.FuncName` |

**Examples:**
```go
func Connect(addr string) (*Conn, error)    // pkg.Connect
func min[T ~int](a, b T) T                  // pkg.min
```

**Sub-elements:**
- `/params[name]` or `/params[0]` - parameters
- `/returns[name]` or `/returns[0]` - return values
- `/body` - function body
- `/doc` - godoc comment
- `/typeparams[T]` - type parameters
- `/receiver` - N/A for functions (methods only)

---

### Method Declaration

| Property | Value |
|----------|-------|
| Addressable | Yes |
| Named | Yes (combined with receiver type) |
| Container | Type (conceptually) |
| Contains | Receiver, signature, body |
| Path | `pkg.Type.Method` |

**Examples:**
```go
func (p *Point) Distance(q Point) float64   // pkg.Point.Distance
func (s Set[T]) Contains(v T) bool          // pkg.Set.Contains
```

**Sub-elements:**
- `/receiver` - the receiver parameter
- `/params[name]` or `/params[0]` - parameters
- `/returns[name]` or `/returns[0]` - return values
- `/body` - method body
- `/doc` - godoc comment
- `/typeparams[T]` - type parameters (on receiver type)

**Note:** Methods use dot notation because they're named members of a type, consistent with Go's own syntax (`point.Distance()`).

---

## 3. Type Components

### Struct Field

| Property | Value |
|----------|-------|
| Addressable | Yes |
| Named | Usually (can be anonymous/embedded) |
| Container | Struct type |
| Contains | Type, optional tag |
| Path | `pkg.Struct/fields[Name]` or `pkg.Struct/fields[0]` |

**Examples:**
```go
type User struct {
    ID        int64             // pkg.User/fields[ID]
    Name      string            // pkg.User/fields[Name]
    Email     string `json:"email"` // pkg.User/fields[Email]
    io.Reader                   // pkg.User/embeds[Reader] (embedded)
    *log.Logger                 // pkg.User/embeds[Logger] (embedded pointer)
}
```

**Sub-elements:**
- `/type` - field type
- `/tag` - struct tag (raw backtick string)
- `/tag[json]` - specific tag key
- `/doc` - field comment

**Embedded fields:** Use `/embeds[TypeName]` since they have special semantics (field promotion).

---

### Struct Tag

| Property | Value |
|----------|-------|
| Addressable | Yes |
| Named | By field |
| Container | Struct field |
| Contains | Key-value pairs |
| Path | `pkg.Struct/fields[Name]/tag` |

**Examples:**
```go
Name string `json:"name" db:"user_name" validate:"required"`
```

**Paths:**
- `pkg.User/fields[Name]/tag` - entire tag: `` `json:"name" db:"user_name"` ``
- `pkg.User/fields[Name]/tag[json]` - just json value: `"name"`
- `pkg.User/fields[Name]/tag[db]` - just db value: `"user_name"`

---

### Interface Method

| Property | Value |
|----------|-------|
| Addressable | Yes |
| Named | Yes |
| Container | Interface type |
| Contains | Signature only (no body) |
| Canonical | `pkg.Interface.Method` |
| Accepted | `pkg.Interface/methods[Name]`, `pkg.Interface/methods[0]` |

**Examples:**
```go
type Reader interface {
    Read(p []byte) (n int, err error)
}
```

**Path forms (all resolve to same method):**
```
pkg.Reader.Read           # canonical - matches Go syntax
pkg.Reader/methods[Read]  # accepted - explicit category
pkg.Reader/methods[0]     # accepted - positional
```

**Sub-elements:**
- `/params[name]` or `/params[0]`
- `/returns[name]` or `/returns[0]`
- `/doc`

**Note:** Interface methods use dot notation (canonical) because they're methods - consistent with concrete methods. The `/methods[]` form is accepted for AST-style thinking or positional access.

---

### Interface Embed

| Property | Value |
|----------|-------|
| Addressable | Yes |
| Named | By embedded type |
| Container | Interface type |
| Contains | Reference to another interface |
| Path | `pkg.Interface/embeds[EmbeddedType]` |

**Examples:**
```go
type ReadWriter interface {
    Reader                       // pkg.ReadWriter/embeds[Reader]
    Writer                       // pkg.ReadWriter/embeds[Writer]
}
```

---

### Interface Type Constraint (Go 1.18+)

| Property | Value |
|----------|-------|
| Addressable | Partially |
| Named | No (type expressions) |
| Container | Interface type |
| Contains | Type expressions |
| Path | `pkg.Interface/types` |

**Examples:**
```go
type Numeric interface {
    ~int | ~float64              // pkg.Numeric/types
}
```

**Note:** Type constraints are expressions, not named elements. Addressing individual type terms is complex and probably low-value.

---

## 4. Function Signature Components

### Parameter

| Property | Value |
|----------|-------|
| Addressable | Yes |
| Named | Usually (can be unnamed) |
| Container | Function/method signature |
| Contains | Name, type |
| Path | `pkg.Func/params[name]` or `pkg.Func/params[0]` |

**Examples:**
```go
func Process(ctx context.Context, data []byte, opts ...Option)
// pkg.Process/params[ctx]
// pkg.Process/params[data]
// pkg.Process/params[opts]
// pkg.Process/params[0], /params[1], /params[2]
```

**Unnamed parameters:**
```go
func Write([]byte) (int, error)
// pkg.Write/params[0] (must use positional)
```

**Sub-elements:**
- `/type` - parameter type
- `/variadic` - boolean flag? Or just check type starts with `...`?

---

### Return Value

| Property | Value |
|----------|-------|
| Addressable | Yes |
| Named | Sometimes |
| Container | Function/method signature |
| Contains | Optional name, type |
| Path | `pkg.Func/returns[name]` or `pkg.Func/returns[0]` |

**Examples:**
```go
func Split(s string) (head, tail string)
// pkg.Split/returns[head]
// pkg.Split/returns[tail]
// pkg.Split/returns[0], /returns[1]

func Read(p []byte) (int, error)
// pkg.Read/returns[0]
// pkg.Read/returns[1]
```

---

### Receiver

| Property | Value |
|----------|-------|
| Addressable | Yes |
| Named | Yes (always named) |
| Container | Method signature |
| Contains | Name, type |
| Path | `pkg.Type.Method/receiver` |

**Examples:**
```go
func (p *Point) Scale(factor float64)
// pkg.Point.Scale/receiver → "p *Point"
// pkg.Point.Scale/receiver/name → "p"
// pkg.Point.Scale/receiver/type → "*Point"
```

---

### Type Parameter (Generics)

| Property | Value |
|----------|-------|
| Addressable | Yes |
| Named | Yes |
| Container | Function or type definition |
| Contains | Name, constraint |
| Path | `pkg.Func/typeparams[T]` or `pkg.Type/typeparams[T]` |

**Examples:**
```go
func min[T ~int | ~float64](a, b T) T
// pkg.min/typeparams[T]
// pkg.min/typeparams[T]/constraint → "~int | ~float64"

type Set[E comparable] map[E]struct{}
// pkg.Set/typeparams[E]
// pkg.Set/typeparams[E]/constraint → "comparable"
```

---

## 5. Documentation

### Doc Comment

| Property | Value |
|----------|-------|
| Addressable | Yes |
| Named | N/A (attached to declaration) |
| Container | Any declaration |
| Contains | Comment text |
| Path | `<symbol>/doc` |

**Examples:**
```
pkg.Connect/doc              // function doc
pkg.User/doc                 // type doc
pkg.User/fields[Name]/doc    // field doc
pkg.User.Validate/doc        // method doc
```

**Note:** Godoc comments are the `//` comments immediately preceding a declaration, with no blank line.

---

## 6. Function Body (Deferred - Separate Ticket)

Body internals require special treatment. Flagging for completeness:

### Local Variables
```go
func Foo() {
    x := 42                      // pkg.Foo/body/vars[x]? Or content-based?
}
```

### Labels
```go
func Foo() {
Retry:                           // pkg.Foo/body/labels[Retry]
    // ...
}
```

### Statements
```go
func Foo() {
    if x > 0 { ... }             // pkg.Foo/body/stmts[0]? Fragile.
}
```

### Anonymous Functions
```go
func Foo() {
    fn := func() { ... }         // pkg.Foo/body/vars[fn]? Complex.
}
```

**Recommendation:** Defer all body-internal addressing to separate ticket. Initial implementation uses `/body` as atomic unit.

---

## 7. Special Cases

### Init Functions

| Property | Value |
|----------|-------|
| Addressable | Partially |
| Named | All named `init` |
| Container | Package |
| Contains | Body only (no params/returns) |
| Path | `pkg.init` or `pkg.init[0]`, `pkg.init[1]` |

**Challenge:** Multiple `init` functions can exist in a package, even in the same file.

**Options:**
1. `pkg.init` - only works if there's exactly one
2. `pkg.init[0]` - positional by declaration order
3. `pkg/file.go.init` - file-qualified
4. `pkg/file.go.init[0]` - file + position for multiple per file

---

### Blank Identifier

```go
var _ Interface = (*Impl)(nil)   // Type assertion check
import _ "image/png"             // Side-effect import
_, err := f()                    // Ignored value
```

**Addressable:** No. By definition, blank identifier discards identity.

---

### Anonymous Struct Fields (Embedding)

```go
type Wrapper struct {
    *bytes.Buffer                // Embedded, addressable as /embeds[Buffer]
    struct { x, y int }          // Anonymous struct type - challenge
}
```

**Anonymous struct types:** The type literal itself has no name. Could use positional: `/fields[0]` for the anonymous struct field.

---

### Method Expressions vs Method Values

```go
Point.Distance                   // Method expression: func(Point, Point) float64
p.Distance                       // Method value: func(Point) float64 (bound to p)
```

**In paths:** We address method declarations, not expressions. `pkg.Point.Distance` refers to the method declaration.

---

### Type Assertions in Code

```go
x.(MyType)                       // Runtime assertion - not addressable as declaration
```

**Not addressable** at declaration level. These are expressions within bodies.

---

### Composite Literals

```go
Point{1, 2}                      // Creates value - not a declaration
map[string]int{"a": 1}           // Same
```

**Not addressable** at declaration level. These are expressions.

---

## 8. Comprehensive Path Grammar

Based on the analysis above, here's the proposed grammar:

```ebnf
(* Root: package + symbol + optional structural path *)
path = package_path "." symbol [ subpath ] .

(* Package identity - slashes separate path components *)
package_path = identifier { "/" identifier } .

(* Symbol identity - dots for methods ONLY *)
(* First identifier is top-level symbol (type, func, var, const) *)
(* Second identifier (if present) is method name *)
symbol = identifier [ "." identifier ] .

(* Structural navigation - slashes for all non-method components *)
subpath = { "/" category [ selector ] } .

(* Categories - structural components accessed via slash *)
category = "fields" | "methods" | "embeds"
         | "params" | "returns" | "receiver"
         | "typeparams" | "body" | "doc" | "tag" | "type"
         | "constraint" | "value" | "imports" .

(* Selection - brackets for named or positional access *)
selector = "[" ( identifier | integer ) "]" .

(* Basic tokens *)
identifier = letter { letter | digit } .
integer = digit { digit } .
letter = "a" ... "z" | "A" ... "Z" | "_" .
digit = "0" ... "9" .
```

**Grammar notes:**

1. **Dots limited to one level:** `pkg.Type.Method` is valid, `pkg.Type.Field.SubField` is not.
   Fields use `/fields[Field]/...` for deeper access.

2. **Accepted alternatives:** The grammar above describes canonical form. Parser should also accept:
   - `/methods[Name]` as alternative to `.Method` for interface methods
   - Positional selectors (`[0]`) wherever named selectors are valid

3. **Ambiguity resolution:** When parsing, resolve package path first (longest match to known package),
   then symbol identity, then structural path.

---

## 9. Summary: Addressable Elements

### Definitely Addressable (V1)

| Element | Canonical Path | Accepted Alternatives |
|---------|----------------|----------------------|
| Package | `pkg` | - |
| Constant | `pkg.Const` | - |
| Variable | `pkg.Var` | - |
| Type (struct) | `pkg.Type` | - |
| Type (interface) | `pkg.Interface` | - |
| Function | `pkg.Func` | - |
| Method (concrete) | `pkg.Type.Method` | - |
| Method (interface) | `pkg.Interface.Method` | `/methods[Name]`, `/methods[0]` |
| Struct field | `pkg.Type/fields[Name]` | `/fields[0]` (positional) |
| Parameter | `pkg.Func/params[name]` | `/params[0]` (positional) |
| Return value | `pkg.Func/returns[name]` | `/returns[0]` (positional) |
| Receiver | `pkg.Type.Method/receiver` | - |
| Type parameter | `pkg.Func/typeparams[T]` | `/typeparams[0]` (positional) |
| Doc comment | `<any>/doc` | - |
| Struct tag | `pkg.Type/fields[Name]/tag` | `/tag[key]` for specific key |
| Function body | `pkg.Func/body` | - |

### Addressable with Caveats (Consider for V1)

| Element | Path Pattern | Caveat |
|---------|--------------|--------|
| Embedded type | `pkg.Type/embeds[Type]` | Semantically special |
| Tag key | `pkg.Type/fields[F]/tag[json]` | Parsing struct tags |
| Type constraint | `pkg.Type/typeparams[T]/constraint` | Complex expressions |
| Embedded interface | `pkg.Interface/embeds[I]` | Same as struct embeds |

### Probably Defer (Separate Tickets)

| Element | Path Pattern | Why Defer |
|---------|--------------|-----------|
| Import | `pkg/imports[name]` | File-scoped, low value |
| Init function | `pkg.init[0]` | Multiple allowed, ordering |
| Local variable | `pkg.Func/body/vars[x]` | Body granularity ticket |
| Label | `pkg.Func/body/labels[L]` | Body granularity ticket |
| Statement | `pkg.Func/body/stmts[N]` | Fragile, needs design |
| Anonymous func | `pkg.Func/body/...` | Complex scoping |

### Not Addressable

| Element | Why |
|---------|-----|
| Blank identifier | No identity by design |
| Expressions | Not declarations |
| Composite literals | Not declarations |
| Type assertions | Runtime constructs |
| Anonymous struct types | No name, use position |

---

## 10. Resolved Design Decisions

1. **Interface methods: dot or slash?**
   - **Resolved:** Dot is canonical (`pkg.Reader.Read`), slash is accepted (`pkg.Reader/methods[Read]`)
   - Consistent with concrete methods; slash form available for positional access

2. **Struct fields: dot or slash?**
   - **Resolved:** Slash only (`pkg.Type/fields[Name]`)
   - Dot notation for fields was explored and discarded due to complexity in reasoning about outputs
   - Dots are reserved for methods only

3. **Positional vs named access:**
   - **Resolved:** Accept both wherever meaningful
   - Named is canonical when name exists
   - Positional required when unnamed, accepted as alternative when named

---

## 11. Open Questions

1. **Nested type parameters?**
   ```go
   func Foo[A any, B interface{ Method(A) B }](a A) B
   ```
   How to address the `A` in `B`'s constraint? Probably defer - edge case.

2. **Method on pointer vs value receiver?**
   - Both `Point.Method` and `(*Point).Method` can exist
   - Current thinking: path normalizes to base type (`pkg.Point.Method`)
   - Receiver type accessible via `/receiver/type`
   - Open: What if same method name on both? (Go disallows this, so non-issue)

3. **Package path with dots?**
   - `github.com/user/my.repo` - dots in package path
   - Potential ambiguity with symbol dots
   - Resolution approach: Package path ends at first recognized package boundary

4. **Accepted form limits?**
   - How many alternative forms do we accept?
   - Need to balance flexibility with spec clarity
   - Principle: Accept forms that map to reasonable mental models (Go syntax, AST structure, positional)
