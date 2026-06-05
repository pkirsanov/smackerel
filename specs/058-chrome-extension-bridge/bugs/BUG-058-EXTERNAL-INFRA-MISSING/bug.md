# BUG-058-EXTERNAL-INFRA-MISSING: Spec 058 blocked by 4 external-infrastructure gaps

**Status:** Open (partial resolution — see `## Reconcile 2026-06-05`)
**Severity:** Blocker (spec 058 cannot reach `done` until ALL blockers are resolved)
**Reported:** 2026-06-03
**Reporter:** bubbles.plan (via operator directive — `done_with_concerns` invalid in governance regime)
**Owner:** Needs operator triage to assign per-blocker
**Affected feature:** `specs/058-chrome-extension-bridge/`
**Affected DoD:** Deferred e2e-ui and live-stack integration rows across Scopes 1–5

## Summary

Operator declared `done_with_concerns` invalid in this repo's governance regime
(specs must be `done` or `blocked`). Spec 058 has been transitioned from
`done_with_concerns` to `blocked` honestly. This bug packet formalizes the four
external-infrastructure gaps that prevent completion of DoD-required e2e-ui +
live-stack integration tiers. Unit-tier behavioral coverage of all 21
SCN-058-001..021 scenarios is complete and green; only the DoD rows that depend
on infrastructure not in repo are open.

## Blockers

### Blocker 1 — F-057-V-001 Playwright harness not in repo

- **Severity:** blocker
- **Status:** open
- **Owner:** needs operator triage
- **Affected DoD surface:** SCN-058-010..015 e2e-ui rows; bookmark roundtrip
  E2E p95 60s; `auth_failure.spec.ts`; the broader e2e-ui regression row for
  Scope 3.
- **Evidence:** `extensions/chrome-bridge/test/e2e/` does not exist (verified
  2026-06-03 — no Playwright spec files, no harness).
- **Resolution path:** Land the Playwright harness under
  `extensions/chrome-bridge/test/e2e/` via the 057 follow-up scope, then
  author `bookmark_roundtrip.spec.ts` + `auth_failure.spec.ts`. After the
  harness exists, the deferred SCN-058-010..015 rows can be implemented.
- **Cross-reference:** Tracked upstream as `DI-058-01-playwright-uds` and
  `DI-058-03` in `state.json.certification.followUps[]`.

### Blocker 2 — Live-Postgres integration test harness deferred

- **Severity:** blocker
- **Status:** open
- **Owner:** needs operator triage
- **Affected DoD surface:** `PostgresDedupStore.ResolveOrPublish` race-loss
  path; Scope 2 live-stack integration rows; Scope 5 admin devices view live
  query path.
- **Evidence:** `./smackerel.sh test integration` does not stand up a
  Postgres-backed harness; current Scope 2/5 coverage is unit-tier only with
  in-memory fakes.
- **Resolution path:** Wire a Postgres-backed integration harness
  (testcontainers, or compose-based isolated test project per the
  bubbles-test-environment-isolation policy) into
  `./smackerel.sh test integration`, then add the deferred Scope 2/5
  follow-up rows that exercise the real `ON CONFLICT` upsert and admin
  aggregation query.

### Blocker 3 — HTMX admin scaffolding generalization missing

- **Severity:** blocker
- **Status:** open
- **Owner:** needs operator triage
- **Affected DoD surface:** `/admin/auth/tokens` HTMX page and the analogous
  `/admin/extension/devices` HTMX surface. The JSON handler for devices is
  mounted today (`GET /v1/admin/extension/devices` in
  `internal/api/router.go`) — only the HTMX rendering layer is missing.
- **Evidence:** No shared admin layout / nav fragment / auth gating helper
  exists; Scope 5 shipped the JSON aggregation but explicitly deferred the
  HTMX page.
- **Resolution path:** Land the shared HTMX admin scaffolding (layout, auth
  gating helper, nav fragment) in a dedicated spec, then add the per-page
  HTMX partials for tokens and devices on top.

### Blocker 4 — SCN-058-019 sideload-by-docs walkthrough automation

- **Severity:** blocker
- **Status:** open
- **Owner:** needs operator decision (manual-only acceptance vs CI smoke
  harness)
- **Affected DoD surface:** SCN-058-019 manual operator walkthrough.
- **Evidence:** Only the 8-step runbook in `docs/Operations.md` exists; no
  automated verification path.
- **Resolution path:** Operator chooses either (a) accept manual-only status
  permanently and close as `wontfix-automated, doc-validated`, or (b) build
  a CI-side Chrome MV3 sideload smoke harness that loads the signed zip,
  verifies cosign signature, and asserts the badge state matrix.

## What Is Preserved As Evidence

