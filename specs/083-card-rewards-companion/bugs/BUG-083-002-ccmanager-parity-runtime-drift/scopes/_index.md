# Card Rewards Parity Runtime Scopes

Links: [spec.md](../spec.md) | [design.md](../design.md) | [report.md](../report.md) | [uservalidation.md](../uservalidation.md)

> Reconciled 2026-07-24 to the current spec.md + design.md (both revised 2026-07-24T05:46Z). The authoritative planning boundaries are the **seven cohesive vertical slices** defined in design.md `## Cohesive Vertical Slice Program` / `## Bounded Scope Seams`. The prior 18-serial-scope layout was stale: it predated the review-driven spec/design revision, contradicted the design by turning cross-cutting concerns (foundation, audit, errors, a11y, security) into separately-claimable serial scopes, and injected phantom scenarios (F01/F02) that failed manifest coverage (16 vs 19). Cross-cutting concerns are now folded into every slice's acceptance kernel, not delegated to a final scope.

## Execution Outline

### Phase Order (7 cohesive vertical slices — NOT a serial chain)

1. **Wallet identity and lifecycle** (row 1 + kernel) — first consuming slice; establishes owner/version/receipt expand shape and the BUG-070-001 `MutationTrustGuard` binding all later slices reuse.
2. **Benefits and pending lifecycle** (rows 2, 3, 10) — multi-category shared-cap offers, tiered/non-tiered selections, pending cause lifecycle; runs after the owner contract (may parallelize with 03/05).
3. **Bonus and Calendar lifecycle** (row 4 + calendar-delivery of 12) — idempotent bonus + stable Calendar UID via transactional outbox; depends only on the owner + Calendar-outbox expand shape.
4. **Optimization, history, and reports** (rows 5, 11) — immutable versions, compare/accept/restore, exclusive report states; depends on wallet/benefit read models.
5. **Sources, config, and durable operations** (rows 6, 8, 12) — source health/verify, fail-loud SST capabilities, durable operation keys/leases/dedupe; may parallelize with 02/03.
6. **Versioned portability and immutable audit** (rows 7, 9) — v1 + legacy `ccmanager/v0` dry-run/apply/replay, secret-free export, insert-only searchable audit; per-domain adapters added incrementally.
7. **Card-local composition and complete parity loop** (row 13 + cross-area confirmation of 1-16) — seven Card views in one shared shell, explicit errors, mobile/assistive parity, security fail-closed, and the full 16-row certification matrix.

### New Types And Signatures

- `ActorContext`, `CommandContext`, `CommandResult[T]`, closed `MutationState` (`persisted|idempotent|conflict|refused|failed`), `SafeCardError`, and closed read/capability states.
- Owner/version fields plus `card_command_receipts`, `card_audit_events`, `card_pending_actions`, and explicit operation availability.
- `BenefitCoverage`, `LimitPool`, `SelectionSet`, `PendingAction`, Calendar binding/outbox, source runtime/media contracts.
- `OptimizationVersion`, immutable entries/current pointer, version comparison, report status, provenance/freshness identity.
- Versioned Card export envelope, import session/conflict plan, export artifact, and `ccmanager/v0` decoder isolated to import.
- Existing ten Card deep links preserved and mapped to Today/Wallet/Benefits/Bonuses/Optimize/Sources/Audit local views.
- Cross-packet binding to BUG-070-001 `MutationTrustGuard` (typed `origin_rejected|csrf_missing|csrf_stale|csrf_mismatch|accepted`) on every cookie-authenticated Card mutation.

### Validation Checkpoints

- Slice 01 establishes the owner/version/receipt + `MutationTrustGuard` binding before any later slice mutates; its kernel (authz, CSRF, typed errors, audit, PG read-back, a11y) must pass before overlays consume it.
- Slices 02, 03, 05 depend only on the owner contract (and their own expand shape) and remain independently eligible — the plan is deliberately not one serial seven-stage chain.
- Slice 04 depends on wallet/benefit read models (01, 02); Slice 06 depends on owner-scoped schemas being readable (01) and adds domain adapters incrementally.
- Each slice exits only with its own acceptance kernel complete and its per-row happy + adversarial real-stack tests green.
- Slice 07 runs all 16 row contracts, migrations A-E, backup/restore, rollback, stress, no-interception Playwright, and cross-product regressions together before any parity claim changes; it consumes — never replaces — Slices 01-06 domain tests.

