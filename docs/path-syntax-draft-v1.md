# Path Syntax for Code Addressing

This document describes the path syntax for addressing Go code by semantic identity rather than file locations or string matching.

## Purpose

Paths let you reference code the way you think about it:

```
golang.WalkReferences/params[ctx]   # "the ctx parameter of WalkReferences"
golang.Symbol.Name                  # "the Name field of Symbol"
golang.Symbol.String/body           # "the body of the String method"
```

Instead of:
- File paths and line numbers (`refs.go:45-67`)
- Fragile string matching (`func WalkReferences(`)

## Syntax Layers

A path has up to three layers:

```
golang.Symbol.String/params[0]
└─── identity ─────┘└part─┘└sel┘
```

### 1. Identity (dots)

The base symbol, using Go's dot notation:

```
golang.Symbol           # a type
golang.WalkReferences   # a function
golang.Symbol.String    # a method
golang.Symbol.Name      # a struct field
```

Dots address named members: packages → types/funcs → methods/fields.

### 2. Structure (slashes)

Navigate into structural parts that aren't named members:

```
/body      # function/method body
/params    # parameter list
/returns   # return value list
```

Slashes are only needed where there's ambiguity. A function has both params and returns, so you need `/params` or `/returns` to distinguish.

### 3. Selection (brackets)

Select a specific element by position or name:

```
[0]        # first element (positional)
[ctx]      # element named "ctx"
```

Both positional and named access use brackets.

## When You Need Each Layer

**Types (structs)** - dots suffice for named access:
```
golang.Symbol           # the type
golang.Symbol.Name      # field by name (dot - it's a member)
golang.Symbol[0]        # field by position (bracket)
```

**Types (interfaces)** - same pattern:
```
golang.RefVisitor           # the interface
golang.RefVisitor.Visit     # method by name
golang.RefVisitor[0]        # method by position
```

**Functions** - need categories to distinguish params from returns:
```
golang.WalkReferences           # the function
golang.WalkReferences/body      # body (unambiguous, no brackets)
golang.WalkReferences/params[0] # first param
golang.WalkReferences/params[ctx] # param named ctx
golang.WalkReferences/returns[0]  # first return value
```

**Methods** - same as functions:
```
golang.Symbol.String           # the method
golang.Symbol.String/body      # body
golang.Symbol.String/returns[0] # return value
```

## Complete Examples

```
# Package scope
golang                              # the package itself

# Types
golang.Symbol                       # struct type
golang.Symbol.Name                  # field (by name)
golang.Symbol.Kind                  # field (by name)
golang.Symbol[0]                    # field (by position)
golang.Symbol.String                # method
golang.Symbol.String/body           # method body

# Interfaces
golang.RefVisitor                   # interface type
golang.RefVisitor.Visit             # interface method

# Functions
golang.WalkReferences               # function
golang.WalkReferences/body          # function body
golang.WalkReferences/params[pkgs]  # named parameter
golang.WalkReferences/params[0]     # positional parameter
golang.WalkReferences/returns[0]    # return value

# Constants and Variables
golang.SymbolKindFunc               # constant
golang.ErrNotFound                  # variable
```

## How to Think About It

**Dots for members.** Anything that's a named member of something else uses dots. A field is a member of a struct. A method is a member of a type. This matches Go's own syntax.

**Slashes for structure.** When you need to go into something that isn't a named member - like a function's parameter list or body - use a slash. These are structural parts, not named members.

**Brackets for selection.** When you have a collection (params, returns, fields by position), brackets select one item - by index or by name.

## Discovery

Use the `paths` command to discover available paths:

```bash
wildcat paths golang              # all paths in package
wildcat paths golang.Symbol       # paths within Symbol (fields, methods)
wildcat paths golang.WalkReferences  # paths within function (params, returns, body)
```

## Relationship to Wildcat Symbols

The identity portion is always a valid wildcat symbol:

```bash
wildcat symbol golang.Symbol        # works
wildcat symbol golang.Symbol.String # works (method)
wildcat symbol golang.Symbol.Name   # works (field) [pending wc-2eea]
```

Paths extend this symbol system with positional access and structural navigation.