Unit-tier coverage of all 21 SCN-058-001..021 scenarios is the behavioral
source of truth and remains green (Go unit suites for `internal/config`,
`internal/api/connectors/extension`, `internal/connector/ingest`,
`internal/api/admin/extensiondevices`; vitest 39/39 across
`extensions/chrome-bridge/test/unit/`). The five `scopeProgress[]` entries in
`state.json.certification` are untouched — the implementation is real.

## Cross-References

- Parent spec: [`../../spec.md`](../../spec.md), [`../../scopes.md`](../../scopes.md)
- Parent report: [`../../report.md`](../../report.md) → `## Status Transition — 2026-06-03 (done_with_concerns → blocked)`
- Parent state: [`../../state.json`](../../state.json) → `blockingDependencies[]`
- Upstream follow-ups: `DI-058-01-playwright-uds`, `DI-058-03` in
  `state.json.certification.followUps[]`

## Reconcile 2026-06-05 (bubbles.goal)

Re-verified all four blockers against repo HEAD. Two blockers were stale; one is
fully resolved this session; two remain open.

### BLOCKER-1 — partially_resolved

`F-057-V-001` was DISCHARGED by spec 077 SCOPE-3 — the cross-cutting Playwright
harness landed under `web/pwa/tests/` (15+ `*.spec.ts` files,
`web/pwa/playwright.config.ts`, `./smackerel.sh test e2e-ui` dispatcher wired to
the disposable Compose project `smackerel-test-e2e-ui`). See
`specs/057-browser-login-redirect/state.json:71` for the discharge record.

Remaining work is MV3-extension-specific: a `extensions/chrome-bridge/test/e2e/`
harness that drives the actual Chrome extension (MV3 sideload + service worker
+ page-context capture) — likely scoped together with BLOCKER-4 since both need
the same MV3 automation infrastructure.

### BLOCKER-2 — resolved

`./smackerel.sh test integration` ALREADY stands up a Postgres-backed harness
(spec 037 SCOPE-10 wired the orchestrator); the misclassification was the bug
artifact's. The deferred Scope 2 + Scope 5 tests have been authored this
session and PASS on the live test stack:

- `tests/integration/extension_dedup_race_test.go::TestPostgresDedupStore_ResolveOrPublish_RaceLossReturnsWinnerArtifact`
  uses a deterministic two-phase sync (barrier + publishGate) to force BOTH
  goroutines to reach `UPDATE → ErrNoRows` before either `INSERT` runs, then
  asserts: both callers return the same `artifact_id`, exactly one
  `deduped=true` / one `deduped=false`, `visit_count=2`, dedup row points to
  the winner's artifact id, both publish callbacks fired. **PASS 0.04s.**
- `tests/integration/extension_dedup_race_test.go::TestPostgresDedupStore_ResolveOrPublish_FastPathHitIncrementsCount`
  seeds a dedup row + artifact and asserts the fast path returns
  `deduped=true`, MUST NOT invoke `publish()`, and bumps `visit_count` from 1
  to 2. **PASS 0.02s.**
- `tests/integration/extension_admin_devices_test.go::TestExtensionDevices_AggregateDevices_LiveAggregationRespectsContract`
  seeds a 5-row fixture covering: multi-row aggregation (`SUM` + `MIN` +
  `MAX`), 30-day window exclusion of stale rows, `source_id` pinning to
  `'browser-extension'` (a `'bookmarks'`-source row MUST NOT appear), and
  owner-filter scoping (non-admin sees only own owner). **PASS 0.22s.**

While re-verifying, discovered + fixed a pre-existing ML sidecar regression:
`ml/app/nats_client.py` was double-converting `ack_wait` to nanoseconds
(passed `ack_wait_seconds * 1_000_000_000` to nats-py 2.9.0's
`ConsumerConfig.ack_wait`, which is documented as `Optional[float]` in
SECONDS — the library then multiplied by 1e9 again, overflowing int64 on
the wire and producing JetStream `err_code=10025 'invalid JSON'` on every
subscribe). The ML sidecar would never reach `healthy` until 30 retries
elapsed. Two related unit tests asserted the broken nanosecond shape and
have been corrected.

### BLOCKER-3 — open (external)

No shared HTMX admin scaffolding exists (verified 2026-06-05: no
`internal/web/admin/`). Requires a dedicated spec.

### BLOCKER-4 — open (operator decision)

SCN-058-019 sideload walkthrough remains manual-only. Operator must choose
between (a) `wontfix-automated, doc-validated` or (b) building a CI-side MV3
sideload smoke harness.

### Unblock signal status

AC-5 from `spec.md` requires AC-1..AC-4 all resolved (or AC-4 explicitly
accepted as manual-only) before spec 058 may transition back from `blocked`
to `in_progress`. After this reconcile session: AC-2 satisfied; AC-1
partially satisfied (cross-cutting harness done, MV3-specific pending);
AC-3 and AC-4 still open. Spec 058 status remains `blocked` until BLOCKER-3
and the BLOCKER-1-residual / BLOCKER-4 pair are dispositioned by the
operator.

