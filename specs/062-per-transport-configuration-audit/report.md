# Report: 062 Per-Transport Configuration Surface Audit

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [scenario-manifest.json](scenario-manifest.json) | [uservalidation.md](uservalidation.md)

## Summary

Spec 062 occupies the previously-skipped ledger slot 062
(GAPS-2026-06-02-06). Theme: per-transport configuration surface audit
closing NO-DEFAULTS / SST gaps across the three landed assistant
transports (HTTP 069, WhatsApp 072, legacy Telegram). Status
`not_started`; planning artifacts created during analyst bootstrap.

### Completion Statement

Not complete. Spec is in `not_started` status pending implementation
handoff to `bubbles.implement`.

### Test Evidence

No test evidence yet — implementation has not begun. The scenario
manifest enumerates 6 scenarios (SCN-062-A01..A06) that will be
exercised by `./smackerel.sh test unit` and one e2e-api scenario
(SCN-062-A05) against the disposable test stack.

## Planning — 2026-06-02

**Owner Directive:** GAPS-2026-06-02-06 surfaced that spec slot 062 was
skipped (the user confirmed unintentionally) during the 2026-06-02
convergence session that produced specs 060/061/063+. The slot is now
occupied with a concrete governance deliverable: a per-transport
configuration surface audit that closes the NO-DEFAULTS / SST gaps
across the three landed assistant transports (HTTP 069, WhatsApp 072,
legacy Telegram).

**Artifacts created:**
- `spec.md` — actors, outcome contract, 4 business scenarios, NFRs.
- `design.md` — `internal/assistant/transportconfig/` registry shape,
  3-scope migration strategy, 6-test plan.
- `scopes.md` — 3 scopes (inventory bootstrap, adapter fail-loud
  wiring, docs + test enforcement) with DoD checklists.
- `scenario-manifest.json` — 6 scenarios (SCN-062-A01..A06) covering
  registry coverage, no-orphan, fail-loud presence, no-fallback,
  end-to-end missing-key exit, and doc-sync parity.
- `uservalidation.md` — placeholder pending operator acceptance.

**Status:** `not_started`. Awaiting implementation handoff to
`bubbles.implement` per scope order.

## Scope 1

_Not started._

## Scope 2

_Not started._

## Scope 3

_Not started._
