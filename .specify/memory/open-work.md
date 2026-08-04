# Open Work Register

This file is COMMITTED on purpose. A record of open work that lives only in a
chat transcript, a terminal scrollback, or an uncommitted file is lost at
exactly the moment it is needed — when the session ends.

Render it with:

```
bash .github/bubbles/scripts/cli.sh open-work
```

## What belongs here

Only **residue**: work that was noticed and never filed. It has no spec, no bug,
and no improvement entry, so nothing else in the repository knows it exists.

Rows for specs, bugs, and improvements are **derived on every run** from
`state.json` (via `work-tracker-project.sh`) and `improvements/INDEX.md`. Do not
author them here. Writing a status into this table that another artifact already
owns creates a second source of truth, and the two will disagree.

## Rules

- A residue row MUST carry both a `next-owner` and a `next-action`. A row nobody
  owns, or whose next step is "finish the thing", does not survive the next
  session and fails `open-work --lint`.
- `kind` must be `residue`. Any other value is a lint defect.
- `id` must be unique, so a row can be removed unambiguously when it closes.
- **Closed rows are DELETED, not tombstoned.** A row disappears when its work is
  done or when it graduates into a spec, bug, or improvement — at which point
  the derived projection covers it. This table answers "what is still open"; a
  growing tail of closed rows destroys that answer. Git history is the audit
  trail for what was removed and when.

## Residue

| id | title | kind | ref | state | next-owner | next-action | opened | last-seen |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| R-001 | Shared test stack accumulates state across packages, so some tests pass alone but fail in a full run | residue | tests/integration/openknowledge, TestOpenKnowledge_P95SLAUnderToolLoad | open | bubbles.test | Reproduce by running the full integration suite twice without `down` between runs, then give the affected packages an ephemeral per-package store instead of the shared stack | 2026-08-03 | 2026-08-03 |
| R-002 | TestConcurrentInvocationIsolation_BS018 is an intermittent race, not a deterministic failure | residue | tests/stress/agent | open | bubbles.test | Run the test under `-race -count=20` to confirm cross-invocation trace-arg leakage at 200 concurrent invocations, then file a bug with the captured race report | 2026-08-03 | 2026-08-03 |
| R-003 | Assistant turn times out against the live stack in e2e (context deadline exceeded on POST /api/assistant/turn) | residue | tests/e2e/assistant, tests/e2e/legacy_retirement | open | bubbles.stabilize | Confirm whether the commodity base model (gemma3:4b) changes the timeout profile, then either raise the e2e turn budget in SST or fix the slow path the spans identify | 2026-08-03 | 2026-08-03 |
| R-004 | PWA experience assets serve Cache-Control no-store instead of immutable digests | residue | TestExperienceAssetsExposeImmutableHeadersExactDigestsAndNetworkOnlyProtectedRoutes | open | bubbles.implement | Set immutable Cache-Control on content-hashed asset routes in the web layer and keep no-store for protected routes only | 2026-08-03 | 2026-08-03 |
| R-005 | Open-knowledge gather path has no tool-capable model in the generic commodity base | residue | config/smackerel.yaml assistant.open_knowledge.llm_model_id | open | bubbles.design | Decide whether the commodity base should name a small tool-capable model with a measured memory profile, or whether a tool-capable gather model stays adapter-injected only; record the decision in the SST comment | 2026-08-03 | 2026-08-03 |
| R-006 | Two specs carry Gate G022 artifact-lint defects (missing gaps and harden phase records) | residue | specs/069-assistant-http-transport, specs/031-live-stack-testing | open | bubbles.gaps | Run the gaps and harden specialists against both specs so the phase records are produced by real runs; do not hand-author phase records, which is the fabrication G022 exists to catch | 2026-08-03 | 2026-08-03 |
