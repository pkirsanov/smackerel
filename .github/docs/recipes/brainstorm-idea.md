# <img src="../../icons/ray-lawnchair.svg" width="28"> Brainstorm an Idea

> *"Sometimes she goes, sometimes she doesn't. But you gotta think about it first."*

## Situation

You have a rough idea for a feature, improvement, or product direction and want to explore it thoroughly before committing to implementation. You want to refine the concept, analyze competitors, explore alternatives, and harden scenarios — but write zero code.

## Recipe

```
/bubbles.workflow  mode: brainstorm for <describe your idea>
```

This runs: `analyze → bootstrap → harden → finalize`

**What you get:**
- `spec.md` — Business analysis, actors, use cases, competitive landscape
- `design.md` — Technical design with alternatives explored, including a short **Design Brief** at the top
- `scopes.md` — Hardened scopes with Gherkin scenarios and DoD, including a short **Execution Outline** at the top
- A decision document — Ready for implementation in a separate session

**What you DON'T get:**
- No code written
- No tests executed
- No deployment changes

## When to Use

- "I have an idea but I'm not sure about the approach"
- "Let me think through this before building"
- "Explore alternatives before we commit"
- "I need a YC office hours session for this feature"

## Follow-Up

Once brainstorming is complete, start implementation:

```
/bubbles.workflow  specs/<NNN-feature> mode: full-delivery
```

For messy legacy code or release-candidate work, prefer:

```
/bubbles.workflow  specs/<NNN-feature> mode: improve-existing
/bubbles.workflow  specs/<NNN-feature> mode: delivery-lockdown
```

## Optional Tags

- `socratic: true` (default for brainstorm) — bounded clarification loop
- `socraticQuestions: 5` (default) — max questions before proceeding
- `grillMode: required-on-ambiguity` — pressure-test the idea before finalizing
