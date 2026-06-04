# Sweep Round 12 — Routing Record (spec 005 gaps)

Stochastic-quality-sweep round 12 ran `gaps-to-doc` on `specs/005-phase4-expansion`. Two non-blocking findings surfaced. Spec 005 remains `done`; findings routed to owners for planning decision before remediation.

## GAP-005-G1 (medium) — Proxy-test evidence for 7 scenarios

DoD evidence for SCN-005-008b/008c/008d/011/011b/011c/013/013b cites constant-existence tests (`TestAlertType_Constants`, `TestAlert_Lifecycle`) as behavior coverage. No `Test*` matches the scenario verbs under `internal/intelligence/`.

Owner: `bubbles.plan`. Decision required: (a) author real behavior tests + update evidence, or (b) demote scenarios to Non-Goals via `bubbles.analyst` (mirrors existing R-406/R-407/SC-E11/E18/E19/E20 deferral pattern).

## GAP-005-G2 (low) — Baseline uservalidation.md on certified spec

`specs/005-phase4-expansion/uservalidation.md` contains only the bootstrap stub. Spec is certified `done` with 5 scopes.

Owner: `bubbles.validate`. Decision required: back-fill per-scope acceptance, or add explicit policy note that phase specs defer user validation to constituent feature specs / `docs/releases/`.

## Status

- Findings filed: 2 (1 medium, 1 low)
- Findings closed: 0
- Findings deferred-with-owner: 2

Routing-only record per stochastic-quality-sweep contract; remediation deferred pending owner planning decisions.
