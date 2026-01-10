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
