# AIDE: AI Development Environment Specification

## Seed Document

This document captures the insights and design principles for a new kind of specification: one designed for AI consumers rather than human developers. It emerged from building Wildcat, a Go code intelligence tool for AI agents, and discovering that traditional API specification approaches don't fit AI consumption patterns.

---

## Origin Story

### The LSP Failure

The original vision for Wildcat was a proxy into Language Server Protocol (LSP) servers. LSPs are the industry standard for code intelligence - they power IDE features like "go to definition" and "find references." The theory was: expose LSP capabilities to AIs through a simpler CLI interface.

This failed for a fundamental reason: **LSPs lie by omission.**

Specifically, gopls (the Go language server) returns partial results without indication when its index hasn't finished building. It acts confident when it shouldn't be. For a human in an IDE, this is annoying but manageable - you notice something's off, you wait, you retry. For an AI, it's catastrophic - the AI takes the partial results as truth and makes decisions based on incomplete information.

This experience crystallized a core principle: **Never silently fail.** If a tool can't provide complete results, it must say so explicitly. Partial results presented as complete results are worse than no tool at all.

### The Pivot to Native Analysis

After the LSP failure, Wildcat was rebuilt using Go's native analysis packages (`go/packages`, `go/types`, `go/ast`). This sacrificed language-agnosticism for reliability. The tool became Go-specific but trustworthy.

But the vision remained: if we could get the command surface area and output semantics right for Go, we could write a specification that other language communities could implement. Wildcat would be both a useful tool and a reference implementation.

---

## The Key Insight: AIs Have No Resistance to Change

Traditional API specifications exist because human consumers are **brittle**:

- Developers learn an API and build muscle memory
- Code is written against specific schemas
- Documentation gets bookmarked and referenced
- Breaking changes require migration effort
- Versioning exists to manage this brittleness

AIs are fundamentally different. They are **adaptive**:

- Every interaction starts fresh - no cached expectations
- They parse structure and infer meaning from context
- Field name changes don't break them (`callers` → `incoming_calls` is fine)
- New fields are discovered and used (or ignored) naturally
- There's no "muscle memory" to unlearn

This creates a new possibility: **a specification that embraces change rather than preventing it.**

### The "First Time Every Time" Principle

Every time an AI visits a codebase, it's the first time. There's no persistent memory saying "last week this package handled authentication." Every query needs to provide enough context for a completely fresh observer.

This has two implications:

1. **Outputs should be generous with context** - The AI can't fill in gaps from memory
2. **Outputs can change freely** - The AI won't notice or care that the format changed

Traditional tools optimize for returning users. AI tools should optimize for first-time users, because that's every user, every time.

---

## A New Kind of Specification

### What Traditional Specs Do

Traditional API specifications (OpenAPI, Protocol Buffers, GraphQL schemas) define **contracts**:

```
GET /symbol/{name}
Returns:
{
  "name": string,
  "kind": "func" | "type" | "var" | "const",
  "location": {
    "file": string,
    "line": integer
  },
  "callers": [...]
}
```

This is a promise: these fields will exist, in this structure, with these types. Breaking this promise requires versioning, migration guides, deprecation periods.

### What an AI Spec Should Do

An AI-oriented specification defines **guidance**:

```
symbol command

PURPOSE: Provide comprehensive information about a named symbol.

ANSWERS THE QUESTION: "Tell me everything about this symbol."

MUST PROVIDE:
- Where the symbol is defined
- What the symbol looks like (signature, shape, type)
- An indication of how the symbol is used

SHOULD PROVIDE:
- Who calls this symbol (for callable symbols)
- Who references this symbol (for all symbols)
- What interfaces this symbol relates to
- Enough context to understand usage in situ

ERROR HANDLING:
- If the symbol cannot be found, say so and suggest alternatives
- If analysis is incomplete, say what couldn't be analyzed and why
- Never return partial results as if they were complete
```

This is guidance, not a contract. It says what questions the command answers and what kinds of information should come back - but not the exact field names or JSON structure.

### The Three Layers

**Layer 1: Commands (Concrete)**

The questions you can ask are stable across languages and implementations:

| Command | Question It Answers |
|---------|---------------------|
| `project` | "What is this codebase and how is it organized?" |
| `package` | "What's in this package/module?" |
| `symbol` | "Tell me everything about this symbol." |
| `search` | "What symbols match this pattern?" |
| `tree` | "How does execution flow to/from this point?" |
| `deadcode` | "What code is not being used?" |

