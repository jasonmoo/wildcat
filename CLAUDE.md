# Wildcat CLI

## Project Overview
Go CLI tool built for hackathon. Code quality is scored, and a mid-hackathon "curve ball" requirement will be introduced.

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
cmd/           # CLI commands
internal/      # Private packages
  config/      # Configuration handling
pkg/           # Public packages (if needed)
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


## Issue Tracking with bd (beads)

**IMPORTANT**: This project uses **bd (beads)** for ALL issue tracking. Do NOT use markdown TODOs, task lists, or other tracking methods.

### Why bd?

- Dependency-aware: Track blockers and relationships between issues
- Agent-optimized: JSON output, ready work detection, discovered-from links
- Prevents duplicate tracking systems and confusion

IMPORTANT: bd is the only way for Claude to persist memory across sessions.
It is important to populate tickets with all information required for any Claude
to do the work.  And it should document the work in such a way that it can be used
in forensics to understand the decisions.

### Quick Start

**Check for ready work:**
```bash
bd ready --json
```

**Create new issues:**
```bash
bd create "Issue title" -t bug|feature|task -p 0-4 --json
bd create "Issue title" -p 1 --deps discovered-from:bd-123 --json
```

**Claim and update:**
```bash
bd update bd-42 --status in_progress --json
bd update bd-42 --priority 1 --json
```

**Complete work:**
```bash
bd close bd-42 --reason "Completed" --json
```

### Issue Types

- `bug` - Something broken
- `feature` - New functionality
- `task` - Work item (tests, docs, refactoring)
- `epic` - Large feature with subtasks
- `chore` - Maintenance (dependencies, tooling)

### Priorities

- `0` - Critical (security, data loss, broken builds)
- `1` - High (major features, important bugs)
- `2` - Medium (default, nice-to-have)
- `3` - Low (polish, optimization)
- `4` - Backlog (future ideas)

### Workflow for AI Agents

1. **Check ready work**: `bd ready` shows unblocked issues
2. **Claim your task**: `bd update <id> --status in_progress`
3. **Work on it**: Implement, test, document
4. **Discover new work?** Create linked issue:
   - `bd create "Found bug" -p 1 --deps discovered-from:<parent-id>`
5. **Complete**: `bd close <id> --reason "Done"`
6. **Commit together**: Always commit the `.beads/issues.jsonl` file together with the code changes so issue state stays in sync with code state

Normal workflow is to commit .beads/issues.jsonl when a new ticket is created,
and also when a ticket is closed.  Commit the related code with the ticket changes
to ensure association is avilable for future inspection.

### Important Rules

- ✅ Use bd for ALL task tracking
- ✅ Always use `--json` flag for programmatic use
- ✅ Link discovered work with `discovered-from` dependencies
- ✅ Check `bd ready` before asking "what should I work on?"
- ❌ Do NOT create markdown TODO lists
- ❌ Do NOT use external issue trackers
- ❌ Do NOT duplicate tracking systems
