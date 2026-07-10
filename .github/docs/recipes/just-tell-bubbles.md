# Recipe: Just Tell Bubbles

> *"Decent. I can see how all this fits together."*

`/bubbles.goal` is the **universal execution endpoint** for one outcome. You do not need to know which agents or workflows it will require — describe the result and constraints in plain English.

## How It Works

Goal resolves the outcome and then executes any required workflows and specialist phases in its own top-level runtime:

| Input Type | What Happens | Example |
|-----------|-------------|---------|
| **One outcome** | Goal resolves and composes whatever modes/agents are necessary | `/bubbles.goal improve the booking feature` |
| **"Continue"** | Goal resumes active outcome state and preserves its mode transitions | `/bubbles.goal continue` |
| **One known mode** | Workflow executes exactly that root mode | `/bubbles.workflow specs/042 mode: full-delivery` |
| **Several timed goals** | Sprint prioritizes the goal queue under one clock | `/bubbles.sprint minutes: 120` |
| **Framework ops or routing advice** | Super resolves or executes the framework action | `/bubbles.super doctor` |

## Examples

```
# Describe one outcome — goal figures out the workflows and agents
/bubbles.goal  improve the booking feature to be competitive
/bubbles.goal  fix the calendar bug in page builder
/bubbles.goal  take multi-tenant booking search from idea to validated delivery
/bubbles.goal  harden the product until the active release is genuinely ready

# Continue from where you left off
/bubbles.goal  continue
/bubbles.goal  fix all found

# Several goals under a time budget
/bubbles.sprint  minutes: 120
1. Fix calendar sync
2. Improve booking search
3. Validate release readiness

# Exactly one workflow mode
/bubbles.workflow  specs/042 mode: full-delivery tdd: true
/bubbles.workflow  specs/042-catalog-assistant mode: full-delivery
/bubbles.workflow  011-037 mode: harden-to-doc

# Domain-owned workflow families
/bubbles.bug  mode: fix calendar sync
/bubbles.releases  v2.0
/bubbles.train  status --all-trains
```

## What The Newer Workflow Improvements Feel Like As A User

| You Type | What You Get |
|----------|--------------|
| `/bubbles.goal  <outcome>` | Any required modes and specialists composed toward one result |
| `/bubbles.workflow  mode: brainstorm for <idea>` | One exploration mode without code |
| `/bubbles.bug  mode: fix <bug>` | Domain-owned reproduce/fix/verify workflow |
| `/bubbles.goal  continue` | Resume the active outcome and its current workflow state |
| `/bubbles.workflow  <feature> mode: full-delivery` | Keep looping through implementation, tests, quality, validation, and audit until truly green |

The planning improvements are mostly artifact-driven:

- **Design Brief** appears at the top of `design.md`
- **Capability Foundation** appears in spec/design/scopes when a request implies reusable providers, adapters, connectors, channels, variants, or shared UI surfaces
- **Execution Outline** appears at the top of `scopes.md`
- **Objective Research Pass** runs automatically inside brownfield modes instead of requiring a separate user command

## When To Use Direct Agents Instead

| Situation | Use |
|-----------|-----|
| Framework ops, advice, command recommendations without execution | `/bubbles.super` |
| Exactly one workflow mode | `/bubbles.workflow <target> mode: <mode>` |
| Several goals under a time budget | `/bubbles.sprint minutes: <N>` |
| Single-iteration work-picking with type filter | `/bubbles.iterate type: tests` |
| Explicit surgical specialist work on a known scope | `/bubbles.implement`, `/bubbles.test`, etc. |
| Bug documentation from scratch | `/bubbles.bug` |

If recap, status, or handoff identifies one active mode, continue through that mode's authorized runner. If it only identifies an outcome, use `/bubbles.goal`.

## The Delegation Graph

```
/bubbles.goal <outcome>
  │
  ├─ runSubagent(super) → resolve intent and authorized runner
  ├─ execute granted mode contract(s) directly
  ├─ runSubagent(phase owners) → collect result envelopes
  └─ loop until the outcome converges or a real blocker remains

/bubbles.workflow <target> mode: <one-mode>
  └─ execute exactly one root mode through its phase owners
```

## Related Recipes

- [Ask the Super First](ask-the-super-first.md) — for framework ops and advice
- [Resume Work](resume-work.md) — for continuing from a saved session
- [New Feature](new-feature.md) — for building from scratch
