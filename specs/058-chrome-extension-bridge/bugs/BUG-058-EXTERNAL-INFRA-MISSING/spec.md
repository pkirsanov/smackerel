# Spec: BUG-058-EXTERNAL-INFRA-MISSING — External infrastructure required to unblock spec 058

## Expected Behavior

The four external-infrastructure gaps that block completion of spec 058's
DoD-required e2e-ui + live-stack integration tiers MUST be individually
resolved (or explicitly accepted as out-of-scope) before spec 058 can
transition from `blocked` back to `in_progress` and then to `done`.

Unit-tier behavioral coverage for all 21 SCN-058-001..021 scenarios is already
complete and green — this packet does NOT relitigate that work. It scopes only
the missing infrastructure.

## Acceptance Criteria

1. **AC-1 (Playwright harness landed):** `extensions/chrome-bridge/test/e2e/`
   exists with a working Playwright harness wired into
   `./smackerel.sh test e2e-ui`, AND `bookmark_roundtrip.spec.ts` +
   `auth_failure.spec.ts` exist and pass. Unblocks SCN-058-010..015 e2e-ui
   rows. (Source blocker: F-057-V-001.)
2. **AC-2 (Live-Postgres integration harness landed):**
   `./smackerel.sh test integration` stands up an ephemeral Postgres-backed
   harness honoring `bubbles-test-environment-isolation`, AND the deferred
   Scope 2 (`PostgresDedupStore.ResolveOrPublish` race-loss path) +
   Scope 5 (admin devices view live aggregation) integration rows are
   authored and pass.
3. **AC-3 (HTMX admin scaffolding landed):** Shared HTMX admin scaffolding
   (layout, auth gating helper, nav fragment) ships in a dedicated spec,
   AND the per-page HTMX partials for `/admin/auth/tokens` and
   `/admin/extension/devices` are authored on top.
4. **AC-4 (SCN-058-019 disposition decided):** Operator decides either
   (a) accept manual-only status permanently and close that DoD row as
   `wontfix-automated, doc-validated`, OR (b) build a CI-side Chrome MV3
   sideload smoke harness and add the automated verification row.
5. **AC-5 (Spec 058 unblock signal):** When AC-1..AC-4 are all resolved (or
   AC-4 is explicitly accepted as manual-only), spec 058's
   `state.json.status` may be flipped from `blocked` back to `in_progress`
   to complete the deferred DoD rows, then to `done`.

## Out of Scope

- Re-doing the unit-tier behavioral coverage already shipped for SCN-058-001..021.
- Modifying the shipped server ingest contract, dedup keyer, MV3 client, or
  admin JSON handler.
- Re-litigating the close-out decisions captured in
  `../../report.md` `## Close-Out 2026-05-28` and `## Discovered Issues`.

## Cross-References

- Blocked spec: `../../spec.md`
- Blocked scope/DoD inventory: `../../scopes.md`
- Blocked DoD evidence catalog: `../../report.md` → `## Deferred DoD Items`
- Status transition record: `../../report.md` → `## Status Transition — 2026-06-03`
- Per-blocker details: `bug.md`

## Closeout Status (2026-06-09)

All four acceptance-criteria blockers are RESOLVED and re-verified 2026-06-09:

- **AC-1** (MV3 Playwright harness) — landed via `BUG-058-002`
  (`extensions/chrome-bridge/test/e2e/`), de-flaked + re-certified via
  `BUG-058-003`.
- **AC-2** (live-Postgres integration harness) —
  `tests/integration/extension_dedup_race_test.go` +
  `extension_admin_devices_test.go` authored 2026-06-05.
- **AC-3** (HTMX admin scaffold) — landed via `BUG-058-001`
  (`internal/web/admin` + `GET /admin/extension/devices`); BLOCKER-3 admin unit
  suite re-verified GREEN 2026-06-09.
- **AC-4** (SCN-058-019 sideload disposition) — resolved as option (b): a local
  MV3 sideload smoke (`sideload_smoke.spec.ts`) was delivered via `BUG-058-002`.

**AC-5 is governed separately and is NOT discharged by closing this umbrella.**
Spec 058's transition out of `blocked` is a distinct, deliberate pass; the
parent remains `blocked` on the keyless-OIDC `cosign verify-blob` / public-Rekor
row, which is tracked on the parent's `blockingDependencies` and is NOT one of
this umbrella's four blockers.
