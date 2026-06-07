# User Validation: BUG-058-001

**Reported by:** Owner directive — "need proper long term solution, no shortcuts/simplifications" (resolving the spec 058 blockers)
**Validated:** 2026-06-07

## Acceptance

- [x] AC-1 — `internal/web/admin` provides a reusable shared base layout + nav fragment + `AuthGate` helper in Go `html/template` (matching `agent_admin.go`; no new engine, no static-embed shortcut).
- [x] AC-2 — `GET /admin/extension/devices` renders the devices as an HTML table on the scaffold, behind `webAuthMiddleware`.
- [x] AC-3 — the page reuses the certified `extensiondevices.Store.AggregateDevices` and the same admin predicate as the JSON handler; no parallel query, no second auth primitive.
- [x] AC-4 — unauthenticated→401; non-admin→own-owner-scoped (403 when absent); admin→all; deterministic order; GET-only (405). All proven by unit tests.
- [x] AC-5 — user-influenced values are HTML-escaped (adversarial `<script>` test passes).

## Notes

This is the proper long-term solution for the spec 058 admin-UI blocker
(BLOCKER-3 of BUG-058-EXTERNAL-INFRA-MISSING), not a shortcut:

- It delivers a REUSABLE scaffold foundation (the "generalization" the blocker
  called for), not another bespoke page.
- It reuses the certified aggregation store and admin predicate — no duplicated
  query, no auth drift.
- It matches the repo's established Go `html/template` convention rather than
  introducing a new dependency.

Two deliberate no-shortcut decisions are recorded: (1) the certified,
XSS-audited `/admin/auth/tokens` static page was NOT rewritten onto the scaffold
(rewriting a working security-audited surface for cosmetic parity would be
change-for-change's-sake and a regression risk) — it is linked from the shared
nav and can adopt the scaffold later; (2) the HTML view reuses the same store as
the JSON view so the two can never diverge.

Scope boundary: this resolves BLOCKER-3 only. Spec 058 remains `blocked` pending
BLOCKER-1 (MV3 Playwright e2e — needs Chromium + the live stack) and BLOCKER-4
(CI MV3 sideload smoke — runs on CI runners), which require browser/CI execution
environments and must be validated there rather than fabricated. BLOCKER-2
(live-Postgres integration) was resolved 2026-06-05; the parent spec's stale
blocking-dependency entry is reconciled in this change.