## Slice Acceptance Kernel (mandatory per slice — design §Slice Acceptance Kernel)

Every slice is accepted only when the SAME request path proves, in-band (never delegated to a final security or accessibility slice): (1) claim-bound owner/operator/system authorization with a cross-user/wrong-role denial; (2) same-origin Origin/Referer + session-bound CSRF via BUG-070-001 `MutationTrustGuard` on every cookie-authenticated mutation, including a forged-request adversary; (3) closed typed read/mutation errors, prior-authoritative-state preservation, no raw dependency/database output; (4) immutable audit for persisted/idempotent/conflict/refused/failed/partial/no-op outcomes; (5) authoritative PostgreSQL read-back on success and refused/failed no-mutation proof; (6) representative keyboard-only + screen-reader semantics; (7) representative 320px/200%-zoom reflow, focus restoration, target sizing, non-color-only state; (8) validate-plane traces + capability metrics with content-free labels.

## Cross-Packet Dependency: BUG-070-001 MutationTrustGuard (CSRF/Origin)

Every cookie-authenticated Card mutation (Slices 01-06 domain mutations and every mutation reached through the Slice 07 shell) binds to BUG-070-001's `MutationTrustGuard` — server-validated same-origin Origin/Referer plus a session-bound anti-CSRF proof that binds PASETO token ID, subject, method, route family, and expiry — returning a typed `origin_rejected|csrf_missing|csrf_stale|csrf_mismatch|accepted` outcome BEFORE service/store access, with no business-state mutation on refusal. Bearer/PASETO API clients remain non-ambient. The dependency edge is declared in `state.json.specDependsOn` (`specs/070-web-username-password-login/bugs/BUG-070-001-production-credential-session-paseto-split`) and in each mutation slice's `Cross-Packet Depends On`. This packet consumes `MutationTrustGuard`; it does not re-implement Origin/CSRF.

## Dependency Graph And 16-Row Ledger

| # | Slice | CCManager Parity Rows | Owned Scenarios | Depends On | Status |
|---|---|---|---|---|---|
| 01 | [Wallet identity and lifecycle](01-wallet-identity-lifecycle/scope.md) | 1 (+ kernel 7,14,15,16) | SCN-083-002-01 | - | Not Started |
| 02 | [Benefits and pending lifecycle](02-benefits-pending-lifecycle/scope.md) | 2, 3, 10 | SCN-083-002-02, -03, -10 | 01 | Not Started |
| 03 | [Bonus and Calendar lifecycle](03-bonus-calendar-lifecycle/scope.md) | 4 (+ 12 delivery) | SCN-083-002-04 | 01 | Not Started |
| 04 | [Optimization, history, and reports](04-optimization-history-reports/scope.md) | 5, 11 | SCN-083-002-05, -11 | 01, 02 | Not Started |
| 05 | [Sources, config, and durable operations](05-sources-config-operations/scope.md) | 6, 8, 12 | SCN-083-002-06, -08, -12 | 01 | Not Started |
| 06 | [Versioned portability and immutable audit](06-portability-immutable-audit/scope.md) | 7, 9 | SCN-083-002-07, -09 | 01 | Not Started |
| 07 | [Card-local composition and complete parity loop](07-composition-parity-loop/scope.md) | 13 + cross-area 1-16 | SCN-083-002-13, -14, -15, -16 | 01, 02, 03, 04, 05, 06 | Not Started |

Distinct owned scenarios across slices = 16 (`SCN-083-002-01`..`-16`), one owning slice each, matching scenario-manifest.json exactly (G057/G059). Cross-cutting rows 14/15/16 are proven per-slice as kernel Test-Plan rows and DoD items AND get a dedicated cross-area confirmation scenario in Slice 07 — no row disappears into a catch-all and no aggregate shell claim substitutes for a row (CARD-PROG-001, spec §Routed Design Questions).

## Migration Checkpoints

| Checkpoint | Owning Slices | Required Safety Proof |
|---|---|---|
| A - ownership, row versions, receipts | 01 | Backup/restore, explicit legacy owner, zero owner orphans, claim-bound canary, old-reader application rollback while columns remain additive. |
| B - offers, selections, lifecycle, pending, Calendar outbox | 02, 03 | Ambiguous legacy shared groups refuse merge, category/tier/bonus counts preserve identity, stable Calendar UID, application rollback before first irreducible multi-category write. |
| C - immutable optimization versions | 04 | Revision-one backfill and current pointers verify; rollback beyond first multi-version write uses a database snapshot plus prior binary, never lossy flattening. |
| D - operations, source runtime/media, immutable audit | 05, 06 | Operation/audit backfill conservative, audit UPDATE/DELETE refused, source/media provenance retained, leases recover without duplicate logical work. |
| E - portability | 06 | Import/export tables remain artifacts rather than business truth; apply is all-or-none; logical compensation appends versions/audit rather than rewriting history. |

