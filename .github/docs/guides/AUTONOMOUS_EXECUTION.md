# <img src="../../icons/tyrone-chain.svg" width="28"> Autonomous Execution Guide

> *"I handle things, that's what I do."* — Tyrone
> *"Inside and outside, both at once. Don't fall behind."* — Erica

---

## Overview

Bubbles provides two autonomous execution modes that run full development cycles without human intervention:

| Mode | Agent | Character | Use Case |
|------|-------|-----------|----------|
| `autonomous-goal` | `bubbles.goal` | Tyrone | Single goal → full convergence loop |
| `autonomous-sprint` | `bubbles.sprint` | Erica | Multiple goals + time budget |

Both modes build on the existing Bubbles infrastructure — phases, gates, specialist agents, and anti-fabrication policies — adding outer convergence loops and time management.

These modes are not limited to feature delivery. They can be used for features, bugs, ops and deployment cleanup, stabilization work, hardening passes, and mixed maintenance backlogs.

---

## Mode 1: Autonomous Goal (`bubbles.goal`)

### What It Does

Give Tyrone a goal. He handles everything:

1. **Understand** — Parse the goal, search codebase, find existing spec/design
2. **Plan** — Create spec → design → scopes (or verify existing ones)
3. **Execute** — Implement all scopes, run tests per scope
4. **Verify** — Full suite: unit + integration + browser E2E + chaos + validate + audit + harden + gaps + security + regression
5. **Remediate** — Fix ALL findings from step 4 (search web/docs if stuck)
6. **Optimize** — Simplify, security review, docs sync
7. **Convergence Check** — All gates pass + zero findings? Done. Otherwise loop to step 4.

### Quick Start

```
/bubbles.goal  Implement the security deposit hold/release feature

/bubbles.goal  Fix all broken E2E tests and make chaos pass

/bubbles.goal  Add webhook notification system to the booking flow

/bubbles.goal  Stabilize the runtime stack, fix deployment drift, and don't stop until validation is clean
```

### With Explicit Spec Target

```
/bubbles.goal  mode: autonomous-goal specs: 101-security-deposits
```

### Convergence Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| Max convergence iterations | 10 | Outer loop limit before forced exit with status report |
| Max identical failure retries | 3 | Same error retried before trying alternative approach |
| E2E per verify cycle | Mandatory | Browser E2E runs every iteration |
| Chaos per verify cycle | Mandatory | `bubbles.chaos` probes every iteration |
| Solution search | On block | Web/docs/codebase searched when implementation is stuck |

### Never-Stop Rules

Tyrone doesn't stop for fixable problems:

| Obstacle | Action |
|----------|--------|
| Missing spec/design | Creates them via analyst → design → plan |
| Test failures | Fixes implementation, never the test |
| Build failures | Diagnoses and fixes |
| Lint warnings | Fixes inline |
| Unknown API/pattern | Searches web/docs for examples |
| Gate failures | Identifies requirement and fulfills it |
| Chaos findings | Remediates via chaos-hardening |
| Pre-existing issues | Fixes them (zero deferral) |

**Only valid stop conditions:**
- Convergence achieved (all gates pass, zero findings)
- Max iterations reached (exits with detailed status report)
- User explicitly stops
- Fundamental impossibility (missing external resources/keys)

---

## Mode 2: Autonomous Sprint (`bubbles.sprint`)

### What It Does

Give Erica a list of goals and a time budget. She runs the show:

1. **Sprint Planning** — Parse goals, estimate effort (small/medium/large), sort by priority
2. **Execute Goals** — For each goal in priority order:
   - Check clock (enough time for this goal?)
   - Execute via `bubbles.goal` convergence loop
   - Mark complete, move to next
3. **Sprint Wrap-Up** — Report, docs sync, save state for resume

### Quick Start

```
/bubbles.sprint  minutes: 240
1. Fix the calendar sync bug
2. Add the deposit hold/release feature
3. Improve browser E2E test coverage for page builder

/bubbles.sprint  minutes: 120
1. Fix the flaky chaos probe
2. Stabilize deploy + config generation
3. Sync docs for the repaired ops flow
```

### With Explicit Priorities

```
/bubbles.sprint  minutes: 180
goals:
  - goal: Fix login redirect bug
    priority: high
    effort: small
  - goal: Implement search filters
    priority: medium
    effort: large
```

### Time Management Rules

| Rule | Behavior |
|------|----------|
| Clock check before each goal | Won't start what won't finish |
| Clock check before each scope | Granular time awareness |
| Finish current scope | Never leaves broken state |
| Dynamic reordering | Swaps in smaller goal if large one won't fit |
| Wrap-up reserve | 15 minutes reserved at end for docs/reporting |
| Goal time cap | 1.5× estimate before forced move-on |

### Effort Estimates

