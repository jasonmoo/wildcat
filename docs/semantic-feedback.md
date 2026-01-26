# Wildcat Semantic Paths: AI Agent Feedback

This document captures feedback from an AI agent (Claude) on a proposed semantic path system for wildcat that would enable reading and editing Go source code using symbolic paths rather than files and line numbers.

## The Proposal

A path-based system for addressing code elements:
- `database.NewDatabase` - a function
- `database.NewDatabase/params[ctx]` - a specific parameter named `ctx`
- `database.User/fields[Name]` - a struct field
- `database.User.GetID/body` - a method body

Commands:
- `ls` - list paths in a package/symbol, discover subpaths
- `read` - read source code at a path
- `edit` - modify source at a path

Goal: Get away from files, lines, and string-based search/replace. Keep the agent in semantic/symbolic thinking. Get the filesystem out of the way entirely.

---

## The Current Pain Point

Right now the workflow has awkward translation boundaries:

```
wildcat symbol (semantic)
    → file:line (filesystem)
    → Read file for exact text (string)
    → Edit with string match (hope it's unique)
    → gopls check (back to semantic)
```

Every transition is a potential failure point. The Edit tool requires exact string matching, which means I'm constantly translating *from* symbolic thinking *to* text manipulation *and back*.

### Concrete Example: Renaming a Parameter

To rename a function parameter today:

1. `wildcat symbol database.NewDatabase` → get definition location
2. Read the file to get exact function signature text
3. Edit tool with old_string = exact signature, new_string = modified signature
4. Hope the signature text is unique in the file
5. Find all callers, read each file, edit each call site

With semantic paths:

1. `edit database.NewDatabase/params[ctx] --rename context`
2. Tool handles all the file operations internally

---

## What the Proposal Enables

Stay semantic the whole way:

```
wildcat symbol → path (database.NewDatabase/params[ctx])
    → read path → source
    → edit path → done
```

No files. No line numbers. No "is this string unique?" anxiety.

---

## Why I Want This

### 1. Structural Edits Are Currently Painful

Adding a parameter to a function means finding the exact insertion point in text, getting the comma placement right, handling the case where there are no existing parameters vs. appending to a list, etc.

With paths, it could be:
```bash
add database.NewDatabase/params --name timeout --type time.Duration
```

The tool knows the structure and handles insertion correctly.

### 2. Rename Refactoring

Today I run `wildcat symbol`, get all references, then do N separate string-replace edits hoping each is unique. With paths, one command could rename the symbol at all reference sites.

### 3. The Snippets-to-Edit Workflow Is Good But Fragile

Wildcat already gives me snippets I can use as `old_string` in Edit tool calls. This works well most of the time. But if the snippet isn't unique in the file, I need to expand context manually.

Paths eliminate this entirely - the path *is* the unique identifier. There's no ambiguity about what `database.NewDatabase/params[ctx]` refers to.

### 4. Annotations on Wildcat Output Would Be Powerful

If `wildcat symbol` showed paths inline:
```
database.NewDatabase // database.NewDatabase
  params:
    ctx: context.Context      // database.NewDatabase/params[ctx]
    logger: log.Logger        // database.NewDatabase/params[logger]
  returns:
    - *Database               // database.NewDatabase/returns[0]
```

I could copy-paste paths directly into `read` or `edit` commands. No translation step.

### 5. The Core Insight Is Correct

**Code is a tree, not a string.** Files and lines are a serialization format, not the true structure. If I can work with the tree directly, I'll make fewer errors and work faster.

I already think in terms of "the ctx parameter of NewDatabase" - being able to *address* that directly rather than translate to file:line:column would be a significant improvement.

---

## Questions and Design Considerations

### Body Granularity

How deep do paths go into function bodies?

| Level | Example | Tradeoff |
|-------|---------|----------|
| Full body only | `Func/body` | Simple, but requires full replacement |
| Statement level | `Func/body/stmts[3]` | More precise, but statements can shift |
| Expression level | `Func/body/stmts[3]/if/else/call/args[1]` | Very precise, probably too fragile |

**My instinct:** Full body replacement is probably fine for the initial implementation. Bodies are where the creative logic lives - I often need to see and change surrounding context anyway. The structural path system is most valuable for the *structural* parts of code: signatures, fields, types, interfaces.

If statement-level addressing is added later, it should probably use content-based identification rather than positional indices (which shift as code changes).

### Docs and Comments

Would `Func/doc` give me the godoc comment? That would be useful for documentation edits.

Possible paths:
- `database.NewDatabase/doc` - godoc comment
- `database.User/fields[Name]/doc` - field comment
- `database.User/fields[Name]/tag` - struct tag

### Struct Tags

`Type/fields[Name]/tag` would be great for JSON/SQL tag manipulation. These are currently annoying to edit because you have to match the exact backtick-quoted string with all its contents.

### Cross-File Atomicity

If I rename a type, does one `edit` command update all files, or do I issue multiple edits?

Options:
1. **Single atomic rename**: `rename database.User → database.Account` updates all references
2. **Path-based edits are per-site**: I still issue multiple edits, but each is unambiguous