## Smackerel Advantages - Mandatory Non-Regression

- PostgreSQL is the sole business authority with transactions, FKs, owner predicates, version checks, and explicit cascade/retention behavior.
- Lifecycle, pending cause identity, immutable history, audit, source qualification, citations, confidence, disagreement, and manual verification remain first-class.
- Shared caps are typed identities counted once; optimizer outputs remain deterministic and explainable.
- Calendar uses stable UID plus durable binding/outbox evidence; source freshness and partial failure remain truthful.
- PASETO claim ownership, scopes, `MutationTrustGuard` CSRF, strict CSP, fail-loud SST, source locking, safe URL handling, and redaction cannot regress.
- Existing Card routes and data hooks remain compatible; Card Rewards stays inside the shared Smackerel shell and never calls CCManager at runtime.
- Responsive, keyboard, screen-reader, reduced-motion, forced-colors, and light/dark/system behavior remain equivalent to desktop workflows.
- Card Rewards tracks rewards and user-entered benefits only; it stores no PAN/CVV, executes no financial transaction, and provides no financial advice.

## Shared Infrastructure Impact Sweep

Protected shared surfaces are authentication/session claims (incl. `MutationTrustGuard`), CSRF, app shell/navigation/theme, Card migration runner, scheduler bootstrap, and Card test fixtures. Every slice touching one must use a surgical adapter/extension, list affected ordering/session/owner/timing contracts, run an independent non-Card canary where appropriate, and retain an executable rollback. Broad auth, shell, scheduler, or fixture replacement is not authorized by this packet; Card mutations consume `MutationTrustGuard` rather than re-implementing Origin/CSRF.

## Consumer Impact Sweep

Every changed Card contract must trace API and web routes, service/store callers, scheduler, Calendar/source adapters, navigation, breadcrumbs, redirects, deep links, template hooks, export/import schema, docs, config, metrics/alerts, and tests. Existing route compatibility projections remain until stale-reference scans and real consumer regressions prove safe removal under a separately owned requirement. Spec 106 consumes Card outcomes/deep links but owns no Card domain row.

## Change Boundary

**Allowed file families:** `internal/cardrewards/**`, Card-specific API/web adapters/templates/styles, additive Card migrations, Card config/generator entries, Card-specific tests, and directly affected managed Card documentation.

**Excluded file families:** `CCManager/**`, spec 079, spec 106 ownership of any Card row, other specs/bug packets, Assistant/knowledge-graph/model-provider/notification runtime, deployment overlays, release-train bundles, unrelated auth/app-shell/scheduler rewrites, financial execution/advice, and any runtime file datastore.

## Impact And Trace Planning

- Observability posture is `wired` with only the `core.health` workflow; Card slices do not falsely tag Card evidence as `core.health` — they plan bounded Card metrics/log/span assertions. A Card-specific `cards.parity` trace workflow and SLO link must be introduced through the owning config/design process before `observabilityWorkflow` or SLO-evidence rows are added.
- No `testImpact` map is configured; each slice runs focused scenario checks first plus the exact broader unit/integration/e2e-api/e2e-ui/stress gates in its Test Plan.
- Numeric operational/SLA bounds are explicit SST inputs owned by the operator; stress plans (Slices 05, 07) assert supplied bounds and never invent fallback thresholds.

### Definition of Done

The packet reaches implementation completion only when Slices 01-07 are Done honoring their dependency graph; all 16 parity rows have independently traceable happy-path and adversarial real-stack evidence (no subset or aggregate shell claim closes the program); every Test Plan row has exactly one matching test-evidence DoD item; every cookie-authenticated Card mutation is proven bound to BUG-070-001 `MutationTrustGuard`; all migrations, backup/restore, rollback, consumer, security, accessibility, stress, source-freshness, Calendar-identity, import/export, and no-interception gates pass; every Smackerel advantage above remains proven; spec 106 owns no Card row; and artifact lint, traceability, regression, audit, and validate-owned certification report no unresolved finding.
