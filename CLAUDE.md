# Wildcat CLI

## Project Overview
Static analysis CLI for AI agents. Uses gopls to provide symbol-based code queries with structured JSON output.

## Design Principles
- **Composability first**: Use interfaces to decouple components. Design for change.
- **Simple over clever**: Clear, readable code beats complex abstractions.
- **Adequate configuration**: Support config where needed, but avoid over-engineering.
- **Small, focused packages**: Each package should do one thing well.

## Code Standards
- Use `cobra` for CLI structure
- Use interfaces at package boundaries
- Keep functions short and testable
- Error handling: wrap errors with context using `fmt.Errorf("context: %w", err)`
- No global state; pass dependencies explicitly

## Project Structure
```
cmd/           # CLI commands (cobra)
internal/      # Private packages
  config/      # Configuration handling
  errors/      # Error types and codes
  golang/      # Go-specific helpers (stdlib detection, etc.)
  lsp/         # LSP client and protocol types
  output/      # Output formatting and writers
  servers/     # Language server specs
  symbols/     # Symbol parsing and resolution
  traverse/    # Call graph traversal
```

## Testing
- Table-driven tests preferred
- Test interfaces, not implementations
- Mock external dependencies

## Build & Run
```bash
go build -o wildcat .
./wildcat
```

Use wildcat while developing wildcat to further inform how the tool should
work and hone the workflows.

----

NOTE: bd has been provided as a mechanism of memory for you. Track your
work and insights and communicate with future Claudes here. Please be
aware your context may not be available to you in future sessions and
bd is your only means of persisting information over time.

bd - Dependency-Aware Issue Tracker

Issues chained together like beads.

CREATING ISSUES
  bd create "Fix login bug"
  bd create "Add auth" -p 0 -t feature
  bd create "Write tests" -d "Unit tests for auth" --assignee alice

VIEWING ISSUES
  bd list       List all issues
  bd list --status open  List by status
  bd list --priority 0  List by priority (0-4, 0=highest)
  bd show bd-1       Show issue details

MANAGING DEPENDENCIES
  bd dep add bd-1 bd-2     Add dependency (bd-2 blocks bd-1)
  bd dep tree bd-1  Visualize dependency tree
  bd dep cycles      Detect circular dependencies

DEPENDENCY TYPES
  blocks  Task B must complete before task A
  related  Soft connection, doesn't block progress
  parent-child  Epic/subtask hierarchical relationship
  discovered-from  Auto-created when AI discovers related work

READY WORK
  bd ready       Show issues ready to work on
            Ready = status is 'open' AND no blocking dependencies
            Perfect for agents to claim next work!

UPDATING ISSUES
  bd update bd-1 --status in_progress
  bd update bd-1 --priority 0
  bd update bd-1 --assignee bob

CLOSING ISSUES
  bd close bd-1
  bd close bd-2 bd-3 --reason "Fixed in PR #42"

AGENT INTEGRATION
  bd is designed for AI-supervised workflows:
    • Agents create issues when discovering new work
    • bd ready shows unblocked work ready to claim
    • Use --json flags for programmatic parsing
    • Dependencies prevent agents from duplicating effort

## bd Command Reference

Quick reference for commonly-used flags. For basic usage, see the bd section above.

### `bd create` - Create Issues

```bash
bd create "Title"                              # Basic (type=task, priority=2)
bd create "Title" -t feature -p 1              # High-priority feature
bd create "Title" -d "Description here"        # With description
bd create "Title" --deps "blocks:bd-5"         # With dependency
bd create "Title" --deps "bd-5,bd-6"           # Multiple deps (default: blocks)
bd create "Title" --parent bd-10               # Child of epic bd-10
bd create -f issues.md                         # Bulk create from markdown
```

**Types:** `bug`, `feature`, `task`, `epic`, `chore`
**Priority:** `0`=critical, `1`=high, `2`=medium (default), `3`=low, `4`=backlog

### `bd list` - Query Issues

```bash
bd list                                        # All issues
bd list -s open                                # By status
bd list -p 0                                   # Critical priority only
bd list -t bug -s open                         # Open bugs
bd list --title "login"                        # Title contains "login"
bd list --id "bd-1,bd-5,bd-10"                 # Specific IDs
bd list -n 10                                  # Limit to 10 results
bd list --json | jq '.[] | .id'                # JSON for scripting
bd list --format dot > deps.dot                # Graphviz output
```

**Status:** `open`, `in_progress`, `blocked`, `closed`

### `bd close` / `bd reopen`

```bash
bd close bd-1                                  # Close single
bd close bd-1 bd-2 bd-3                        # Close multiple
bd close bd-1 -r "Fixed in commit abc123"      # With reason (recommended)
bd reopen bd-1                                 # Reopen closed issue
bd reopen bd-1 -r "Needs more work"            # With reason
```

### `bd dep` - Dependencies

