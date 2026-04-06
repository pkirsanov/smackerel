# Recipe: New Feature (End-to-End)

> *"Freedom 35, boys!"* — Taking a feature from idea to shipped code.

---

## The Situation

You have a feature idea (or a requirement from a stakeholder) and need to take it through the full pipeline: analysis → design → implementation → testing → docs → ship.

## Quick Start — Natural Language

**All agents accept natural language.** Just describe what you want:

```
# Easiest — one command does everything:
/bubbles.workflow  Build a real-time notification system with email and push support mode: product-to-delivery

# Or ask super for guidance first:
/bubbles.super  I want to build a notification system, what should I do?
```

## The Steps (Manual Control)

### Step 1: Discover Requirements

```
/bubbles.analyst  Build a real-time notification system with email and push support
```

If you want Bubbles to stay autonomous, stop there. If you want a bounded clarification loop first, opt in explicitly:

```
/bubbles.workflow  product-to-delivery for notification-system socratic: true socraticQuestions: 4
```

**What happens:** The analyst agent researches requirements, identifies actors, creates use cases, and writes `spec.md`.

**You'll get:** A `specs/NNN-notification-system/spec.md` with requirements, acceptance criteria, and use cases.

If UX sections are needed, run `bubbles.ux` after analyst work. If you skip that and later run `bubbles.design`, design will route back to analyst or UX instead of inventing missing sections itself.

### Step 2: Design the System

```
/bubbles.design  Create technical design for the notification system
```

**What happens:** The design agent reads the spec, creates data models, API contracts, service boundaries, and writes `design.md`.

**You'll get:** A `design.md` with architecture, schemas, and technical decisions.

### Step 3: Break Into Scopes

```
/bubbles.plan  Create scopes for the notification feature
```

**What happens:** The plan agent reads spec + design, creates implementable scopes with Gherkin scenarios, test plans, and Definition of Done checklists.

**You'll get:** `scopes.md` with 3-8 scopes, each with clear DoD.

### Step 4: Implement (Scope by Scope)

```
/bubbles.workflow  notification-system mode: delivery-lockdown
```

**What happens:** The workflow keeps driving the feature through implementation, tests, validation, and certification until the current state is legitimately clean.

Repeat for each scope, or use:

```
/bubbles.implement  Execute scope 1 of notification system
```

Use the direct specialist form only when you intentionally want a surgical single-scope step.

### Step 5: Full Pipeline (Or Do It All At Once)

If you want the orchestrator to handle the entire flow:

```
/bubbles.workflow  full-delivery for notification-system
```

Optional execution tags:

```
/bubbles.workflow  full-delivery for notification-system gitIsolation: true autoCommit: true maxScopeMinutes: 20 maxDodMinutes: 8

/bubbles.workflow  full-delivery for notification-system grillMode: required-on-ambiguity tdd: true
```

This runs all phases automatically, routing artifact changes to the correct owner when needed and consuming concrete result envelopes between phases: analyze → bootstrap → implement → test → security → docs → validate → audit → chaos → finalize.

If validation, hardening, gap analysis, stability review, or security review discovers missing planning or design artifacts, the workflow routes those changes back to `bubbles.plan`, `bubbles.design`, or `bubbles.analyst` instead of letting the diagnostic phase rewrite those files directly.

---

## Tips

- **Start small.** If the feature is large, use `/bubbles.plan` first, then `/bubbles.iterate` to work through scopes.
- **Check progress** anytime: `/bubbles.status`
- **Something wrong?** `/bubbles.workflow  notification-system mode: validate-to-doc` to check gates and route fixes correctly.
- **End of day?** `/bubbles.handoff` to save context.

---

## The Artifacts You'll End Up With

```
specs/NNN-notification-system/
├── spec.md              # What we're building (requirements)
├── design.md            # How we're building it (architecture)
├── scopes.md            # The work breakdown (DoD per scope)
├── report.md            # Evidence of execution
├── uservalidation.md    # User acceptance checklist
└── state.json           # Machine-readable progress
```
