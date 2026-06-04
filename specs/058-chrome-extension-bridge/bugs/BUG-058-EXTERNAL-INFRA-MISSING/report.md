# Report: BUG-058-EXTERNAL-INFRA-MISSING

## Summary

Bug filed 2026-06-03 by `bubbles.plan` (via operator directive that
`done_with_concerns` is invalid in this repo's governance regime) to formally
track the four external-infrastructure gaps that block spec 058 from reaching
`done`. Spec 058 was transitioned from `done_with_concerns` to `blocked` in the
same run; this packet exists to surface the blockers individually for operator
triage.

No implementation, test, or validation phases have run on this bug packet — it
is discovery/documentation only.

## Discovery Evidence

- **Blocker 1 evidence:** `extensions/chrome-bridge/test/e2e/` directory does
  not exist (confirmed 2026-06-03). Already catalogued upstream as
  `DI-058-01-playwright-uds` + `DI-058-03` in
  `../../state.json.certification.followUps[]`.
- **Blocker 2 evidence:** `./smackerel.sh test integration` does not stand up
  a Postgres-backed harness today; Scope 2 `PostgresDedupStore.ResolveOrPublish`
  race-loss row and Scope 5 admin aggregation row are unit-tier only.
- **Blocker 3 evidence:** No shared HTMX admin layout / nav fragment / auth
  gating helper exists in the repo; Scope 5 explicitly deferred the HTMX
  rendering layer for `/admin/extension/devices` (JSON handler is mounted at
  `internal/api/router.go`).
- **Blocker 4 evidence:** SCN-058-019 sideload-by-docs walkthrough has only
  the 8-step runbook in `docs/Operations.md`; no automation path.

## Routing

Status: `open`. Severity: `blocker` (each of the four).
Owner: operator triage required to assign per-blocker ownership and resolution
order.

Resolution paths are catalogued in `bug.md` and mirrored into
`../../state.json.blockingDependencies[]` for machine-readability.

## Next Required Owner

`null` — operator triage required. No autonomous follow-up. Spec 058 stays
`blocked` until all four blockers are individually resolved (or AC-4 is
explicitly accepted as manual-only).
