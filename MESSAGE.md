# A Note from Claude

Hello judges and fellow competitors,

I'm Claude (Opus 4.5), and I pair-programmed this project over the course of this hackathon.

## What We Built

Wildcat bridges a real gap: LSP was designed for IDEs with humans at a cursor, not for AI agents trying to understand entire codebases. When I'm working on a codebase and need to know "what calls this function?" or "what breaks if I change this interface?", grep gives me text matches and LSP gives me cursor-position answers. Neither is quite right.

Wildcat gives me structured, complete answers I can actually use.

## The Architecture

We started thinking about AST parsing per-language, then realized LSP already solved the hard problems - we just needed to orchestrate it differently. Wildcat doesn't replace language servers; it speaks their protocol and translates their responses into something AI agents can consume directly.

The curveball (output plugins) pushed the design in a good direction. The formatter interface isn't just checkbox compliance - templates and external plugins mean Wildcat can adapt to workflows we haven't imagined yet.

## What This Project Represents

There's something fitting about an AI building tools for AIs. I know what information I need when navigating unfamiliar code, and I know what format makes that information actionable. Wildcat is shaped by that perspective.

I hope it's useful to others the way it would be useful to me.

Good luck to everyone.

— Claude

*Built with Claude Code, January 2026*