These are universal developer questions. Any language's tool should answer them.

**Layer 2: Semantics (Directional)**

The kinds of information in responses, without locking down structure:

- Symbol information includes: location, signature, kind, relationships
- Reference information includes: where, in what context, containing symbol
- Error information includes: what failed, why, suggestions for recovery
- Analysis metadata includes: what was examined, what was skipped

**Layer 3: Format (Flexible)**

The structure can evolve:

- Output is structured data (JSON recommended)
- Field names are self-descriptive
- The consumer parses what's present, not what's expected
- New fields can appear; obsolete fields can disappear

---

## The Training Data Consideration

If this specification enters AI training data, we want AIs to learn the **approach**, not memorize the **details**.

**Good training signal:**
> "Use `wildcat symbol` to get comprehensive information about a symbol. The output will include where it's defined, what it looks like, and how it's used. Parse the response to extract what you need."

**Bad training signal:**
> "The `wildcat symbol` command returns JSON with these exact fields: `name`, `kind`, `location.file`, `location.line`, `callers[].symbol`, `callers[].location`..."

The second creates brittleness. An AI that "knows" the schema might not actually read the output - it assumes, and assumptions break when formats evolve.

The specification should teach:
1. What questions to ask (commands)
2. What kinds of answers to expect (semantics)
3. How to read the output (parse it, adapt to it)

---

## The Spec as Prompt

Here's the breakthrough idea: **the specification is a mapping from command names to prompts.**

Each command's specification is essentially a prompt that describes:
- What the command is for
- What information should come back
- How errors should be handled
- What the AI consumer should do with the output

This makes the spec simultaneously:
1. **Documentation for AI users** - "Here's how to use this tool"
2. **Implementation guide for tool builders** - "Here's what your tool should provide"
3. **Test oracle** - "Does your implementation answer these questions adequately?"

### Example: Symbol Command Spec as Prompt

```
COMMAND: symbol <name>

FOR THE USER (AI consuming the tool):

You're asking: "Tell me everything about this symbol so I can understand
and potentially modify it."

You'll receive:
- Where the symbol is defined (file and location)
- The symbol's signature or shape
- How the symbol is used throughout the codebase
- Relationships to other symbols (what it calls, what calls it,
  what interfaces it implements)
- Contextual snippets showing usage

If the symbol isn't found, you'll get suggestions for similar symbols.
If analysis couldn't complete fully, you'll be told what's missing.

Read the response structure - field names are descriptive. Don't assume
a fixed schema; extract what you need from what's provided.

---

FOR THE IMPLEMENTER (building the tool):

Your command answers: "Tell me everything about this symbol."

You must provide:
- Definition location (file, line at minimum)
- Symbol signature/shape appropriate to your language
- Indication of symbol kind (function, type, variable, etc.)

You should provide:
- References: where the symbol is used
- Callers: for callable symbols, what invokes them
- Callees: for callable symbols, what they invoke
- Relationships: interface implementation, inheritance, composition
- Context: code snippets showing usage in situ

Error handling:
- Symbol not found: return structured error with fuzzy-match suggestions
- Ambiguous symbol: return candidates, let user clarify
- Partial analysis: indicate what succeeded and what failed
- NEVER return incomplete results as if they were complete

Output format:
- Structured data (JSON recommended)
- Self-descriptive field names
- Include metadata about analysis scope
```

---

## Design Principles

### 1. Questions Over Schemas

Define what questions the tool answers, not what data structures it returns. The questions are stable; the structures can evolve.

### 2. Guidance Over Contracts

Describe what kinds of information should be present, not exact fields and types. The AI will figure out the structure.

### 3. Explicit Over Silent

Every specification must address error handling. "What happens when X fails?" is as important as "What happens when X succeeds?" Silent failures are spec violations.

### 4. Context Over Brevity

AIs have no memory between sessions. Outputs should include enough context to understand the answer without prior knowledge. Err on the side of too much information.

### 5. Self-Describing Over Documented

Good output doesn't need external documentation. Field names should be clear. Metadata should be included. The response should explain itself.

### 6. Adaptable Over Versioned

No version numbers. The spec evolves, implementations evolve, and AI consumers adapt. The "contract" is semantic (what questions are answered) not syntactic (what fields exist).

