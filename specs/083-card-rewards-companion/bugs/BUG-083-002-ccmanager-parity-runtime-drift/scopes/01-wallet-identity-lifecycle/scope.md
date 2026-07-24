# Slice 01: Wallet Identity And Lifecycle

**Status:** Not Started
**Depends On:** - (first consuming slice; establishes owner/version/receipt expand shape and the MutationTrustGuard binding all later slices reuse)
**Cross-Packet Depends On:** BUG-070-001 `MutationTrustGuard` (session-bound Origin/CSRF proof)
**Primary Parity Rows:** 1 (plus kernel rows 7-audit, 14-errors, 15-a11y, 16-security)
**Scope-Kind:** cohesive-vertical-slice

## Cohesive Outcome

An authenticated owner discovers or creates a custom/catalog card, round-trips complete metadata, activates/deactivates, previews a named cascade, deletes, reloads, and inspects the resulting audit — the full request path through router → service → PostgreSQL and back, with claim-bound ownership, version checks, and immutable audit authoritative.

## Slice Acceptance Kernel (design §Slice Acceptance Kernel)

This slice is accepted only when the SAME wallet request path proves, in-band (not delegated to a later security/a11y slice): (1) claim-bound owner/operator authorization with a cross-user/wrong-role denial; (2) same-origin Origin/Referer + session-bound CSRF via BUG-070-001 `MutationTrustGuard` on every cookie-authenticated wallet mutation, including a forged-request adversary (typed `origin_rejected|csrf_missing|csrf_stale|csrf_mismatch|accepted`); (3) closed typed read/mutation errors with prior-authoritative-state preservation and no raw dependency output; (4) immutable audit for persisted/idempotent/conflict/refused/failed/no-op wallet outcomes; (5) authoritative PostgreSQL read-back on success and proof that refused/failed preconditions did not mutate state; (6) representative keyboard-only + screen-reader semantics for the wallet read+mutation journey; (7) representative 320px/200%-zoom reflow, focus restoration, non-color-only state; (8) validate-plane traces + capability metrics with content-free labels.

## Gherkin Scenarios

```gherkin
Scenario: SCN-083-002-01 Wallet metadata round-trips and cascades safely
	Given an authenticated owner encounters an ambiguous catalog match or creates a custom card with nickname note type fee and source metadata
	When the card is saved reloaded edited deactivated reactivated and deleted after reviewing dependent effects
	Then each authoritative version and lifecycle state reads back from PostgreSQL and every outcome appends an immutable audit event
	And cancellation, stale version, cross-owner ID, delete-with-dependents, and forged-CSRF requests never cause an undeclared mutation or false success
```

## Implementation Plan

1. Consume the owner/version/receipt expand shape: `ActorContext`, `CommandContext`, `CommandResult[T]`, closed `MutationState`, and `card_command_receipts` replay safety (design §Command And Concurrency Contract).
2. Bind every cookie-authenticated wallet mutation to BUG-070-001 `MutationTrustGuard` before service/store access; a bearer/PASETO API client remains non-ambient; failure returns `CARD_CSRF_INVALID`-class typed refusal with no state change.
3. Extend claim-bound wallet CRUD: catalog/custom distinction, complete mutable metadata, source provenance, `row_version`, closed lifecycle (`active|inactive|deleted`), owner-scoped SQL predicate on every store method.
4. Deterministic discovery (exact/ambiguous/no-match); explicit separate custom-create command; delete preview with dependent counts + preview hash + one-use confirmation token; transactional soft cascade + queued Calendar cleanup.
5. Return the committed re-read row after create/edit/toggle; prove final absence after delete; retain prior state on conflict/refusal/failure; append the audit event in the same transaction.
6. Augment `/cards/wallet` and children with in-flow validating/saving/persisted/conflict/refused/failed states and named destructive confirmation; store no PAN/CVV/expiry.

## Migration And Rollback

Consume Migration A (ownership, row versions, receipts). Wallet writes stay disabled during owner/version backfill; verify prior-reader compatibility before any soft-cascade write; rollback after new lifecycle writes uses the captured database snapshot plus prior binary, never owner/version column deletion.

## Consumer Impact Sweep

Trace wallet store/service/API/web routes, discovery, offers/selections/bonuses/recommendations/Calendar references, navigation/deep links, import/export records, audit, docs, and existing wallet Playwright hooks.

## Change Boundary

