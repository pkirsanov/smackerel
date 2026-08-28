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
| R-007 | specs/069-assistant-http-transport has one evidence block the lint heuristic scores 1/2 | residue | specs/069-assistant-http-transport/report.md (block ending ~line 171) | open | bubbles.docs | The block is a GENUINE quoted excerpt of historical e2e output; it scores only the HTTP-status signal because the test filename in it carries no directory prefix. It is not closable honestly today: adding a `$ `-prefixed command would fabricate an invocation nobody ran, and wrapping it in the `bubbles:evidence-legitimacy-skip` markers would mislabel real evidence as an illustration. Close it by re-running that e2e scenario and replacing the excerpt with fresh output captured alongside its actual command. UPDATE 2026-08-28: spec 069 artifact lint now reports 0 issues, but this row is NOT closed - the block is unchanged. A certifying window was added to report.md during the 069 reconciliation, and the lint SKIPS every evidence block preceding that marker (evidence_prewindow_skipped). The heuristic no longer sees this block; it was not fixed. Do not read the green lint as closure | 2026-08-04 | 2026-08-04 |
| R-008 | Three gaps findings on spec 069 are recorded but unrouted | residue | specs/069-assistant-http-transport/report.md#gaps-evidence--bubblesgaps-2026-08-04 | open | bubbles.test | GAP-069-G01: tests/e2e/assistant/http_capture_test.go:72,106 bail out via behavior-conditional `t.Skipf` on `!env.CaptureRoute`, bypassing the SCN-069-A06 live-stack assertions. GAP-069-G02 (-> bubbles.implement): 4 design-declared observability metrics have no non-test occurrences. GAP-069-G03 (-> bubbles.design): design declares error code `auth_invalid`, which appears nowhere; the adapter emits `auth_required` for both missing and invalid tokens | 2026-08-04 | 2026-08-04 |
| R-009 | `status: done` across the spec portfolio is not guard-backed - 7 of 8 sampled done specs fail their own state-transition guard | residue | sampled 2026-08-28 with seed 11 over specs/*/state.json where status==done | open | bubbles.spec-review | Measured failureCount per sampled done spec: 061-conversational-assistant 46, 059-google-keep-live-mode 27, 074-capture-as-fallback-policy 23, 078-cross-surface-surfacing-prioritizer 23, 068-structured-intent-compiler 18, 025-knowledge-synthesis-layer 17, 024-design-doc-reconciliation 8, 104-universal-ask-self-knowledge 0 (0 only because it was repaired the same day). Where the cause was actually investigated it was GATE DRIFT rather than fabrication - 061's 32 findings are stale Test Plan paths left by its own ratified SCOPE-04 rework, and 069's 16 were a ledger never written for phases whose evidence sections exist and name their agent. So this is NOT an accusation that the work is missing. It IS a warning that the 325 done count has largely not been re-checked against current gates, and any sweep that trusts status==done is trusting an unverified number. Next: run the guard across all done specs to get the true distribution rather than an 8-spec sample, then classify each failing spec as drift (artifact refresh) or substance (real gap), and route only the substance ones | 2026-08-28 | 2026-08-28 |
