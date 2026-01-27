# AX: AI Experience

AX is the discipline of designing tools, interfaces, and outputs for AI agents as the primary user.

## Why AX Matters

Software tools have always been designed for humans. Even CLIs — the most "machine-friendly" interfaces — assume a human is reading the output, making decisions, and typing the next command.

AI agents are different users. They have different strengths, different constraints, and different failure modes. Tools designed for human ergonomics are often awkward or inefficient for agents. Tools designed with AX in mind unlock capabilities that neither humans nor poorly-served agents can achieve.

The opportunity: AI agents can operate at superhuman speed and scale, but only if their tools don't bottleneck them. Good AX is a force multiplier.

## How Agents Differ From Humans

| Dimension | Human | AI Agent |
|-----------|-------|----------|
| **Input processing** | Visual scanning, pattern recognition | Sequential text parsing |
| **Memory** | Persistent but lossy | Perfect within context, zero across sessions |
| **Attention** | Can skim, jump around | Processes everything in context window |
| **Interaction** | Keyboard, mouse, eyes | Text in, text out |
| **Ambiguity tolerance** | High — can guess, ask, infer | Low — needs explicit information |
| **Error recovery** | Can reason about partial info | Often fails silently or hallucinates |
| **Speed** | Seconds per action | Milliseconds per action (if not blocked) |

## AX Design Principles

### 1. Structured Over Pretty

Humans benefit from visual hierarchy, color, and whitespace. Agents benefit from consistent, parseable structure.

```
# Bad AX: optimized for human scanning
Found 3 results in src/

  → Parse (function) - parse.go:25
    Main entry point for parsing

  → parser (type) - parse.go:35
    Internal parser state

  → parseSymbol (method) - parse.go:166
    Handles symbol extraction
```

```
# Good AX: optimized for agent processing
spath.Parse          func    parse.go:25   Main entry point for parsing
spath.parser         type    parse.go:35   Internal parser state
spath.parser.parseSymbol  method  parse.go:166  Handles symbol extraction
```

The second format is greppable, splittable, and unambiguous.

### 2. Complete Over Curated

Humans get overwhelmed by too much information. Agents get confused by too little.

When a human asks "who calls this function?" they want the top 5 most relevant callers. When an agent asks, they need *all* callers — because they're going to modify the function and need to verify every call site still works.

Partial information presented as complete is the cardinal sin of AX. An agent that knows it's missing information can adapt. An agent that doesn't know is operating blind.

### 3. Actionable Over Informative

Every piece of output should answer: "What can the agent do with this?"

```
# Bad AX: informative but not actionable
Error: function not found in parse.go

# Good AX: actionable
Error: function "Prase" not found
Suggestions: spath.Parse, spath.parser.parse, golang.ParseKind
```

The agent can immediately retry with a corrected input.

### 4. Copyable Over Readable

Agents interact through copy-paste. Output that requires transformation is friction.

```
# Bad AX: human-readable paths
Found in: /home/jason/go/src/github.com/jasonmoo/wildcat/internal/commands/spath/parse.go:25

# Good AX: directly usable
Found: spath.Parse
```

The agent can feed `spath.Parse` directly into the next command. The file path requires parsing and mental model translation.

### 5. Explicit Over Conventional

Humans learn conventions and apply them automatically. Agents need explicit signals.

```
# Bad AX: relies on convention
Parse(input string) (*Path, error)  // exported, hence capitalized

# Good AX: explicit
Parse(input string) (*Path, error)  // exported: true
```

Don't make agents infer what you could state directly.

### 6. Composable Over Comprehensive

One tool that does everything is harder to use than focused tools that chain together.

```
# Bad AX: monolithic
analyze --show-callers --show-callees --show-types --format-tree --depth=3

# Good AX: composable
tree Parse --down=3 | filter --kind=func | read
```

Agents excel at chaining simple operations. Let them.

## AX Anti-Patterns

### The Chatty Response
Explanatory text that pads output without adding information. Agents pay for every token in their context window.

### The Invisible Failure
Returning partial results without indicating incompleteness. The agent proceeds with confidence based on incomplete data.

### The Pretty Table
Visual formatting that breaks parsing. ASCII art, dynamic column widths, decorative borders.

### The Assumed Context
Requiring information from a previous command without providing a way to reference it. Agents don't have persistent memory.

### The Ambiguous Reference
"The function" — which one? "The error above" — agents don't have spatial reasoning about "above."

## Measuring AX

Good AX is measurable:

- **Actions to insight**: How many commands to answer a question?
- **Copy-paste ratio**: What percentage of output is directly usable as input?
- **Error recovery rate**: When something fails, how often can the agent self-correct?
- **Context efficiency**: Information density per token of output
- **Composition depth**: How many tools can be chained before friction compounds?

## AX and Wildcat

Wildcat is built AX-first. The primary user is an AI agent working on a Go codebase.

This means:
- Semantic paths over file paths — agents think in symbols, not filesystems
- Complete reference graphs — every caller, every callee, no sampling
- Structured output — JSON available, markdown parseable
- Actionable errors — suggestions included, not just error messages
- Composable commands — `ls` discovers, `read` retrieves, `tree` traces

The filesystem is an implementation detail. The agent operates in semantic space.

## The Broader Opportunity

Most developer tools were built in an era when humans were the only users. As AI agents become primary users of these tools, there's an opportunity to rebuild the entire stack with AX in mind.

This isn't about making existing tools "AI-compatible." It's about asking: if we were designing this tool today, knowing that AI agents would be the primary users, what would we build?

The answer is usually something quite different from what exists.
