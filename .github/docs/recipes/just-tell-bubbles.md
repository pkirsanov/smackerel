# Recipe: Just Tell Bubbles

> *"Decent. I can see how all this fits together."*

`/bubbles.workflow` is the **universal entry point** for all Bubbles work. You don't need to know which agent, mode, or parameters to use — just describe what you want in plain English.

## How It Works

Workflow has a Phase -1 (Intent Resolution) that classifies your input:

| Input Type | What Happens | Example |
|-----------|-------------|---------|
| **Plain English** | Delegates to `super` for NLP resolution → gets mode + spec + tags | `/bubbles.workflow improve the booking feature` |
| **"Continue" / continuation language** | Resumes the active workflow when continuation context exists; otherwise falls back to `iterate` for work-picking | `/bubbles.workflow continue` |
| **Structured** | Skips resolution, executes directly | `/bubbles.workflow specs/042 mode: full-delivery` |
| **Framework ops** | Delegates to `super` for framework operations | `/bubbles.workflow doctor` |

## Examples

```
# Describe what you want — workflow figures out the rest
/bubbles.workflow  improve the booking feature to be competitive
/bubbles.workflow  fix the calendar bug in page builder
/bubbles.workflow  mode: brainstorm for multi-tenant booking search with competitive differentiation
/bubbles.workflow  spend 2 hours on whatever needs attention
/bubbles.workflow  harden specs 11 through 37
/bubbles.workflow  chaos test the whole system

# Continue from where you left off
/bubbles.workflow  continue
/bubbles.workflow  next
/bubbles.workflow  fix all found
/bubbles.workflow  address the rest

# Framework operations
/bubbles.workflow  doctor
/bubbles.workflow  show runtime lease conflicts
/bubbles.workflow  show status

# Structured input still works
/bubbles.workflow  specs/042 mode: full-delivery tdd: true
/bubbles.workflow  specs/042-catalog-assistant mode: delivery-lockdown
/bubbles.workflow  011-037 mode: harden-to-doc
```

## What The Newer Workflow Improvements Feel Like As A User

| You Type | What You Get |
|----------|--------------|
| `/bubbles.workflow  mode: brainstorm for <idea>` | Exploration without code, plus planning artifacts you can steer |
| `/bubbles.workflow  improve <feature>` | Objective brownfield research before design and implementation |
| `/bubbles.workflow  fix the <bug>` | Reproduce/fix/verify bug loop with the quality chain intact |
| `/bubbles.workflow  continue` | Resume the active workflow if possible; otherwise `iterate` picks the next slice |
| `/bubbles.workflow  <feature> mode: delivery-lockdown` | Keep looping through implementation, tests, quality, validation, and audit until truly green |

The planning improvements are mostly artifact-driven:

- **Design Brief** appears at the top of `design.md`
- **Execution Outline** appears at the top of `scopes.md`
- **Objective Research Pass** runs automatically inside brownfield modes instead of requiring a separate user command

## When To Use Direct Agents Instead

| Situation | Use |
|-----------|-----|
| Framework ops, advice, command recommendations without execution | `/bubbles.super` |
| Single-iteration work-picking with type filter | `/bubbles.iterate type: tests` |
| Explicit surgical specialist work on a known scope | `/bubbles.implement`, `/bubbles.test`, etc. |
| Bug documentation from scratch | `/bubbles.bug` |

If recap, status, or handoff told you what to do next, prefer feeding that recommendation back into `/bubbles.workflow` so orchestration, certification, and retries stay intact.

## The Delegation Graph

```
/bubbles.workflow <anything>
  │
  ├─ structured input → execute phases directly
  ├─ vague input     → runSubagent(super) → resolve → execute
  ├─ continuation    → resume active workflow if available → else runSubagent(iterate) → pick work → execute
  └─ framework op    → runSubagent(super) → execute op → report
```

## Related Recipes

- [Ask the Super First](ask-the-super-first.md) — for framework ops and advice
- [Resume Work](resume-work.md) — for continuing from a saved session
- [New Feature](new-feature.md) — for building from scratch