| Size | Minutes | Indicators |
|------|---------|------------|
| Small | 30 | Bug fix (known cause), doc update, config change, single-file refactor |
| Medium | 90 | Investigation bug fix, 1-3 new endpoints, UI component change, multi-file refactor |
| Large | 240 | New feature, DB schema change, cross-service integration, major UI overhaul |

### Sprint Report

At wrap-up, Erica produces:

```yaml
sprint_report:
  total_goals: 3
  completed: 1
  in_progress: 1
  not_started: 1
  time_budget_minutes: 240
  time_used_minutes: 225

  goals:
    - name: "Fix calendar sync bug"
      status: completed
      actual_duration_minutes: 22

    - name: "Add deposit hold/release"
      status: in_progress
      scopes_completed: [ "scope-01", "scope-02" ]
      scopes_remaining: [ "scope-03" ]

    - name: "Improve E2E coverage"
      status: not_started
      note: "Insufficient time remaining"
```

### Resume

```
/bubbles.sprint  mode: autonomous-sprint resume: true
```

---

## How Autonomous Modes Relate to Existing Infrastructure

```
┌──────────────────────────────────────────────────────┐
│                  bubbles.sprint                       │
│  (Erica: multi-goal, time-bounded)                   │
│                                                      │
│  ┌──────────────────────────────────────────────┐   │
│  │              bubbles.goal                     │   │
│  │  (Tyrone: single-goal convergence loop)       │   │
│  │                                               │   │
│  │  ┌─────────────────────────────────────────┐ │   │
│  │  │         bubbles.workflow                 │ │   │
│  │  │  (Bubbles: phase orchestrator)           │ │   │
│  │  │                                          │ │   │
│  │  │  ┌── Specialist Agents ──────────────┐  │ │   │
│  │  │  │ analyst, design, plan, implement, │  │ │   │
│  │  │  │ test, validate, audit, chaos,     │  │ │   │
│  │  │  │ harden, gaps, security, docs...   │  │ │   │
│  │  │  └───────────────────────────────────┘  │ │   │
│  │  └─────────────────────────────────────────┘ │   │
│  └──────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────┘
```

- `bubbles.sprint` applies the goal execution contract repeatedly with time management and multi-goal sequencing
- `bubbles.goal` is the universal one-outcome controller and may execute several granted modes
- `bubbles.workflow` executes exactly one root mode
- every active top-level runner invokes specialist phase owners directly
- All existing gates, policies, and anti-fabrication rules apply at every level

### Outcome-First Dispatch

Autonomous agents optimize for the user's requested outcome. If an authorized runner needs another granted mode, it resolves and executes that mode directly in the same top-level runtime; specialist phases still use `runSubagent`.

If the active top-level runtime lacks the subagent tool, return a blocked envelope naming the missing `agent` tool and intended phase owner. Never dispatch another workflow-running orchestrator as a subagent and never emulate specialist work inline.

---

## TPB Vocabulary

| Term | Meaning |
|------|---------|
| *"I handle things"* | bubbles.goal autonomous execution |
| *"On the clock"* | bubbles.sprint time-bounded execution |
| *"Convergence loop"* | Verify → remediate → verify cycle until zero findings |
| *"Sprint report"* | End-of-sprint status (done / in-progress / not-started) |
| *"Never-stop rules"* | Goal agent continues through fixable obstacles |
| *"Dynamic reordering"* | Sprint swaps in smaller goals when time is tight |
| *"Solution search"* | Web/docs/codebase search when implementation is stuck |
| *"Smoooth as silk"* | bubbles.train release-train cut/promote/rollback cycle |
| *"Trailer don't clean itself"* | bubbles.upkeep calendar-driven recurring maintenance |
| *"Same act, every stop"* | Trunk-based release-trains shipping the same code via per-train flag bundles |
| *"Same fix, every park"* | bubbles.propagate cross-train forward/backport propagation |
| *"What's in prod and dev"* | bubbles.train `status --all-trains` portfolio rollup |
| *"Prod is broken"* | incident-fastlane: stabilize diagnosis → train rollback → devops execute → validate |
| *"Framework health"* | bubbles.retro `target: framework` proposal-first self-observation |

---

## Command Aliases

| Alias | Agent | Quote |
|-------|-------|-------|
| `sunnyvale i-handle-things` | `bubbles.goal` | *"I handle things, that's what I do."* |
| `sunnyvale tyrone-got-this` | `bubbles.goal` | *"Peace. Tyrone got this."* |
| `sunnyvale on-the-clock` | `bubbles.sprint` | *"Inside and outside, both at once. Don't fall behind."* |
| `sunnyvale next-on-the-board` | `bubbles.sprint` | *"That one's done — keep both flows moving."* |

---

<p align="center">
  <em>"Peace. Tyrone got this."</em>
</p>
