# Go Programming Language Specification (Go 1.25)

Source: https://go.dev/ref/spec (fetched 2026-01-27)

This is the official reference manual for Go, embedded here for path system design reference.

---

## Core Sections

### 1. Source Code Representation
- UTF-8 encoding, Unicode character support
- Characters: newline, unicode_char, unicode_letter, unicode_digit
- Letters and digits: underscore considered lowercase letter

### 2. Lexical Elements

#### Comments
- Line comments: `//` to end of line
- General comments: `/* */`

#### Tokens
Four classes: identifiers, keywords, operators/punctuation, literals

#### Keywords (25 total)
```
break    case     chan      const     continue
default  defer    else      fallthrough for
func     go       goto      if        import
interface map     package   range     return
select   struct   switch    type      var
```

#### Literals

**Integer Literals:**
- Decimal: `42`, `0`, `1_000_000`
- Binary: `0b1010`, `0B_1111`
- Octal: `0o755`, `0755`, `0O123`
- Hexadecimal: `0xFF`, `0x_CAFE_BABE`

**Floating-Point Literals:**
- Decimal: `0.`, `72.40`, `.25`, `1.e+0`, `6.67428e-11`
- Hexadecimal: `0x1p-2`, `0x2.p10`, `0X.8p-0`

**Imaginary Literals:** `0i`, `2.71828i`, `1E6i`

**Rune Literals:** Single characters in single quotes
```go
'a'          // Unicode U+0061
'ä'          // U+00E4
'\n'         // newline
'\x07'       // hex escape
'\u12e4'     // unicode escape
'\U00101234' // long unicode escape
```

**String Literals:**
- Raw strings (backticks): `` `foo\nbar` `` (no escaping)
- Interpreted strings (double quotes): `"foo\nbar"` (with escaping)

### 3. Constants

Untyped constants have default types:
- Boolean → `bool`
- Rune → `rune`
- Integer → `int`
- Floating-point → `float64`
- Complex → `complex128`
- String → `string`

Constants represent exact values with arbitrary precision.

### 4. Variables

A variable stores a value determined by its type. Storage allocated by:
- Variable declarations
- Function parameters/results
- `new()` builtin
- Composite literals

**Static type**: declared type
**Dynamic type**: actual runtime type (for interface values)
**Zero value**: default value when uninitialized

### 5. Types

#### Boolean Type
```go
bool  // predeclared boolean type
```

#### Numeric Types

**Unsigned integers:**
```go
uint8  uint16  uint32  uint64
byte   (alias for uint8)
uint   (implementation-specific: 32 or 64 bits)
uintptr
```

**Signed integers:**
```go
int8  int16  int32  int64
int   (same size as uint)
rune  (alias for int32)
```

**Floating-point:**
```go
float32  float64
```

**Complex:**
```go
complex64   // float32 real and imaginary parts
complex128  // float64 real and imaginary parts
```

#### String Type
```go
string  // immutable sequence of bytes (UTF-8 encoded)
```

#### Array Types
```go
[32]byte
[2*N] struct { x, y int32 }
[3][5]int  // multidimensional
```

#### Slice Types
```go
[]T  // descriptor for contiguous segment of underlying array
```

#### Struct Types
```go
struct {
    x, y int
    u float32
    A *[]int
    F func()
}
```

Embedded fields promote methods/fields.

#### Pointer Types
```go
*T  // pointer to type T
```

#### Function Types
```go
func()
func(x int) int
func(a, b int, z float32) (bool)
func(prefix string, values ...int)  // variadic
```

#### Interface Types

**Basic interface (methods only):**
```go
interface {
    Read([]byte) (int, error)
    Write([]byte) (int, error)
    Close() error
}
```

**Embedded interfaces:**
```go
type Reader interface {
    Read(p []byte) (n int, err error)
    Close() error
}

type ReadWriter interface {
    Reader  // embedded
    Writer  // embedded
}
```

**General interfaces (type sets):**
```go
interface {
    int                    // only int
    ~int                   // all types with underlying int
    ~int | ~float64        // union
}
```

