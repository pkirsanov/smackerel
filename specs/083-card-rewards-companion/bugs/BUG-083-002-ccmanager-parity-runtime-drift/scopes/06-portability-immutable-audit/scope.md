# Slice 06: Versioned Portability And Immutable Audit

**Status:** Not Started
**Depends On:** 01 (owner-scoped schemas readable; per-domain import/export adapters are added incrementally as each domain slice lands, per design §Delivery Slices — the all-domain round-trip test runs once its imported domains are available, without relaxing all-or-none apply)
**Cross-Packet Depends On:** BUG-070-001 `MutationTrustGuard` (session-bound Origin/CSRF proof)
**Primary Parity Rows:** 7, 9 (plus kernel rows 14-errors, 15-a11y, 16-security)
**Scope-Kind:** cohesive-vertical-slice

## Cohesive Outcome

An owner or operator dry-runs, resolves conflicts for, applies, and replays canonical `v1` and legacy `ccmanager/v0` imports, exports a safe secret-free version, and searches one immutable append-only audit history for every persisted/idempotent/conflict/refused/failed/no-op outcome — with PostgreSQL the sole business truth and import/export tables mere artifacts.

## Slice Acceptance Kernel (design §Slice Acceptance Kernel)

Accepted only when the same request path proves in-band: claim-bound authz with a cross-user/wrong-role denial (legacy import requires explicit operator-selected owner after confirmation); Origin/Referer + session-bound CSRF via BUG-070-001 `MutationTrustGuard` on every cookie-authenticated import/export mutation (typed `origin_rejected|csrf_missing|csrf_stale|csrf_mismatch|accepted`); closed typed errors with prior-state preservation; audit is insert-only (UPDATE/DELETE refused); PostgreSQL read-back with all-or-none apply proof; keyboard/screen-reader semantics; 320px/200%-zoom reflow and non-color-only state; content-free validate-plane traces/metrics.

## Gherkin Scenarios

```gherkin
Scenario: SCN-083-002-07 Audit records success failure and no-op
	Given the immutable Card audit history
	When a mutation succeeds, fails, is refused, or is a no-op, and an UPDATE/DELETE against audit is attempted
	Then every outcome including failed and no-op is recorded truthfully, audit is searchable, and no history rewrite is permitted by database or route

Scenario: SCN-083-002-09 Versioned import export round-trips safely
	Given a versioned canonical v1 and a legacy ccmanager/v0 snapshot
	When the owner/operator dry-runs, resolves conflicts, applies, and replays, and an unknown/newer version or duplicate/partial-invalid record is submitted
	Then apply is transactional all-or-none and idempotent, unknown/newer version and downgrade are refused, no secret field is exported, and an equivalent snapshot hash proves round-trip fidelity
```

## Implementation Plan

1. Consume owner/version/receipt + audit contract; bind cookie-authenticated import/export mutations to `MutationTrustGuard`; legacy import requires explicit operator-selected owner after confirmation.
2. Implement a `v1` schema registry, `ccmanager/v0` decoder isolated to import, validated dry-run, conflict plan, transactional idempotent apply, and immutable secret-free export artifact with a replay receipt.
3. Add per-domain import/export adapters incrementally without relaxing all-or-none apply; logical compensation appends versions/audit rather than rewriting history.
4. Implement insert-only `card_audit_events` with searchable trigger/status/counts; refuse UPDATE/DELETE at both database and route; expose Audit search/detail.
5. Add `/cards/import-export` and `/cards/audit` with in-flow typed states, keyboard/screen-reader parity, and no secret in DOM/URL/storage/console/export.

## Migration And Rollback

Consume Migration E (portability) and Migration D audit. Import/export tables remain artifacts, not business truth; apply is all-or-none; rollback appends compensating versions/audit rather than rewriting history; audit partition retention is an audited operation, never row editing.

## Consumer Impact Sweep

Trace import/export/audit store/service/API/web routes, `ccmanager/v0` decoder isolation, all domain export/import schema, Audit/import-export templates, navigation, deep links, docs, and existing admin Playwright hooks.

## Change Boundary

