---
name: bubbles-result-envelope
description: Compose a valid Bubbles result envelope at the end of every agent invocation. Use when finishing any Bubbles agent task and about to return control to the orchestrator, when routing work to another specialist via a packet, or when emitting a continuation envelope from a read-only/advisory surface. Covers outcome vocabulary, finding accounting, addressedFindings/unresolvedFindings, and next-owner routing.
---

# Bubbles Result Envelope

## Goal
Return work to `bubbles.workflow` (or another orchestrator) with a machine-readable envelope that carries enough provenance for the next routing decision, without hiding deferred findings or claiming work that did not happen.

## When to use
- End of every `bubbles.*` agent invocation
- Returning from a sub-agent dispatch
- Emitting a continuation envelope from `bubbles.recap`, `bubbles.status`, `bubbles.handoff`, `bubbles.super`
- Routing a foreign-artifact finding to its owner

## Envelope shape (terminal — completion or block)
```
## RESULT-ENVELOPE
agent: bubbles.<name>
mode: <workflow-mode>
spec: specs/<NNN-feature>
scope: SCOPE-<id>          # if applicable
outcome: completed_owned | route_required | blocked | done_with_concerns
addressedFindings:
  - <finding-id>: <one-line resolution + evidence ref>
unresolvedFindings:
  - <finding-id>: <one-line reason + suggested owner>
nextRequiredOwner: bubbles.<name>   # only when outcome=route_required or blocked
reason: <one-line explanation>
evidenceRefs:
  - specs/<NNN>/report.md#<section>
  - specs/<NNN>/scopes.md#scope-<id>
```

## Envelope shape (continuation — advisory surfaces)
For recap/status/handoff/super and read-only outputs that suggest how to continue:
```
## CONTINUATION-ENVELOPE
target: specs/<NNN-feature>
intent: <one-line natural-language goal>
preferredWorkflowMode: <mode-from-workflows.yaml>
tags: [<routing-tag>, ...]
reason: <why this is the next step>
```

The orchestrator routes continuation envelopes through `bubbles.workflow`, not directly to a specialist.

## Outcome vocabulary (canonical)
| Outcome | When to use |
|---------|-------------|
| `completed_owned` | All owned DoD items `[x]` with evidence, no unresolved findings, all gates pass |
| `route_required` | Found work that belongs to another specialist; package it as a finding with `nextRequiredOwner` |
| `blocked` | Cannot proceed; external dependency, missing input, or mechanical gate refuses |
| `done_with_concerns` | Spec-defined "ship but flag" outcome; `followUpOwner`/`followUpAction`/`followUpTarget` fields populated; NOT a deferral |

## Finding accounting (NON-NEGOTIABLE)
Every finding raised in this invocation MUST appear in EXACTLY ONE of:
- `addressedFindings` (fixed in this scope, evidence linked)
- `unresolvedFindings` (routed to owner)
- structured `followUps[]` (only under `done_with_concerns`)

A finding that disappears between rounds is the cherry-pick anti-pattern and is blocked by the workflow agent's post-fix-cycle verification.

## Common mistakes
- **Claiming `completed_owned` while leaving unchecked DoD items** — guard rejects this.
- **Marking `done_with_concerns` to dodge a fixable bug** — `done_with_concerns` is for genuinely out-of-scope follow-ups with explicit owners, not for shipping known regressions.
- **Setting `nextRequiredOwner` to `bubbles.workflow`** — workflow is the dispatcher, not the next owner. Name the actual specialist.
- **Using `@bubbles.X` in envelope or output** — the slash convention applies everywhere: `/bubbles.X` only.

## Workflow-only continuation (NON-NEGOTIABLE)
Read-only/advisory surfaces (recap, status, handoff, super, retro, recommend-first) MUST suggest `/bubbles.workflow <mode>` for continuation, not `/bubbles.implement`, `/bubbles.test`, `/bubbles.validate`, or `/bubbles.audit` directly. Direct specialist commands are valid only when the user explicitly requests a surgical invocation.

## Authoritative modules
- `agents/bubbles_shared/workflow-orchestration-core.md` — envelope schema + dispatcher contract
- `agents/bubbles_shared/workflow-delegation-core.md` — natural-language intent routing
- `agents/bubbles_shared/workflow-execution-loops.md` — per-round synchronous completion
- `agents/bubbles_shared/workflow-fix-cycle-protocol.md` — finding-set closure during remediation
- `agents/bubbles_shared/agent-common.md` — Workflow-Only Continuation section
