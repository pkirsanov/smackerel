# Report: BUG-058-EXTERNAL-INFRA-MISSING

## Summary

Bug filed 2026-06-03 by `bubbles.plan` (via operator directive that
`done_with_concerns` is invalid in this repo's governance regime) to formally
track the four external-infrastructure gaps that block spec 058 from reaching
`done`. Spec 058 was transitioned from `done_with_concerns` to `blocked` in the
same run; this packet exists to surface the blockers individually for operator
triage.

**Closeout (2026-06-07):** all four blockers are now discharged — BLOCKER-2
(live-Postgres integration) on 2026-06-05, BLOCKER-3 (HTMX admin scaffolding) by
`BUG-058-001`, and BLOCKER-1 (MV3 Playwright e2e) + BLOCKER-4 (sideload
automation) by `BUG-058-002` — so this umbrella's tracked blockers are all discharged (ready for
operator closeout). The owner confirmed build/deploy is local, so the two "needs
CI/infra" blockers were resolved locally rather than deferred. See
`## Resolution` below.

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

Status: `open` — all four blockers are `resolved`; this umbrella is a triage
tracker (intentionally minimal: `bug.md` + `spec.md` + `report.md` + `state.json`,
no implementation scopes of its own) and is ready for operator closeout. Severity:
`blocker` (each of the four, now resolved).
Owner history: operator triage assigned the discharging packets; resolution
order was BLOCKER-2 → BLOCKER-3 → (BLOCKER-1 + BLOCKER-4).

Resolution paths are catalogued in `bug.md` and mirrored into
`../../state.json.blockingDependencies[]` for machine-readability.

## Resolution

| Blocker | Discharged by | Date |
|---------|---------------|------|
| BLOCKER-2 live-Postgres integration | 3 live integration tests (`tests/integration/extension_dedup_race_test.go`, `extension_admin_devices_test.go`) | 2026-06-05 |
| BLOCKER-3 HTMX admin scaffolding | `BUG-058-001` (`internal/web/admin` scaffold + `GET /admin/extension/devices`) | 2026-06-07 |
| BLOCKER-1 MV3 Playwright e2e | `BUG-058-002` (`extensions/chrome-bridge/test/e2e/` harness, `./smackerel.sh test e2e-ext`) | 2026-06-07 |
| BLOCKER-4 sideload automation | `BUG-058-002` (`sideload_smoke.spec.ts`) | 2026-06-07 |

### Test Evidence

The final discharging run (BLOCKER-1 + BLOCKER-4) through the repo CLI surface:

```
$ ./smackerel.sh test e2e-ext
Running 11 tests using 1 worker
  ✓  2 …extension/ingest with the bearer token and correct artifact shape (1.0s)
  ✓  8 …e built extension sideloads and its MV3 service worker registers (530ms)
  11 passed (13.4s)
```

Earlier discharges (cited from their packets): BLOCKER-3 `BUG-058-001` —
`go test ./internal/web/admin/` 10/10 PASS; BLOCKER-2 — three live-Postgres
integration tests PASS (2026-06-05).

### Validation Evidence

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/058-chrome-extension-bridge/bugs/BUG-058-002-mv3-e2e-sideload-harness
✅ DoD completion gate passed for status 'done' (all DoD checkboxes are checked)
Artifact lint PASSED.
```

Both discharging packets (`BUG-058-001`, `BUG-058-002`) are artifact-lint clean
and carry their own Test/Validation/Audit evidence.

### Audit Evidence

```
$ grep -nE '"status": "(open|partially_resolved)"' specs/058-chrome-extension-bridge/bugs/BUG-058-EXTERNAL-INFRA-MISSING/state.json
$ echo "BLOCKERS_OPEN=$?"
BLOCKERS_OPEN=1
```

No blocker remains `open` or `partially_resolved` — all four `blockers[]`
entries are `resolved`. The parent `../../state.json.blockingDependencies[]` is
reconciled in the same change. No framework files (`.github/bubbles`) touched.

## Completion Statement

All four external-infrastructure blockers that held spec 058 in `blocked` are
discharged: BLOCKER-2 (live-Postgres integration, 2026-06-05), BLOCKER-3 (HTMX
admin scaffolding via `BUG-058-001`, 2026-06-07), and BLOCKER-1 + BLOCKER-4 (the
local MV3 Playwright e2e harness + automated sideload smoke via `BUG-058-002`,
2026-06-07). The two blockers previously thought to need CI/infra were resolved
locally per the owner's confirmation that build/deploy is local. This umbrella's tracked work is complete (all four
blockers resolved) and it is ready for operator closeout; the parent spec 058
`blockingDependencies` are reconciled to `resolved`, and any promotion of spec
058 out of `blocked` is a separate, deliberate state-transition pass.