Allowed: `internal/cardrewards/**` import/export/audit models/store/service/API/web/tests, additive Migration E, isolated `ccmanager/v0` import decoder. Excluded: any runtime CCManager call, business rewrite of domain slices, shared auth internals beyond consuming `MutationTrustGuard`, CCManager source, spec 079/106.

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Immutable audit | Mixed success/failure/no-op outcomes | Search audit; attempt UPDATE/DELETE | All outcomes recorded; rewrite refused | `e2e-ui` |
| Import round-trip | v1 + legacy v0 snapshots | Dry-run, resolve, apply, replay, export | All-or-none idempotent apply; equivalent snapshot hash; no secret field | `e2e-ui` |
| Forged-CSRF / downgrade adversary | Forged token; unknown/newer version; downgrade | Submit import/export | `MutationTrustGuard` typed refusal or version refusal; no partial apply | `e2e-api` |

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Behavior / Test Title | Command | Live System |
|---|---|---|---|---|---|---|---|
| CARD06-TP01 | Import/export unit | `unit` | `internal/cardrewards/import_test.go` | SCN-083-002-09 | v1/v0 decode, dry-run/conflict plan, version/downgrade refusal, secret-field rejection | `./smackerel.sh test unit` | No |
| CARD06-TP02 | Round-trip integration | `integration` | `internal/cardrewards/import_test.go` | SCN-083-002-09 | Transactional all-or-none idempotent apply + equivalent snapshot hash on ephemeral PostgreSQL | `./smackerel.sh test integration` | Yes |
| CARD06-TP03 | Immutable audit integration | `integration` | `internal/cardrewards/store_test.go` | SCN-083-002-07 | Insert-only audit; UPDATE/DELETE refused at database; failed/no-op recorded truthfully | `./smackerel.sh test integration` | Yes |
| CARD06-TP04 | Audit search E2E API | `e2e-api` | `web/pwa/tests/cardrewards_admin.spec.ts` | SCN-083-002-07 | `TestAuditSearchRecordsAllOutcomesAndRefusesRewrite` through real router/service/store | `./smackerel.sh test e2e` | Yes |
| CARD06-TP05 | Adversarial + MutationTrustGuard | `e2e-api` | `web/pwa/tests/cardrewards_admin.spec.ts` | SCN-083-002-09 | `TestRegressionImportAllOrNoneDowngradeRefusedAndForgedCSRFFailClosed` red-before/green-after; typed CSRF states | `./smackerel.sh test e2e` | Yes |
| CARD06-TP06 | Import/export live Playwright | `e2e-ui` | `web/pwa/tests/cardrewards_admin.spec.ts` | SCN-083-002-09 | Import/export + audit keyboard/screen-reader + no-secret-in-DOM kernel, no interception | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done

#### Core Outcomes

- [ ] SCN-083-002-07 insert-only searchable audit for every outcome; UPDATE/DELETE refused at database and route; no history rewrite.
- [ ] SCN-083-002-09 versioned v1 + legacy v0 dry-run/conflict/transactional idempotent apply/replay; unknown/newer/downgrade refused; secret-free export; equivalent snapshot hash proves round-trip.
- [ ] Kernel proven in-band: `MutationTrustGuard` Origin/CSRF typed states, cross-user/operator denial, PostgreSQL all-or-none read-back, keyboard/screen-reader + no-secret-in-DOM parity.
- [ ] Migration E/D, rollback (compensating append), consumer, and change-boundary checks report zero orphan or collateral change.

#### Test Evidence - 6 Rows / 6 Items

- [ ] CARD06-TP01 import/export unit evidence is recorded.
- [ ] CARD06-TP02 round-trip PostgreSQL integration evidence is recorded.
- [ ] CARD06-TP03 immutable-audit integration evidence is recorded.
- [ ] CARD06-TP04 audit-search E2E API evidence is recorded.
- [ ] CARD06-TP05 adversarial + forged-CSRF MutationTrustGuard red-to-green evidence is recorded.
- [ ] CARD06-TP06 live no-interception import/export Playwright evidence is recorded.

#### Build Quality Gate

- [ ] Focused and broader Card checks, Migration E/D compatibility, lint, format check, artifact lint, traceability, docs alignment, and zero-warning output pass with current-session evidence.