```bash
# Add dependency (bd-2 blocks bd-1, i.e., bd-1 depends on bd-2)
bd dep add bd-1 bd-2                           # Default type: blocks
bd dep add bd-1 bd-2 -t related                # Soft relationship
bd dep add bd-1 bd-2 -t discovered-from        # Tracking origin

bd dep remove bd-1 bd-2                        # Remove dependency
bd dep tree bd-1                               # What blocks bd-1
bd dep tree bd-1 --reverse                     # What depends on bd-1
bd dep tree bd-1 --format mermaid              # Mermaid.js output
bd dep cycles                                  # Detect circular deps
```

**Types:** `blocks` (hard), `related` (soft), `parent-child`, `discovered-from`

### `bd comments`

```bash
bd comments bd-1                               # List comments
bd comments bd-1 --json                        # JSON format
bd comments add bd-1 "Comment text"            # Add comment
bd comments add bd-1 -f notes.txt              # From file
```

### Common Patterns

```bash
# Find ready work (open + no blockers)
bd ready

# Find in progress work
bd list -s in_progress

# Close with context for future reference
bd close bd-5 -r "Implemented in src/handler.go, tested with go test ./..."

# Track discovered work during implementation
bd create "Edge case: empty input" --deps "discovered-from:bd-5"
```

## bd Workflow Lifecycle

**IMPORTANT:** bd is not just for tracking—it's the first step in any work.

### The Complete Flow

```
User requests work
       ↓
   bd create         ← BEFORE starting work
       ↓
   git commit        ← Commit ticket creation immediately
       ↓
   bd update --status in_progress
       ↓
   ... do the work ...
       ↓
   bd close          ← Close FIRST (updates .beads/)
       ↓
   git add && git commit   ← Commit code + .beads/ together
```

### Rules

1. **Commit ticket creation immediately**: After `bd create`, always commit `.beads/` right away
2. **Ticket updates are case-by-case**: Status changes, priority updates, etc. don't require immediate commits
3. **Commit closure with the work**: Close the issue, then commit code AND `.beads/` together

### Why This Order Matters

- **Create commits preserve intent**: Ticket exists in git even if work is interrupted
- **Updates are transient**: Status changes during work don't need individual commits
- **Closure is atomic with work**: Code changes + issue closure in one commit for traceability

# Notes

When executing commands the following guidance enalbes smoother workflow withour requiring
approval for commands.

The following commands do not require user approval:

- Bash(bd:*)
- Bash(wildcat:*)
- Bash(go build:*)
- Bash(go test:*)
- Bash(go doc:*)
- Bash(gopls:*)
- Bash(jq:*)

- Bash(git status:*)
- Bash(git add:*)
- Bash(git commit:*)
- Bash(git log:*)
- Bash(git diff:*)
- Bash(git show:*)

- Bash(git submodule:*)
- Bash(git lfs:*)
- Bash(git checkout:*)
- Bash(git branch:*)
- Bash(git remote:*)
- Bash(git fetch:*)
- Bash(git pull:*)
- Bash(git merge:*)
- Bash(git tag:*)
- Bash(git rev-parse:*)

- WebSearch
- WebFetch(domain:github.com)
- WebFetch(domain:github.io)
- WebFetch(domain:microsoft.github.io)
- WebFetch(domain:raw.githubusercontent.com)
- WebFetch(domain:go.dev.googlesource.com)

- Bash(tree:*)
- Bash(find:*)
- Bash(dirname:*)
- Bash(grep:*)
- Bash(ls:*)

When executing commands that take a -C <dir>, avoid that pattern and instead
cd into the target dir.  This will enable the command to be used without approval.

NEVER use `go run -` or `cat <<'EOF' | go run -` patterns. They don't work.
To test Go code, write a proper test file and use `go test`.

**Testing Go code**: Prefer writing a temporary `*_test.go` file in the appropriate
package directory, then run `go test` and delete the file. Avoid `cat > /tmp/test.go`
patterns because:
- They require approval every time
- Package imports often fail due to path issues
- Proper test files integrate with existing package context

**When tests aren't suitable** (e.g., need real project context, testing harness is
overkill): Add temporary print statements to the code, build with `go build`, and
run `./wildcat` locally. This gives real answers quickly without test fixtures.
Remove the prints when done.

## File Operations

ALWAYS use built-in tools (Edit, Write, Read) instead of shell patterns like:
- `cat >> file << 'EOF'`
- `echo "..." >> file`
- `sed -i ...`

Built-in tools don't require extra approvals and are faster. The Edit tool
handles insertions, replacements, and deletions cleanly.

## Dogfooding Wildcat

**Use wildcat to develop wildcat.** This is critical for finding bugs, improving
UX, and understanding what's missing. Every time you need to understand code,
find symbols, or explore the codebase - use wildcat instead of grep/find.

A stable version of `wildcat` is installed in PATH:
- `wildcat ...` - stable version, always works
- `./wildcat ...` - local build (after `go build -o wildcat .`)

Examples:
```bash
wildcat search "DeadCode"                    # find symbols
wildcat package internal/golang              # understand a package
wildcat deadcode internal/lsp                # find dead code
wildcat tree internal/commands/deadcode.Execute  # trace call graphs
```

When something doesn't work well or output is confusing, create a ticket.