**Empty interface:**
```go
interface{}  // or predeclared 'any'
```

#### Map Types
```go
map[string]int
map[*T]struct{ x, y float64 }
```

Key type must support `==` and `!=`.

#### Channel Types
```go
chan T          // bidirectional
chan<- float64  // send-only
<-chan int      // receive-only
```

### 6. Type Properties

#### Underlying Types
Every type has an underlying type. For predeclared types, it's itself. For defined types, it's the type being defined.

#### Type Identity
Two types are identical if:
- Same name (for named types), OR
- Structurally equivalent type literals

#### Type Parameters
```go
func min[T ~int|~float64](x, y T) T { … }
type List[T any] struct { … }
```

#### Assignability
A value of type `V` is assignable to type `T` if:
- `V` and `T` are identical
- Underlying types identical (at least one not named)
- Channel types with identical elements, `V` bidirectional
- `T` is interface and value implements `T`
- Value is `nil` and `T` is pointer/function/slice/map/channel/interface
- Value is untyped constant representable by `T`

### 7. Method Sets

Methods callable on an operand depend on type:
- Defined type `T`: methods with receiver `T`
- Pointer `*T`: methods with receiver `*T` or `T`
- Interface type: intersection of method sets of type set

### 8. Blocks and Scope

**Implicit blocks:**
1. Universe block (all Go source)
2. Package block (per package)
3. File block (per file)
4. Implicit blocks in `if`, `for`, `switch`, `select`, clause bodies

**Scope rules:**
- Predeclared identifiers: universe scope
- Top-level declarations: package scope
- Imported package names: file scope
- Function parameters/results: function body
- Type parameters: from name to end of signature/type definition
- Local variables: from declaration to end of innermost block

### 9. Declarations and Scope

#### Constant Declarations
```go
const Pi float64 = 3.14159265358979323846
const zero = 0.0         // untyped
const (
    size int64 = 1024
    eof        = -1  // untyped
)
```

#### Iota
```go
const (
    c0 = iota  // 0
    c1 = iota  // 1
    c2 = iota  // 2
)

const (
    a = 1 << iota  // 1
    b = 1 << iota  // 2
    c = 3          // 3 (iota unused)
    d = 1 << iota  // 8
)
```

#### Type Declarations

**Alias declarations:**
```go
type nodeList = []*Node
type Polar = polar
type set[P comparable] = map[P]bool  // generic alias
```

**Type definitions:**
```go
type Point struct{ x, y float64 }
type Block interface { … }
```

Generic types:
```go
type List[T any] struct {
    next  *List[T]
    value T
}
```

#### Type Parameter Declarations
```go
[P any]
[S interface{ ~[]byte|string }]
[S ~[]E, E any]
[P Constraint[int]]
```

#### Type Constraints
```go
type Constraint ~int         // illegal in declaration
[T interface{~int}]          // ok
[T ~int]                     // shorthand for above
[T int|string]               // union
[T comparable]               // strictly comparable types
```

#### Variable Declarations
```go
var i int
var U, V, W float64
var k = 0
var x, y float32 = -1, -2
var d = math.Sin(0.5)  // d is float64
```

#### Short Variable Declarations
```go
i, j := 0, 10
f := func() int { return 7 }
r, w, _ := os.Pipe()
field1, offset := nextField(str, 0)
field2, offset := nextField(str, offset)  // redeclares offset
```

#### Function Declarations
```go
func IndexRune(s string, r rune) int {
    for i, c := range s {
        if c == r {
            return i
        }
    }
}

func min[T ~int|~float64](x, y T) T { … }  // generic
```

#### Method Declarations
```go
func (p *Point) Length() float64 {
    return math.Sqrt(p.x*p.x + p.y*p.y)
}

func (p Pair[A, B]) Swap() Pair[B, A] { … }  // generic receiver
```

### 10. Expressions

#### Operands
```go
x                // identifier
math.Sin         // qualified identifier
(x + y)          // parenthesized expression
f[int]           // instantiated generic function
```