Option 1 is more powerful but more complex. Option 2 is simpler and still a big improvement over string matching.

### Interface Relationships

Could paths express interface relationships?

- `database.User/implements` - list interfaces this type satisfies
- `database.Repository/implemented-by` - list types implementing this interface

These might be query-only (not editable), but useful for discovery.

---

## What Would Make This Transformative

If the system could handle these operations:

### Rename (all references)
```bash
wildcat rename database.User database.Account
```

### Add struct field
```bash
wildcat add database.Config/fields \
  --name Timeout \
  --type time.Duration \
  --tag 'json:"timeout"'
```

### Add function parameter
```bash
wildcat add database.NewDatabase/params \
  --name timeout \
  --type time.Duration \
  --position 2
```

### Move symbol
```bash
wildcat move database.helperFunc database/internal.HelperFunc
```

Unexported → new package, with all call sites updated.

### Change return type
```bash
wildcat edit database.Query/returns[1] --type error
```

These are refactorings that are currently tedious multi-step operations with high error potential.

---

## Path Syntax Thoughts

A possible grammar:

```
path := package "." symbol subpath*
subpath := "/" category ("[" selector "]")?

category := "params" | "returns" | "fields" | "methods" | "body" | "doc" | "tag"
selector := name | index

Examples:
  database.NewDatabase                      # function
  database.NewDatabase/params[ctx]          # named parameter
  database.NewDatabase/params[0]            # positional parameter
  database.NewDatabase/returns[0]           # first return value
  database.User                             # type
  database.User/fields[Name]                # struct field
  database.User/fields[Name]/tag            # struct tag
  database.User/fields[Name]/doc            # field doc comment
  database.User.GetID                       # method
  database.User.GetID/body                  # method body
  database.Repository/methods[Get]          # interface method
```

The syntax should be:
- Unambiguous (one path = one code element)
- Tab-completable (for CLI ergonomics)
- Grep-friendly (can search for paths in output)
- Copy-pasteable (from wildcat output to edit commands)

---

## Integration with Existing Commands

### wildcat symbol

Annotate output with paths:
```
database.NewDatabase                              // path: database.NewDatabase
  defined: database/db.go:45
  params:
    ctx context.Context                           // path: database.NewDatabase/params[ctx]
    logger log.Logger                             // path: database.NewDatabase/params[logger]
  returns:
    *Database                                     // path: database.NewDatabase/returns[0]
    error                                         // path: database.NewDatabase/returns[1]
  callers(12):
    cmd/bot.main                                  // path: cmd/bot.main
    ...
```

### wildcat package

Show paths for all exported symbols:
```
# package database

## Functions
database.NewDatabase(ctx, logger) (*Database, error)
database.Connect(dsn) (*sql.DB, error)

## Types
database.User                                     // 5 fields, 3 methods
  database.User/fields[ID]
  database.User/fields[Name]
  database.User/fields[Email]
  ...
```

### wildcat ls (new)

Discovery command:
```bash
$ wildcat ls database
database.NewDatabase
database.Connect
database.User
database.Config
...

$ wildcat ls database.User
database.User/fields[ID]
database.User/fields[Name]
database.User/fields[Email]
database.User/fields[CreatedAt]
database.User/fields[UpdatedAt]
database.User.GetID
database.User.GetName
database.User.Validate

$ wildcat ls database.NewDatabase
database.NewDatabase/params[ctx]
database.NewDatabase/params[logger]
database.NewDatabase/returns[0]
database.NewDatabase/returns[1]
database.NewDatabase/body
database.NewDatabase/doc
```

### wildcat read (new)

Read source at a path:
```bash
$ wildcat read database.NewDatabase
func NewDatabase(ctx context.Context, logger log.Logger) (*Database, error) {
    db, err := Connect(os.Getenv("DATABASE_URL"))
    if err != nil {
        return nil, fmt.Errorf("connect: %w", err)
    }
    return &Database{db: db, logger: logger}, nil
}

$ wildcat read database.NewDatabase/params[ctx]
ctx context.Context

$ wildcat read database.User/fields[Name]
Name string `json:"name" db:"name"`

$ wildcat read database.User/fields[Name]/tag
`json:"name" db:"name"`
```

### wildcat edit (new)

Modify source at a path:
```bash
# Rename a parameter
$ wildcat edit database.NewDatabase/params[ctx] --rename context

# Change a type
$ wildcat edit database.User/fields[Name] --type sql.NullString

# Update a struct tag
$ wildcat edit database.User/fields[Name]/tag '`json:"full_name" db:"name"`'

# Replace function body
$ wildcat edit database.NewDatabase/body < new_body.go
```

---

## Summary

The semantic path system would eliminate an entire category of errors (string uniqueness, line drift, file path management) and let me think at the abstraction level that matches the actual problem domain.

The `ls`/`read`/`edit` command structure with paths feels natural. I already think in terms of "the ctx parameter of NewDatabase" - being able to *address* that directly rather than translate to file:line:column would be a significant improvement.

The key insight driving this proposal is correct: **code is a tree, files are a serialization format**. Working with the tree directly is the right abstraction for code manipulation.