Allowed: `internal/cardrewards/**` wallet/catalog-custom models/store/service/API/web/tests, additive Migration A, and direct cascade consumers. Excluded: offer/selection/bonus/optimizer implementation, shared auth internals beyond consuming `MutationTrustGuard`, `CCManager/**`, spec 079/106, and unrelated product surfaces.

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Catalog and custom creation | Ambiguous catalog + owner session | Search, choose explicit custom path, save, reload | One owner card with complete metadata/provenance and persisted feedback | `e2e-ui` |
| Versioned edit and toggle | Existing owner card | Edit note/nickname/fee/type, deactivate/reactivate, reload | Version increments and authoritative state remains visible | `e2e-ui` |
| Cascade delete | Card with dependent rows | Preview, cancel once, confirm once | Named counts, cancel no-op, final absence and audit counts | `e2e-ui` |
| Forged-CSRF / stale / cross-owner adversary | Stale version, second owner ID, forged Origin/token | Submit edits/deletes | `MutationTrustGuard` typed refusal or conflict/not-found; unchanged state; no target disclosure | `e2e-api` |

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Behavior / Test Title | Command | Live System |
|---|---|---|---|---|---|---|---|
| CARD01-TP01 | Wallet unit | `unit` | `internal/cardrewards/service_wallet_test.go` | SCN-083-002-01 | Discovery classification, metadata validation, lifecycle transitions, preview hash, cascade plan | `./smackerel.sh test unit` | No |
| CARD01-TP02 | Wallet integration | `integration` | `internal/cardrewards/store_crud_test.go` | SCN-083-002-01 | Claim-bound create/read/edit/toggle/delete + FK/cascade/audit round-trip on ephemeral PostgreSQL | `./smackerel.sh test integration` | Yes |
| CARD01-TP03 | Wallet E2E API happy | `e2e-api` | `web/pwa/tests/cardrewards_wallet.spec.ts` | SCN-083-002-01 | `TestWalletCRUDMetadataVersionAndCascadeRoundTrip` through real router/service/store | `./smackerel.sh test e2e` | Yes |
| CARD01-TP04 | Adversarial + MutationTrustGuard | `e2e-api` | `web/pwa/tests/cardrewards_wallet.spec.ts` | SCN-083-002-01 | `TestRegressionWalletAmbiguityStaleCrossOwnerAndForgedCSRFFailClosed` red-before/green-after; asserts typed `origin_rejected|csrf_missing|csrf_stale|csrf_mismatch|accepted` | `./smackerel.sh test e2e` | Yes |
| CARD01-TP05 | Wallet Playwright live | `e2e-ui` | `web/pwa/tests/cardrewards_wallet.spec.ts` | SCN-083-002-01 | `SCN-083-002-01 Regression: wallet lifecycle + cascade + keyboard/mobile kernel` with no interception | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done

#### Core Outcomes

- [ ] SCN-083-002-01 Wallet metadata round-trips and cascades safely with complete owner-scoped CRUD, lifecycle, preview, cancellation, conflict, and authoritative reload.
- [ ] Parity row 1 delivers complete claim-bound wallet CRUD/metadata with explicit catalog ambiguity and custom creation; activation and cascade-confirmed deletion return authoritative versions and preserve FK/source/audit truth.
- [ ] Kernel proven in-band: `MutationTrustGuard` Origin/CSRF (typed states), claim-bound authz cross-user denial, immutable audit for every wallet outcome, PostgreSQL read-back, and no PAN/CVV data.
- [ ] Wallet UI exposes complete pending/error/conflict/destructive states responsively with keyboard/screen-reader parity.
- [ ] Migration A, rollback, consumer, and change-boundary checks report zero orphan or collateral change.

#### Test Evidence - 5 Rows / 5 Items

- [ ] CARD01-TP01 wallet-unit evidence is recorded.
- [ ] CARD01-TP02 PostgreSQL wallet-integration evidence is recorded.
- [ ] CARD01-TP03 wallet E2E API happy-path evidence is recorded.
- [ ] CARD01-TP04 adversarial + forged-CSRF MutationTrustGuard red-to-green evidence is recorded.
- [ ] CARD01-TP05 live no-interception wallet Playwright evidence is recorded.

#### Build Quality Gate

- [ ] Focused and broader Card checks, Migration A compatibility, lint, format check, artifact lint, traceability, docs alignment, and zero-warning output pass with current-session evidence.