#### Composite Literals
```go
// Array
[3]int{1, 2}
[...]int{1, 2, 3}

// Slice
[]int{1, 2, 3}

// Struct
Point{1, 2}
Point{x: 1, y: 2}

// Map
map[string]int{"a": 1, "b": 2}
```

#### Operators

**Arithmetic:** `+`, `-`, `*`, `/`, `%`

**Comparison:** `==`, `!=`, `<`, `<=`, `>`, `>=`

**Logical:** `&&`, `||`, `!`

**Bitwise:** `&`, `|`, `^`, `<<`, `>>`, `&^`

**Other:** `&` (address), `*` (dereference), `<-` (receive)

#### Index Expressions
```go
a[x]          // array, slice, string, map, type parameter
a[low : high] // slice (low:high, low:, :high, :)
a[low : high : max]  // slice with capacity
```

#### Type Assertions
```go
x.(T)           // asserts x is of type T
x.(T), ok       // ok reports whether assertion succeeds
```

#### Calls
```go
f()
f(g())
f(x, y, ..., z)
f(s...)  // pass slice elements as arguments
```

#### Conversions
```go
uint(x)
float64(x)
[]rune(x)
```

### 11. Statements

#### Expression Statements
```go
h(x+y)
f.Close()
<-ch
```

#### Send Statements
```go
ch <- 3
```

#### Assignments
```go
x = 1
a, b = b, a  // swap
x += 1
```

#### If Statements
```go
if x > 0 {
    f()
}

if x := f(); x < y {
    return x
} else if x > z {
    return z
} else {
    return y
}
```

#### For Statements
```go
for i := 0; i < 10; i++ { … }
for i < 10 { … }
for { … }  // infinite loop
for i, v := range a { … }
for range 10 { … }  // Go 1.22+
```

#### Switch Statements
```go
switch tag {
case 0, 1, 2:
    f()
default:
    h()
}

switch v := x.(type) {
case int:
    fmt.Println("x is int")
default:
    fmt.Println("unknown")
}
```

#### Select Statements
```go
select {
case v := <-ch:
    fmt.Println(v)
case ch <- x:
    fmt.Println("sent")
default:
    fmt.Println("no activity")
}
```

#### Control Flow
- `break`, `break Label`
- `continue`, `continue Label`
- `goto Label`
- `return`, `return x`, `return x, y`
- `defer f()`

### 12. Built-in Functions

```go
append(s []T, x ...T) []T
cap(v) int
clear(m)           // Go 1.21+
close(c chan<- T)
complex(r, i) complex128
copy(dst, src []T) int
delete(m, key)
imag(c) float64
len(v) int
make(T, args...) T
max(x, y...) T     // Go 1.21+
min(x, y...) T     // Go 1.21+
new(T) *T
panic(v any)
print(args...)
println(args...)
real(c) float64
recover() any
```

### 13. Packages

#### Package Clause
```go
package main
package math
```

#### Import Declaration
```go
import "fmt"
import . "fmt"   // dot import
import f "fmt"   // aliased import
import _ "fmt"   // blank import (side effects only)
```

#### Exported Identifiers
- First character is uppercase letter
- Declared in package block OR field/method name
- Accessible from other packages

#### Package Initialization
1. Import statements processed (recursively)
2. Package-level const/var declarations
3. `init()` functions (called automatically)
4. `main()` function (entry point for main package)

### 14. Program Execution

1. All packages imported
2. All package-level const/var initialized
3. All `init()` functions called
4. `main()` function in main package called
5. Program terminates (return from main or `os.Exit`)

---

## Go 1.18+ Generics

### Type Parameters
```go
func min[T ~int|~float64](x, y T) T
type List[T any] struct { next *List[T]; value T }
```

### Type Constraints
```go
[T any]                    // any type
[T comparable]             // types supporting == and !=
[T ~int|~float64]          // underlying type constraints
[T interface{~[]E}, E any] // multiple type params
```

### Type Sets
Interfaces define sets of types, not just method sets.