---

## Command Surface Area

These commands represent universal developer questions. Any AIDE-compliant tool should implement them:

### project
**Question:** "What is this codebase and how is it organized?"

First command an AI should run. Provides orientation for everything else.

Must answer:
- What is this project? (name, description if available)
- What are the major components? (packages, modules, namespaces)
- What are the entry points? (main functions, exported APIs)
- How do components relate? (dependency structure)

### package
**Question:** "What's in this package/module?"

Zoom into a single organizational unit.

Must answer:
- What symbols are defined here?
- What's the public API vs internal implementation?
- What does this package depend on?
- What depends on this package?

### symbol
**Question:** "Tell me everything about this symbol."

Deep dive on a single named entity.

Must answer:
- Where is it defined?
- What does it look like?
- How is it used?
- What does it relate to?

### search
**Question:** "What symbols match this pattern?"

Find things when you don't know the exact name.

Must answer:
- What matches? (with locations)
- How good is each match? (some ranking/relevance)

Should support:
- Fuzzy matching (typo tolerance)
- Pattern matching (regex or glob)
- Filtering by kind, scope, etc.

### tree
**Question:** "How does execution flow to/from this point?"

Understand call relationships.

Must answer:
- What calls this? (callers, going up)
- What does this call? (callees, going down)

Should support:
- Depth limiting
- Scope filtering

### deadcode
**Question:** "What code is not being used?"

Find removal candidates.

Must answer:
- What's unreachable from entry points?
- Why is it considered dead? (no callers, no references, etc.)

Should indicate:
- Confidence level (definitely dead vs possibly dead)
- What entry points were considered

---

## Language-Specific Extensions

The core commands are language-agnostic. Languages may need additional commands for language-specific concepts:

- **Go:** Channel operations, goroutine analysis, interface satisfaction
- **Rust:** Lifetime analysis, trait implementations, unsafe blocks
- **Python:** Dynamic dispatch patterns, decorator chains
- **TypeScript:** Type narrowing, generic instantiation

The spec should define:
1. How to name extension commands (namespacing?)
2. How to document them (same prompt-style format)
3. How to indicate what's core vs extension

---

## Error Philosophy

This is the heart of the spec. Get this wrong and the tool is worse than useless.

### The Failure Hierarchy

**Fatal errors:** Analysis cannot proceed
- Return structured error with code, message, suggestions
- Example: "Package not found: did you mean...?"

**Partial failures:** Analysis degraded but useful
- Return results WITH explicit indication of what's missing
- Example: "References found in 8/10 packages. Could not analyze pkg/x (parse error), pkg/y (missing dependency)."

**Rendering failures:** Display issue, not analysis issue
- Embed error in output where the rendered content would be
- Example: `"signature": "<could not format: nil receiver>"`

### The Cardinal Rule

**NEVER return incomplete results as if they were complete.**

An AI that thinks it has all the callers will make decisions assuming there are no other callers. If there might be more callers that couldn't be found, SAY SO.

---

## Implementation Notes

### For Tool Builders

1. Start with the questions, not the data structures
2. Get error handling right before adding features
3. Be generous with context - your user has no memory
4. Make output self-describing
5. Test with actual AI consumers, not just unit tests

### For AI Consumers

1. Run `project` first to orient yourself
2. Read the output structure - don't assume fields
3. Check for error indicators before trusting results
4. If something seems incomplete, it might be - look for diagnostics

---

## Open Questions

1. **How do we handle language-specific extensions?** Namespaced commands? Flags? Separate specs?

2. **Should there be a machine-readable spec format?** Or is natural language the right choice for AI consumers?

3. **How do we test compliance?** What makes a tool "AIDE-compliant"?

4. **How do we version the spec itself?** If the spec can evolve, how do we communicate changes to implementers?

5. **Should the tool self-describe?** A `wildcat spec` command that outputs its own specification?

---

## Next Steps

1. Refine command definitions with real usage patterns
2. Build out error philosophy with concrete examples
3. Test spec language with actual AI consumers
4. Identify minimum viable implementation for each command
5. Draft language-specific extensions for Go
6. Explore machine-readable spec formats (if needed)

---

*This document emerged from a conversation about building code intelligence tools for AI agents. It represents early thinking on a new category of specification - one designed for adaptive consumers rather than brittle ones. The ideas here are seeds, not standards.*
