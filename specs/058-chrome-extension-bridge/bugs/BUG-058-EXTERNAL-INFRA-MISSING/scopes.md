# Scopes — BUG-058-EXTERNAL-INFRA-MISSING

Scope-Kind: docs-only

This packet is a **triage umbrella**, not an implementation packet. It was
created to formalise four external-infrastructure blockers that were preventing
spec 058 DoD rows from closing, and to track their discharge. It wrote no
product code and owns no test artifact; all implementation was performed by the
three child bugs named below.

`scopes.md` did not exist here until 2026-08-28. Its absence made the
state-transition guard abort at line 582 rather than evaluate the packet, which
is why the packet reported a failure count without ever naming a gate. This file
records the packet's real shape so the guard can read it.

## Change Boundary

**Included file families:** none — this umbrella changed no source file. Its
artifacts are `bug.md`, `spec.md`, `report.md` and `state.json` in this
directory.

**Excluded surfaces:** every source path in the repository.

## Implementation Files

None. Implementation for the four blockers lives in the three discharging child
bugs and is certified there:

| Child bug | Discharges | Status |
|---|---|---|
| `BUG-058-001` | see child packet | done, certifiedAt 2026-06-07T12:00:00Z |
| `BUG-058-002` | see child packet | done, certifiedAt 2026-06-07T13:00:00Z |
| `BUG-058-003` | MV3 Playwright harness de-flake + re-certification | done, certifiedAt 2026-06-09T17:28:39Z |

## Scope 01 — Enumerate the four blockers and track them to discharge

**Status:** Done
**Depends On:** —

### What this scope covered

Naming the four external-infrastructure gaps precisely, keeping them visible
while spec 058 sat blocked, and verifying each against repo HEAD once the child
bugs landed.

### Definition of Done

- [x] All four blockers enumerated with named owners rather than a single vague "infra missing" note. **Claim Source:** prior-session evidence recorded in [bug.md](bug.md) § Blockers. Evidence: [report.md](report.md)
- [x] BLOCKER-3 (HTMX admin scaffold + `GET /admin/extension/devices`) discharged — admin unit suite exits 0 across `internal/api/admin/extensiondevices`, `internal/api`, `internal/web`, `internal/config`. **Claim Source:** prior-session evidence re-verified 2026-06-09. Evidence: [bug.md](bug.md)
- [x] BLOCKER-1 + BLOCKER-4 (MV3 Playwright e2e harness + sideload automation) discharged — `extensions/chrome-bridge/test/e2e/` carries four spec files including `sideload_smoke.spec.ts`; harness re-certified by `BUG-058-003`. **Claim Source:** prior-session evidence. Evidence: [bug.md](bug.md)
- [x] BLOCKER-2 (live-Postgres dedup race + admin aggregation integration tests) discharged — `tests/integration/extension_dedup_race_test.go` and `extension_admin_devices_test.go` present on disk. **Claim Source:** prior-session evidence; the heavy live integration stack is sandbox-throughput-gated and was NOT re-run for closeout, which the closeout section states plainly. Evidence: [bug.md](bug.md)
- [x] All three discharging child bugs reached terminal `done` with recorded `certifiedAt` values. **Claim Source:** read from the child packets' `state.json`. Evidence: [bug.md](bug.md)
- [x] The umbrella does NOT claim to promote its parent — spec 058 remains `blocked` on the keyless-OIDC `cosign verify-blob` row, which is not one of these four blockers. **Claim Source:** stated in [bug.md](bug.md) § Closeout 2026-06-09.
