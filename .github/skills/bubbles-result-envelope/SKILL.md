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
outcome: completed_owned | completed_diagnostic | route_required | blocked
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
provenance:                          # binding provenance (IMP-025 MR3)
  repositoryRoot: <git-toplevel-of-edited-repo>
  agentSourceRoot: <repo-slug the active agent was installed for>
  frameworkVersion: <installed Bubbles version>
workBoundary:                        # immutable task scope (IMP-100 R6, optional)
  repositoryRoots: [<repo-slug>, ...]      # the ONLY repos this task may modify
  specTargets: [specs/<NNN-feature>, ...]  # optional: the in-scope specs
  allowedPaths: [<glob>, ...]              # optional: in-scope paths (dir/**, dir/, exact)
  crossRepoPolicy: forbidden               # forbidden (default) | authorized
```

The orchestrator routes continuation envelopes through `bubbles.workflow`, not directly to a specialist.

**Binding provenance (multi-root, IMP-025 MR3).** The `provenance` block lets a resumed
session verify it is still bound to the intended repository + agent source. On resume, the
orchestrator MUST validate that the envelope's `agentSourceRoot` still matches the
`repositoryRoot` being edited (reuse `bubbles/scripts/repo-binding-preflight.sh
--repo-root <repositoryRoot> --agent-source <agentSourceRoot>`); a mismatch is a REFUSE
(the resumed run is bound to a different workspace root than the handoff assumed). Canonical
framework-source work sets `agentSourceRoot` to the framework slug and passes via
`--canonical-source`. Omitting the block is permitted only for single-root sessions where
the marker is absent (the preflight is advisory there); populate it whenever the
`.install-source.json` `targetRepoSlug` marker exists. See
[docs/guides/AI_ENVIRONMENT.md](../../docs/guides/AI_ENVIRONMENT.md) (Multi-Root Workspaces).

**Work boundary (anti-wandering, IMP-100 R6).** The optional `workBoundary` block declares
the immutable scope of the requested task so a session cannot drift into unrelated work (the
"works on repo A, starts fixing repo B" failure mode). Before touching any candidate change,
the orchestrator consults `bubbles/scripts/work-boundary-resolve.sh --feature-dir <spec>
--candidate-repo <slug> [--candidate-spec <id>] [--candidate-path <path>]`, which classifies
the candidate as `in-boundary` (proceed inline), `route-same-repo` (unrelated same-repo →
file/route a finding, never inline-fix), `route-cross-repo` (a different repo, allowed ONLY
when `crossRepoPolicy: authorized`), or `refuse-cross-repo` (a different repo under the
default `forbidden` policy → REFUSE). The block is opt-in + backward-compatible: absent it,
every candidate resolves `in-boundary`. A present-but-malformed boundary is fail-closed
(exit 2). It composes with `repo-binding-preflight.sh` (which guards the agent-source install
binding); the boundary guards the per-task ALLOWED SCOPE. Unrelated same-repo findings are
filed/routed; different-repo findings are route-only unless the user authorized a cross-repo
scenario.

## Outcome vocabulary (canonical)
State-modifying and diagnostic agents MUST end with EXACTLY ONE of these four outcomes (per `agents/bubbles_shared/validation-core.md` and `evidence-rules.md`; enforced by `bubbles/scripts/audit-result-contract-lint.sh`):
| Outcome | When to use |
|---------|-------------|
| `completed_owned` | A state-modifying agent finished its owned work: all owned DoD items `[x]` with evidence, no unresolved findings, all gates pass |
| `completed_diagnostic` | A read-only/analysis agent (audit, review, regression, security, …) finished its diagnostic pass: findings packaged for owners, no owned DoD to check |
| `route_required` | Found work that belongs to another specialist; package it as a finding with `nextRequiredOwner` |
| `blocked` | Cannot proceed; external dependency, missing input, or mechanical gate refuses |

`done_with_concerns` is **legacy read-only compatibility only** (pre-G092 specs carrying `legacyStatusCompatibility:true`); it is **not** a valid new RESULT-ENVELOPE outcome. To ship non-blocking notes, validate certifies `done` with an `observations[]` array (severity `low`/`medium`), or a diagnostic agent surfaces observation-shaped findings (`followUpOwner`/`followUpAction`) for the orchestrator to attach — see [completion-governance.md](../../agents/bubbles_shared/completion-governance.md#legacy-status-done_with_concerns). Anything warranting `high` severity is `blocked`, not a concern.

## Finding accounting (NON-NEGOTIABLE)
Every finding raised in this invocation MUST appear in EXACTLY ONE of:
- `addressedFindings` (fixed in this scope, evidence linked)
- `unresolvedFindings` (routed to owner)
- `observations[]` (non-blocking `low`/`medium` notes attached to `completed_owned` or a validate-certified `done`, per completion-governance.md — never used to launder a gate failure)

A finding that disappears between rounds is the cherry-pick anti-pattern and is blocked by the workflow agent's post-fix-cycle verification.

## Common mistakes
- **Claiming `completed_owned` while leaving unchecked DoD items** — guard rejects this.
- **A diagnostic agent emitting `completed_owned`** — read-only/analysis agents own no DoD; they emit `completed_diagnostic`.
- **Emitting `done_with_concerns` as a current outcome** — it is legacy-read-only; use `completed_owned`/`completed_diagnostic` with `observations[]` for non-blocking notes, or `blocked` for anything a gate would refuse. Never use an observation to dodge a fixable bug.
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
