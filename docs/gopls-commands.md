# gopls Command Reference

A comprehensive reference for all gopls commands, LSP methods, code actions, and features.

**LSP Version:** 3.17.0

---

## Table of Contents

1. [CLI Commands](#cli-commands)
2. [LSP Execute Commands](#lsp-execute-commands-workspaceexecutecommand)
3. [Code Action Kinds](#code-action-kinds-textdocumentcodeaction)
4. [Code Lenses](#code-lenses-textdocumentcodelens)
5. [Inlay Hints](#inlay-hints-textdocumentinlayhint)
6. [LSP Methods Summary](#lsp-methods-summary)

---

## CLI Commands

gopls provides a command-line interface that wraps LSP functionality for scripting and terminal use.

### Navigation Commands

#### `definition`

Show the declaration location of an identifier.

```bash
# By line:column (1-indexed)
gopls definition main.go:10:15

# By byte offset
gopls definition main.go:#256

# Output as JSON
gopls definition -json main.go:10:15

# With markdown support
gopls definition -markdown main.go:10:15
```

**Flags:**
- `-json` - Emit output in JSON format
- `-markdown` - Support markdown in responses

---

#### `references`

Find all references to an identifier across the workspace.

```bash
# Find all references to symbol at position
gopls references handler.go:25:10

# Include the declaration in results
gopls references -d handler.go:25:10
```

**Flags:**
- `-d, -declaration` - Include the declaration in results

---

#### `implementation`

Find implementations of an interface or types that implement an interface.

```bash
# Find implementations of interface at position
gopls implementation reader.go:12:6

# Using byte offset
gopls implementation reader.go:#180
```

---

#### `call_hierarchy`

Display the call hierarchy (callers and callees) for a function.

```bash
# Show call hierarchy for function at position
gopls call_hierarchy service.go:45:10
```

---

#### `symbols`

List all symbols (functions, types, variables, constants) in a file.

```bash
gopls symbols handler.go
```

**Output:** Lists symbols with their kind, name, and location.

---

#### `workspace_symbol`

Search for symbols across the entire workspace.

```bash
# Case-insensitive search (default)
gopls workspace_symbol Handler

# Fuzzy matching
gopls workspace_symbol -matcher fuzzy hndlr

# Case-sensitive matching
gopls workspace_symbol -matcher casesensitive Handler

# Fast fuzzy (less accurate but faster)
gopls workspace_symbol -matcher fastfuzzy hdl
```

**Flags:**
- `-matcher` - Matching algorithm: `fuzzy`, `fastfuzzy`, `casesensitive`, `caseinsensitive` (default)

---

### Code Intelligence Commands

#### `hover`

*(Not directly exposed as CLI command - use LSP)*

Get hover information for a symbol. Available through LSP `textDocument/hover`.

---

#### `signature`

Display signature help for a function call.

```bash
# Show signature at call site
gopls signature main.go:20:15
```

---

#### `highlight`

Show all occurrences of an identifier in the current file.

```bash
gopls highlight main.go:15:8
```

---

### Diagnostics Commands

#### `check`

Run diagnostics on a file and display results.

```bash
# Show warnings and errors (default)
gopls check main.go

# Show all diagnostics including hints
gopls check -severity=hint main.go

# Show only errors
gopls check -severity=error main.go
```

**Flags:**
- `-severity` - Minimum severity: `hint`, `info`, `warning` (default), `error`

---

### Transformation Commands

#### `format`

Format Go source code using gofmt/gofumpt.

```bash
# Print formatted output to stdout
gopls format main.go

# Write changes back to file
gopls format -w main.go

# Show diff instead of full content
gopls format -d main.go

# List files that would be modified
gopls format -l main.go

# Preserve original files when writing
gopls format -w -preserve main.go
```

**Flags:**
- `-w, -write` - Write changes to source files
- `-d, -diff` - Display diffs instead of content
- `-l, -list` - Display names of edited files
- `-preserve` - Make backup copies when writing

---

#### `imports`

Organize imports (add missing, remove unused, sort).

```bash
# Print organized imports to stdout
gopls imports main.go

# Write changes back to file
gopls imports -w main.go

# Show diff
gopls imports -d main.go
```

**Flags:** Same as `format`

---

#### `rename`

Rename an identifier across the workspace.

```bash
# Rename symbol to new name
gopls rename main.go:15:10 NewName

# Write changes to files
gopls rename -w main.go:15:10 NewName

# Show diff of changes
gopls rename -d main.go:15:10 NewName

# List affected files
gopls rename -l main.go:15:10 NewName
```

**Flags:** Same as `format`

---

#### `prepare_rename`

Validate that a rename operation is possible at a location.

```bash
gopls prepare_rename main.go:15:10
```

**Output:** Returns the current name and range if rename is valid.

---

#### `codeaction`

List or execute code actions for a file or range.

```bash
# List all code actions in file
gopls codeaction main.go

# List actions at specific line
gopls codeaction main.go:25

# List actions at specific position
gopls codeaction main.go:25:10

# Filter by kind
gopls codeaction -kind=quickfix main.go
gopls codeaction -kind=refactor.extract main.go
gopls codeaction -kind=source.organizeImports main.go

# Filter by title (regex)
gopls codeaction -title="Extract.*" main.go

# Execute first matching action
gopls codeaction -exec -kind=quickfix main.go

# Execute and show diff
gopls codeaction -exec -d -kind=quickfix main.go

# Execute and write to file
gopls codeaction -exec -w -kind=quickfix main.go
```

**Flags:**
- `-kind` - Comma-separated list of action kinds to filter
- `-title` - Regex to match action title
- `-exec` - Execute the first matching action
- `-w, -write` - Write changes to files
- `-d, -diff` - Show diff
- `-l, -list` - List affected files
- `-preserve` - Backup files when writing

**Valid Kinds:**
```
quickfix
refactor
refactor.extract
refactor.extract.constant
refactor.extract.function
refactor.extract.method
refactor.extract.toNewFile
refactor.extract.variable
refactor.inline
refactor.inline.call
refactor.rewrite
refactor.rewrite.changeQuote
refactor.rewrite.fillStruct
refactor.rewrite.fillSwitch
refactor.rewrite.invertIf
refactor.rewrite.joinLines
refactor.rewrite.removeUnusedParam
refactor.rewrite.splitLines
source
source.assembly
source.doc
source.fixAll
source.freesymbols
source.organizeImports
source.test
gopls.doc.features
```

---

#### `codelens`

List or execute code lenses for a file.

```bash
# List all code lenses in file
gopls codelens main_test.go

# List code lenses on specific line
gopls codelens main_test.go:15

# Filter by title
gopls codelens main_test.go "run test"

# Execute a code lens
gopls codelens -exec main_test.go:15 "run test"
```

**Flags:** Same as `codeaction`

---

### Semantic Analysis Commands

#### `semtok`

Display semantic tokens for a file (used for syntax highlighting).

```bash
gopls semtok main.go
```

**Output:** Lists tokens with their type, modifiers, and range.

---

#### `folding_ranges`

Show collapsible regions in a file.

```bash
gopls folding_ranges main.go
```

---

#### `links`

Extract URLs from comments and string literals.

```bash
# List links
gopls links main.go

# Output as JSON
gopls links -json main.go
```

---

### Server Commands

#### `serve`

Run gopls as an LSP server (default command).

```bash
# Start LSP server on stdio
gopls serve

# Start with debug server
gopls serve -debug=localhost:6060

# Start listening on TCP
gopls serve -listen=localhost:4389

# Enable RPC tracing
gopls serve -rpc.trace

# Log to file
gopls serve -logfile=/tmp/gopls.log
```

---

#### `version`

Print gopls version information.

```bash
gopls version

# JSON output
gopls version -json
```

---

#### `stats`

Print workspace statistics (useful for debugging).

```bash
gopls stats

# Anonymized output (no file paths)
gopls stats -anon
```

---

#### `mcp`

Start gopls as an MCP (Model Context Protocol) server for AI tools.

```bash
# Start over stdio
gopls mcp

# Start over HTTP/SSE
gopls mcp -listen=localhost:3000

# Print MCP instructions
gopls mcp -instructions

# Enable RPC tracing
gopls mcp -rpc.trace

# Log to file
gopls mcp -logfile=/tmp/gopls-mcp.log
```

---

#### `execute`

Execute a custom gopls LSP command directly.

```bash
# Add an import
gopls execute gopls.add_import '{"ImportPath": "fmt", "URI": "file:///path/to/main.go"}'

# Run tests
gopls execute gopls.run_tests '{"URI": "file:///path/to/main_test.go", "Tests": ["TestFoo"]}'

# List known packages
gopls execute gopls.list_known_packages '{"URI": "file:///path/to/main.go"}'

# Write changes to files
gopls execute -w gopls.add_import '{"ImportPath": "fmt", "URI": "file:///path/to/main.go"}'

# Show diff
gopls execute -d gopls.add_import '{"ImportPath": "fmt", "URI": "file:///path/to/main.go"}'
```

**Flags:**
- `-w, -write` - Write changes to files
- `-d, -diff` - Show diff
- `-l, -list` - List affected files
- `-preserve` - Backup files when writing

---

## LSP Execute Commands (`workspace/executeCommand`)

These commands are invoked via the LSP `workspace/executeCommand` request. They power code lenses, code actions, and can be called directly via `gopls execute`.

### Dependency Management

#### `gopls.add_dependency`

Add a new dependency to go.mod.

```bash
gopls execute gopls.add_dependency '{
  "URI": "file:///path/to/go.mod",
  "GoCmdArgs": ["example.com/pkg@latest"],
  "AddRequire": true
}'
```

**Parameters:**
- `URI` - URI of go.mod file
- `GoCmdArgs` - Arguments for `go get`
- `AddRequire` - Whether to add require directive

---

#### `gopls.remove_dependency`

Remove a dependency from go.mod.

```bash
gopls execute gopls.remove_dependency '{
  "URI": "file:///path/to/go.mod",
  "ModulePath": "example.com/old-pkg",
  "OnlyDiagnostic": false
}'
```

**Parameters:**
- `URI` - URI of go.mod file
- `ModulePath` - Module path to remove
- `OnlyDiagnostic` - Only remove if flagged by diagnostic

---

#### `gopls.upgrade_dependency`

Upgrade a dependency to a newer version.

```bash
gopls execute gopls.upgrade_dependency '{
  "URI": "file:///path/to/go.mod",
  "GoCmdArgs": ["example.com/pkg@latest"],
  "AddRequire": false
}'
```

**Parameters:** Same as `add_dependency`

---

#### `gopls.check_upgrades`

Check for available module upgrades.

```bash
gopls execute gopls.check_upgrades '{
  "URI": "file:///path/to/go.mod",
  "Modules": ["example.com/pkg"]
}'
```

**Parameters:**
- `URI` - URI of go.mod file
- `Modules` - Specific modules to check (empty = all)

---

#### `gopls.go_get_package`

Run `go get` to fetch a package.

```bash
gopls execute gopls.go_get_package '{
  "URI": "file:///path/to/main.go",
  "Pkg": "example.com/pkg",
  "AddRequire": true
}'
```

**Parameters:**
- `URI` - URI of a Go file in the module
- `Pkg` - Package path to get
- `AddRequire` - Add to require directive

---

### Import Management

#### `gopls.add_import`

Add an import statement to a Go file.

```bash
gopls execute gopls.add_import '{
  "ImportPath": "encoding/json",
  "URI": "file:///path/to/main.go"
}'
```

**Parameters:**
- `ImportPath` - Import path to add
- `URI` - Target file URI

---

#### `gopls.list_imports`

List all imports in a file and its package.

```bash
gopls execute gopls.list_imports '{
  "URI": "file:///path/to/main.go"
}'
```

**Returns:**
```json
{
  "Imports": [
    {"Path": "fmt", "Name": ""},
    {"Path": "encoding/json", "Name": "json"}
  ],
  "PackageImports": [
    {"Path": "fmt"},
    {"Path": "encoding/json"}
  ]
}
```

---

#### `gopls.list_known_packages`

List all packages that could be imported.

```bash
gopls execute gopls.list_known_packages '{
  "URI": "file:///path/to/main.go"
}'
```

**Returns:** Array of importable package paths (excludes already imported).

---

### Code Refactoring

#### `gopls.apply_fix`

Apply a suggested fix from diagnostics or code actions.

```bash
gopls execute gopls.apply_fix '{
  "Fix": "fillstruct",
  "URI": "file:///path/to/main.go",
  "Range": {
    "start": {"line": 10, "character": 5},
    "end": {"line": 10, "character": 20}
  },
  "ResolveEdits": true
}'
```

**Parameters:**
- `Fix` - Name of the fix to apply
- `URI` - File URI
- `Range` - LSP range to apply fix
- `ResolveEdits` - Whether to resolve workspace edits

---

#### `gopls.change_signature`

Refactor a function signature (experimental).

```bash
gopls execute gopls.change_signature '{
  "RemoveParameter": {
    "URI": "file:///path/to/main.go",
    "Range": {"start": {"line": 5, "character": 20}, "end": {"line": 5, "character": 25}}
  },
  "ResolveEdits": true
}'
```

---

#### `gopls.extract_to_new_file`

Move selected declarations to a new file.

```bash
gopls execute gopls.extract_to_new_file '{
  "URI": "file:///path/to/main.go",
  "Range": {
    "start": {"line": 20, "character": 0},
    "end": {"line": 50, "character": 0}
  }
}'
```

---

#### `gopls.add_test`

Generate a test function for the selected function.

```bash
gopls execute gopls.add_test '{
  "URI": "file:///path/to/handler.go",
  "Range": {
    "start": {"line": 15, "character": 0},
    "end": {"line": 15, "character": 20}
  }
}'
```

Creates a table-driven test in `*_test.go`.

---

### Testing & Validation

#### `gopls.run_tests`

Execute specific test or benchmark functions.

```bash
gopls execute gopls.run_tests '{
  "URI": "file:///path/to/handler_test.go",
  "Tests": ["TestHandler", "TestHandlerError"],
  "Benchmarks": []
}'
```

**Parameters:**
- `URI` - Test file URI
- `Tests` - Test function names to run
- `Benchmarks` - Benchmark function names to run

**Note:** Runs asynchronously, returns progress token.

---

#### `gopls.run_govulncheck`

Run vulnerability analysis asynchronously.

```bash
gopls execute gopls.run_govulncheck '{
  "URI": "file:///path/to/go.mod",
  "Pattern": "./..."
}'
```

**Parameters:**
- `URI` - Module URI
- `Pattern` - Package pattern to check

---

#### `gopls.vulncheck`

Run vulnerability analysis synchronously.

```bash
gopls execute gopls.vulncheck '{
  "URI": "file:///path/to/go.mod",
  "Pattern": "./..."
}'
```

---

#### `gopls.fetch_vulncheck_result`

Retrieve cached vulnerability check results.

```bash
gopls execute gopls.fetch_vulncheck_result '{
  "URI": "file:///path/to/go.mod"
}'
```

---

### Module & Workspace Operations

#### `gopls.tidy`

Run `go mod tidy`.

```bash
gopls execute gopls.tidy '{
  "URIs": ["file:///path/to/go.mod"]
}'
```

---

#### `gopls.update_go_sum`

Update go.sum file.

```bash
gopls execute gopls.update_go_sum '{
  "URIs": ["file:///path/to/go.mod"]
}'
```

---

#### `gopls.vendor`

Run `go mod vendor`.

```bash
gopls execute gopls.vendor '{
  "URI": "file:///path/to/go.mod"
}'
```

---

#### `gopls.edit_go_directive`

Change the Go version in go.mod.

```bash
gopls execute gopls.edit_go_directive '{
  "URI": "file:///path/to/go.mod",
  "Version": "1.21"
}'
```

---

#### `gopls.generate`

Run `go generate`.

```bash
gopls execute gopls.generate '{
  "Dir": "file:///path/to/dir",
  "Recursive": true
}'
```

**Parameters:**
- `Dir` - Directory to run generate in
- `Recursive` - Include subdirectories

---

#### `gopls.regenerate_cgo`

Regenerate cgo definitions after editing C code.

```bash
gopls execute gopls.regenerate_cgo '{
  "URI": "file:///path/to/cgo_file.go"
}'
```

---

#### `gopls.run_go_work_command`

Execute go.work commands.

```bash
gopls execute gopls.run_go_work_command '{
  "ViewID": "",
  "InitFirst": true,
  "Args": ["use", "./submodule"]
}'
```

---

#### `gopls.modules`

Get information about modules in a directory.

```bash
gopls execute gopls.modules '{
  "Dir": "file:///path/to/workspace",
  "MaxDepth": 3
}'
```

**Returns:**
```json
{
  "Modules": [
    {"Path": "example.com/mymodule", "Version": "", "GoMod": "file:///path/to/go.mod"}
  ]
}
```

---

#### `gopls.packages`

Get package metadata information.

```bash
gopls execute gopls.packages '{
  "Files": ["file:///path/to/main.go"],
  "Recursive": false,
  "Mode": 0
}'
```

---

#### `gopls.package_symbols`

Return symbols in a file's package.

```bash
gopls execute gopls.package_symbols '{
  "URI": "file:///path/to/main.go"
}'
```

---

### Diagnostics & Analysis

#### `gopls.diagnose_files`

Force diagnostics to be published for specific files.

```bash
gopls execute gopls.diagnose_files '{
  "Files": ["file:///path/to/main.go", "file:///path/to/handler.go"]
}'
```

---

#### `gopls.reset_go_mod_diagnostics`

Clear go.mod diagnostics (useful after external changes).

```bash
gopls execute gopls.reset_go_mod_diagnostics '{
  "URI": "file:///path/to/go.mod",
  "DiagnosticSource": ""
}'
```

---

#### `gopls.gc_details`

Toggle display of compiler optimization details in diagnostics.

```bash
gopls execute gopls.gc_details '"file:///path/to/main.go"'
```

Shows inlining decisions, escape analysis, etc.

---

### Documentation & Analysis

#### `gopls.doc`

Open package documentation in browser.

```bash
gopls execute gopls.doc '{
  "URI": "file:///path/to/main.go",
  "Range": {
    "start": {"line": 5, "character": 10},
    "end": {"line": 5, "character": 15}
  }
}'
```

---

#### `gopls.assembly`

Display assembly listing for a function in browser.

```bash
gopls execute gopls.assembly '["viewID", "packageID", "SymbolName"]'
```

---

#### `gopls.free_symbols`

Analyze free symbols (unbound identifiers) in a selection.

```bash
gopls execute gopls.free_symbols '["viewID", {"URI": "...", "Range": {...}}]'
```

Useful for understanding what a code block depends on.

---

### Server Introspection

#### `gopls.views`

List current workspace views.

```bash
gopls execute gopls.views
```

**Returns:**
```json
[
  {
    "ID": "view-1",
    "Type": "GoPackagesDriver",
    "Root": "file:///path/to/workspace",
    "Folder": "file:///path/to/workspace",
    "EnvOverlay": ["GOFLAGS=-mod=vendor"]
  }
]
```

---

#### `gopls.workspace_stats`

Get comprehensive workspace statistics.

```bash
gopls execute gopls.workspace_stats
```

**Returns:** File counts, package counts, diagnostic counts, etc.

---

#### `gopls.mem_stats`

Get memory usage statistics.

```bash
gopls execute gopls.mem_stats
```

**Returns:**
```json
{
  "HeapAlloc": 123456789,
  "HeapInUse": 234567890,
  "TotalAlloc": 345678901
}
```

---

#### `gopls.scan_imports`

Force a synchronous scan of the imports cache.

```bash
gopls execute gopls.scan_imports
```

**Note:** Primarily for testing.

---

### Debugging & Profiling

#### `gopls.start_debugging`

Start the gopls debug server.

```bash
gopls execute gopls.start_debugging '{
  "Addr": "localhost:6060"
}'
```

**Returns:** URLs for debug endpoints.

---

#### `gopls.start_profile`

Begin CPU/memory profiling.

```bash
gopls execute gopls.start_profile '{}'
```

---

#### `gopls.stop_profile`

Stop profiling and save results.

```bash
gopls execute gopls.stop_profile '{}'
```

**Returns:** Profile filename.

---

### Telemetry

#### `gopls.add_telemetry_counters`

Update telemetry counters (internal).

```bash
gopls execute gopls.add_telemetry_counters '{
  "Names": ["counter1"],
  "Values": [1]
}'
```

---

#### `gopls.maybe_prompt_for_telemetry`

Prompt user to enable telemetry.

```bash
gopls execute gopls.maybe_prompt_for_telemetry
```

---

#### `gopls.client_open_url`

Request the client to open a URL in browser.

```bash
gopls execute gopls.client_open_url '"https://pkg.go.dev/fmt"'
```

---

## Code Action Kinds (`textDocument/codeAction`)

Code actions are quick fixes and refactorings offered by gopls. They are accessed via the `textDocument/codeAction` LSP request or `gopls codeaction` CLI command.

### Quick Fixes (`quickfix`)

Automatically suggested fixes for diagnostics.

**Examples:**
- Add missing import
- Remove unused import
- Fix type errors
- Apply suggested simplifications

```bash
gopls codeaction -kind=quickfix -exec main.go:10
```

---

### Source Actions (`source.*`)

#### `source.organizeImports`

Organize imports: remove unused, add missing, sort alphabetically.

```bash
gopls codeaction -kind=source.organizeImports -exec main.go
```

---

#### `source.fixAll`

Apply all "safe" fixes that don't require user choice.

```bash
gopls codeaction -kind=source.fixAll -exec main.go
```

---

#### `source.addTest`

Generate a table-driven test for the selected function.

```bash
gopls codeaction -kind=source.test -exec handler.go:25
```

Creates test in `handler_test.go` with:
- Test function `TestFunctionName`
- Table-driven test structure
- Subtests for each case

---

#### `source.assembly`

Open assembly output for the current function in browser.

---

#### `source.doc`

Open documentation for the symbol under cursor.

---

#### `source.freesymbols`

Analyze free symbols in the selection.

---

#### `source.toggleCompilerOptDetails`

Toggle display of compiler optimization annotations.

---

### Extract Refactorings (`refactor.extract.*`)

#### `refactor.extract.function`

Extract selected statements into a new function.

```go
// Before
func main() {
    // [selected]
    x := compute()
    y := process(x)
    fmt.Println(y)
    // [/selected]
}

// After
func main() {
    extracted()
}

func extracted() {
    x := compute()
    y := process(x)
    fmt.Println(y)
}
```

```bash
gopls codeaction -kind=refactor.extract.function -exec main.go:5:10
```

---

#### `refactor.extract.method`

Extract statements into a method of the receiver type.

```bash
gopls codeaction -kind=refactor.extract.method -exec handler.go:20
```

---

#### `refactor.extract.variable`

Extract an expression into a local variable.

```go
// Before
fmt.Println(expensive() + 1)

// After
v := expensive()
fmt.Println(v + 1)
```

```bash
gopls codeaction -kind=refactor.extract.variable -exec main.go:10:15
```

---

#### `refactor.extract.variable-all`

Extract and replace all occurrences of an expression.

---

#### `refactor.extract.constant`

Extract an expression into a local constant.

```go
// Before
area := 3.14159 * r * r

// After
const pi = 3.14159
area := pi * r * r
```

---

#### `refactor.extract.constant-all`

Extract and replace all occurrences as a constant.

---

#### `refactor.extract.toNewFile`

Move selected declarations to a new file.

```bash
gopls codeaction -kind=refactor.extract.toNewFile -exec main.go:20:100
```

New filename is generated from the first symbol name.

---

### Inline Refactorings (`refactor.inline.*`)

#### `refactor.inline.call`

Inline a function call, replacing it with the function body.

```go
// Before
func double(x int) int { return x * 2 }
result := double(5)

// After
result := 5 * 2
```

```bash
gopls codeaction -kind=refactor.inline.call -exec main.go:10:15
```

**Handles:**
- Parameter substitution
- Return value handling
- Multi-statement bodies
- Receiver methods

---

#### `refactor.inline.variable`

Inline a variable, replacing references with its initializer.

```go
// Before
x := getValue()
fmt.Println(x)

// After
fmt.Println(getValue())
```

---

### Rewrite Refactorings (`refactor.rewrite.*`)

#### `refactor.rewrite.fillStruct`

Fill in missing fields in a struct literal.

```go
// Before
cfg := Config{}

// After
cfg := Config{
    Host:    "",
    Port:    0,
    Timeout: 0,
}
```

```bash
gopls codeaction -kind=refactor.rewrite.fillStruct -exec main.go:15
```

Uses heuristics to match variable names to field values.

---

#### `refactor.rewrite.fillSwitch`

Add missing cases to a switch statement.

```go
// Before (where Status is an enum)
switch status {
case Active:
}

// After
switch status {
case Active:
case Inactive:
case Pending:
}
```

Works with:
- Enum types (const blocks with iota)
- Type switches

---

#### `refactor.rewrite.invertIf`

Negate condition and swap if/else blocks.

```go
// Before
if x > 0 {
    doPositive()
} else {
    doNonPositive()
}

// After
if x <= 0 {
    doNonPositive()
} else {
    doPositive()
}
```

---

#### `refactor.rewrite.changeQuote`

Convert between raw and interpreted strings.

```go
// Before
s := "line1\nline2"

// After
s := `line1
line2`
```

---

#### `refactor.rewrite.joinLines`

Combine multi-line list into single line.

```go
// Before
slice := []int{
    1,
    2,
    3,
}

// After
slice := []int{1, 2, 3}
```

---

#### `refactor.rewrite.splitLines`

Split single-line list into multiple lines.

```go
// Before
slice := []int{1, 2, 3}

// After
slice := []int{
    1,
    2,
    3,
}
```

---

#### `refactor.rewrite.addTags`

Add struct tags (default: json with snake_case).

```go
// Before
type User struct {
    FirstName string
    LastName  string
}

// After
type User struct {
    FirstName string `json:"first_name"`
    LastName  string `json:"last_name"`
}
```

---

#### `refactor.rewrite.removeTags`

Remove struct tags from fields.

---

#### `refactor.rewrite.removeUnusedParam`

Remove or rename unused function parameters.

```go
// Before
func process(ctx context.Context, unused int, data []byte) {}

// After
func process(ctx context.Context, _ int, data []byte) {}
// or
func process(ctx context.Context, data []byte) {}
```

Updates all call sites.

---

#### `refactor.rewrite.moveParamLeft`

Move a parameter one position left in the signature.

---

#### `refactor.rewrite.moveParamRight`

Move a parameter one position right in the signature.

---

#### `refactor.rewrite.eliminateDotImport`

Remove dot import and qualify all usages.

```go
// Before
import . "fmt"
Println("hello")

// After
import "fmt"
fmt.Println("hello")
```

---

### Special Actions

#### `gopls.doc.features`

Open gopls feature documentation in browser.

---

## Code Lenses (`textDocument/codeLens`)

Code lenses are actionable annotations displayed above code. They are configured via the `codelenses` setting.

### `generate`

**Location:** Above `//go:generate` comments

**Action:** Run `go generate` in directory

```go
//go:generate stringer -type=Status  // [run go generate]
```

**Default:** Enabled

---

### `regenerate_cgo`

**Location:** Above `import "C"` declarations

**Action:** Regenerate cgo definitions

```go
import "C"  // [regenerate cgo]
```

**Default:** Enabled

---

### `test`

**Location:** Above `Test*` and `Benchmark*` functions

**Action:** Run the specific test/benchmark

```go
func TestHandler(t *testing.T) {  // [run test] [debug test]
```

**Default:** Disabled (enable with `"test": true`)

---

### `run_govulncheck`

**Location:** Above `module` directive in go.mod

**Action:** Run govulncheck asynchronously

```
module example.com/myapp  // [run govulncheck]
```

**Default:** Enabled (legacy)

---

### `vulncheck`

**Location:** Above `module` directive in go.mod

**Action:** Run govulncheck synchronously

**Default:** Disabled (experimental)

---

### `tidy`

**Location:** Above `module` directive in go.mod

**Action:** Run `go mod tidy`

```
module example.com/myapp  // [tidy]
```

**Default:** Enabled

---

### `upgrade_dependency`

**Location:** Above `module` directive in go.mod

**Action:** Check for and apply dependency upgrades

```
module example.com/myapp  // [check for upgrades]
```

**Default:** Enabled

---

### `vendor`

**Location:** Above `module` directive in go.mod

**Action:** Run `go mod vendor`

```
module example.com/myapp  // [vendor]
```

**Default:** Enabled

---

## Inlay Hints (`textDocument/inlayHint`)

Inlay hints are inline annotations that reveal implicit information. Configure via `hints` setting.

### `assignVariableTypes`

Show types of variables in assignments.

```go
x := getValue()  // x: string
```

**Default:** Disabled

---

### `compositeLiteralFields`

Show field names in composite literals.

```go
return Point{/*x:*/ 10, /*y:*/ 20}
```

**Default:** Disabled

---

### `compositeLiteralTypes`

Show types in composite literals.

```go
items := []/*Point*/{Point{1, 2}}
```

**Default:** Disabled

---

### `constantValues`

Show computed values of constants.

```go
const size = 1 << 10  // = 1024
```

**Default:** Disabled

---

### `functionTypeParameters`

Show inferred type parameters in generic function calls.

```go
slices.Sort/*[int]*/(numbers)
```

**Default:** Disabled

---

### `parameterNames`

Show parameter names at call sites.

```go
process(/*ctx:*/ ctx, /*timeout:*/ 5*time.Second)
```

**Default:** Disabled

---

### `rangeVariableTypes`

Show types of range variables.

```go
for i/*: int*/, v/*: string*/ := range items {
```

**Default:** Disabled

---

### `ignoredError` (special)

Flag ignored error returns.

```go
_ = Write(data)  // ← ignored error
```

**Default:** Disabled

---

## LSP Methods Summary

### Fully Supported

| Method | Description |
|--------|-------------|
| `textDocument/hover` | Symbol information on hover |
| `textDocument/completion` | Code completion |
| `completionItem/resolve` | Completion detail resolution |
| `textDocument/signatureHelp` | Function signature help |
| `textDocument/definition` | Go to definition |
| `textDocument/typeDefinition` | Go to type definition |
| `textDocument/implementation` | Find implementations |
| `textDocument/references` | Find all references |
| `textDocument/documentHighlight` | Highlight symbol occurrences |
| `textDocument/documentSymbol` | File outline/symbols |
| `textDocument/codeAction` | Quick fixes and refactorings |
| `textDocument/codeLens` | Actionable code annotations |
| `codeLens/resolve` | Code lens resolution |
| `textDocument/formatting` | Format document |
| `textDocument/rename` | Rename symbol |
| `textDocument/prepareRename` | Validate rename |
| `textDocument/foldingRange` | Collapsible regions |
| `textDocument/selectionRange` | Smart selection |
| `textDocument/semanticTokens/full` | Semantic highlighting |
| `textDocument/inlayHint` | Inline type hints |
| `inlayHint/resolve` | Hint resolution |
| `textDocument/documentLink` | URL extraction |
| `documentLink/resolve` | Link resolution |
| `textDocument/prepareCallHierarchy` | Call hierarchy setup |
| `callHierarchy/incomingCalls` | Find callers |
| `callHierarchy/outgoingCalls` | Find callees |
| `textDocument/prepareTypeHierarchy` | Type hierarchy setup |
| `typeHierarchy/subtypes` | Find subtypes |
| `typeHierarchy/supertypes` | Find supertypes |
| `workspace/symbol` | Workspace symbol search |
| `workspace/executeCommand` | Custom commands |
| `workspace/applyEdit` | Apply workspace edits |
| `textDocument/publishDiagnostics` | Push diagnostics |
| `textDocument/diagnostic` | Pull diagnostics (experimental) |

### Not Applicable to Go

| Method | Reason |
|--------|--------|
| `textDocument/declaration` | Go has no separate declaration |
| `textDocument/documentColor` | Not applicable |
| `textDocument/colorPresentation` | Not applicable |
| `textDocument/linkedEditingRange` | HTML/template feature |
| `textDocument/onTypeFormatting` | Go uses save-time formatting |
| `textDocument/rangeFormatting` | Go formats whole files |
| `textDocument/moniker` | Cross-language linking |
| `textDocument/inlineValue` | Debugger feature |

---

## Configuration Example

```json
{
  "gopls": {
    "formatting.gofumpt": true,
    "ui.completion.usePlaceholders": true,
    "ui.diagnostic.staticcheck": true,
    "ui.inlayhint.parameterNames": true,
    "ui.codelenses": {
      "test": true,
      "generate": true,
      "tidy": true
    },
    "ui.semanticTokens": true
  }
}
```

---

## References

- [gopls Documentation](https://github.com/golang/tools/tree/master/gopls/doc)
- [gopls Features](https://github.com/golang/tools/tree/master/gopls/doc/features)
- [LSP 3.17 Specification](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/)
- [gopls Command Package](https://pkg.go.dev/golang.org/x/tools/gopls/internal/protocol/command)
