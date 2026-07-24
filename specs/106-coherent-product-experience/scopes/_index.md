# Spec 106 Scope Index

Planning authority: [spec.md](../spec.md) and [design.md](../design.md). Execution evidence belongs in each scope's `report.md`; user acceptance remains in [uservalidation.md](../uservalidation.md).

## Execution Outline

### Phase Order

1. **SCOPE-106-01 — Source-locked visual assets and appearance foundation:** establish the shared token, typography, icon, asset-integrity, appearance-cookie, CSP, and service-worker contract without changing active navigation.
2. **SCOPE-106-02 — Canonical catalog and exact route inventory:** compile one product-surface hierarchy, validate current route bindings, inventory every consumer, and keep Lists, Meals, and Expenses unbound until exact existing contracts are proven.
3. **SCOPE-106-03 — Truthful state and feedback foundation:** provide one renderer-neutral availability, content-state, authenticated-request, privacy-clear, and mutation-feedback component model without deriving domain truth.
4. **SCOPE-106-04 — Shared shell shadow adapters and canaries:** project the catalog through legacy/server, PWA, and Card chrome in shadow-contract mode with independent high-fan-out canaries and an atomic rollback path.
5. **SCOPE-106-05 — Shared shell cutover and compatibility:** replace independent navigation authorities together, preserve every supported deep link and hook, and complete the consumer impact sweep without a static optimistic fallback.
6. **SCOPE-106-06 — Search, Today, Digest, and synthesis composition:** compose repaired owner read models and progressive Search behavior after BUG-002-006, BUG-002-007, and BUG-004-004 are complete.
7. **SCOPE-106-07 — Assistant and Capture composition:** compose paired Assistant outcomes and Capture states after BUG-073-006 and the authenticated Assistant owner contract are complete.
8. **SCOPE-106-08 — Knowledge and Graph shell integration:** integrate spec 105's completed explorer into Knowledge only after BUG-080 activation precedes and enables spec 105.
9. **SCOPE-106-09 — Lists, Meals, and Expenses route composition:** add Work leaves only from the exact inventory produced by SCOPE-106-02; any leaf lacking a registered browser route and real journey remains unavailable without an invented endpoint.
10. **SCOPE-106-10 — Cards shell integration:** compose BUG-083's complete Card behavior into the shared shell, seven-view IA, tokens, states, and responsive layout without taking ownership of Card domain repair.
11. **SCOPE-106-11 — Recommendations projection:** compose recommendation availability, actions, watches, provenance, and degraded outcomes only after BUG-039 establishes real provider-backed truth.
12. **SCOPE-106-12 — Sources, Photos, Drive, Models, Activity, and Admin projections:** align the remaining owner-ready projections under route-free groups and authorized utilities without adding product-data APIs.
13. **SCOPE-106-13 — Cross-surface responsive and accessibility hardening:** prove stable non-overlapping composition, light/dark/system/forced-color themes, reduced motion, typography, keyboard, screen-reader, zoom, and narrow layouts across all integrated surfaces.
14. **SCOPE-106-14 — Disposable-stack product journeys and NFR proof:** execute persistent real-stack Playwright journeys with no interception and run stress/load only for NFR-106-001 and NFR-106-002.
15. **SCOPE-106-15 — Readiness and acceptance projection integration:** consume BUG-032 and BUG-102 owner projections only after SCOPE-106-14 produces current real-journey evidence.
16. **SCOPE-106-16 — Final cross-product acceptance handoff:** re-run the complete scenario register, validate all owner evidence and consumers, and emit a content-free result without deploying, publishing managed docs, or certifying.

### New Types And Signatures

- `ProductExperienceCatalog` and generated renderer inventory: stable surface ID, hierarchy, label, audience, exact route binding, readiness capability ID, renderer support, and local-view membership.
- `ExperienceRouteValidator.validate(catalog, browserManifest) -> Result<RouteInventory, F106RouteDrift>`.
- `ExperienceAssetManifest`: source, license, immutable path, SHA-256, media type, CSP class, size, and service-worker policy for each shared byte.
- `AppearancePreferenceCodec`: closed `system|light|dark` and `comfortable|compact` benign cookie contract.
- `ExperienceProjector.project(catalog, principal, readiness) -> ExperienceProjection` with no domain-store query or readiness inference.
- `ExperienceStatePresenter.present(ownerOutcome) -> ViewState` and `MutationFeedbackPresenter.present(ownerOutcome) -> MutationState` with unknown combinations failing closed.
- `AuthenticatedRequestAdapter`: common 401 privacy-clear/safe-return and 403 access-denied behavior over existing transports.
- Stable DOM contract: `data-experience-version`, `data-product-navigation`, `data-projection-digest`, `data-surface-id`, `data-parent-surface-id`, `data-capability-availability`, `data-view-state`, `data-operation-state`, and `data-experience-settled`.
- No new product-data API is introduced; dependency-owned readiness, Graph, Search, Digest, synthesis, Assistant, recommendation, Card, and acceptance contracts are consumed only after their owners deliver them.

### Validation Checkpoints

- **After SCOPE-106-01:** asset digest/source/license/CSP/service-worker checks, appearance codec tests, contrast/computed-style checks, and source-lock canary pass before any renderer consumes shared bytes.
- **After SCOPE-106-02:** catalog schema, cycle/order/audience validation, exact route inventory, generated server/PWA parity, and stale-reference scans pass before shell adapters begin.
- **After SCOPE-106-03:** state contradiction, false-empty, privacy-clear, 401/403, duplicate-submit, and authoritative read-back contract tests pass before shared presentation adoption.
- **After SCOPE-106-04:** independent login, native Search, HTMX read, HTMX mutation, Card PRG, PWA auth, logout/replay, and service-worker canaries pass before user-visible shell cutover.
- **After SCOPE-106-05:** both renderers expose an identical projection and every existing deep link or explicit redirect passes before body overlays proceed.
- **After each SCOPE-106-06 through SCOPE-106-13:** owner-specific real-stack regression suites and a cross-surface shell loop pass; owner logic is reused, never cloned into spec 106.
- **After SCOPE-106-14:** desktop, tablet, 390px, and 320px/200% zoom matrices pass in system/light/dark, reduced-motion, and forced-color modes with screenshots, bounding boxes, computed styles, accessibility snapshots, and focus assertions.
- **After SCOPE-106-14:** all implemented primary journeys have current disposable-stack evidence and NFR-106-001/002 measurements before BUG-032 or BUG-102 consumption.
- **After SCOPE-106-15:** Settings, client availability, and Admin Acceptance consume post-journey owner projections without re-derivation or mutation controls.
- **After SCOPE-106-16:** all 22 SCN-106 and 72 UX-E2E-106 contracts have direct results, and the content-free owner handoff is complete; any owner rejection leaves readiness unpromoted and acceptance blocked.

## Planning Boundaries

- Spec 106 owns only shared cross-surface product composition. The ten bug packets and spec 105 remain sole owners of their domain contracts, stores, APIs, provider logic, session issuance, Graph behavior, Card behavior, readiness evidence, and deployment acceptance.
- Allowed implementation families will be enumerated per scope. Excluded throughout: every foreign spec packet, domain repair source/tests, spec 079, deployment adapters, `knb`, CCManager, release-train configuration, and managed readiness/docs claims.
- Every active scope is `runtime-behavior`, starts `Not Started`, and keeps every Definition of Done item unchecked until current-session execution evidence exists.
- External packet dependencies are entry gates, not work delegated into these scopes. A missing owner outcome keeps only the dependent integration unavailable; it never authorizes a local approximation.

## Dependency Graph

| # | Scope | Depends On | External Entry Gates | Surfaces | Status |
|---|---|---|---|---|---|
| 01 | [Source-locked visual assets and appearance foundation](01-source-locked-visual-foundation/scope.md) | — | — | Shared assets, themes, typography, CSP, service worker | Not Started |
| 02 | [Canonical catalog and exact route inventory](02-canonical-catalog-route-inventory/scope.md) | 01 | — | Config, route manifests, navigation consumers | Not Started |
| 03 | [Truthful state and feedback foundation](03-truthful-state-feedback-foundation/scope.md) | 01, 02 | — | Shared state, auth recovery presentation, mutations | Not Started |
| 04 | [Shared shell shadow adapters and canaries](04-shared-shell-shadow-canaries/scope.md) | 01, 02, 03 | BUG-070-001 for API-backed session canaries | Server, PWA, Card chrome, shared harness | Not Started |
| 05 | [Shared shell cutover and compatibility](05-shared-shell-cutover-compatibility/scope.md) | 04 | BUG-070-001 | Server/PWA navigation, deep links, redirects | Not Started |
| 06 | [Search, Today, Digest, and synthesis composition](06-search-today-digest-synthesis/scope.md) | 05 | BUG-002-006, BUG-002-007, BUG-004-004 | Search, Today/Digest, synthesis projection | Not Started |
| 07 | [Assistant and Capture composition](07-assistant-capture-composition/scope.md) | 05 | BUG-073-006, BUG-070-001, spec 104 Scope 8 | Assistant, Today context, Capture | Not Started |
| 08 | [Knowledge and Graph shell integration](08-knowledge-graph-shell-integration/scope.md) | 05 | BUG-080-001 -> spec 105 | Knowledge, Wiki, Graph | Not Started |
| 09 | [Lists, Meals, and Expenses route composition](09-work-route-composition/scope.md) | 05 | Exact SCOPE-106-02 inventory and existing API/route owners | Work, Lists, Meals, Expenses | Not Started |
| 10 | [Cards shell integration](10-cards-shell-integration/scope.md) | 05 | BUG-083-002 Scopes 01-18 | Cards seven-view composition and old deep links | Not Started |
| 11 | [Recommendations projection](11-recommendations-projection/scope.md) | 05 | BUG-039-005 Scopes 01-08 | Recommendations, watches, provider state | Not Started |
| 12 | [Sources, Photos, Drive, Models, Activity, and Admin projections](12-sources-activity-admin-projections/scope.md) | 05 | Exact owner-ready route and journey inventory | Sources, Connectors, Photos, Drive, Models, Activity, Admin | Not Started |
| 13 | [Cross-surface responsive and accessibility hardening](13-responsive-accessibility-hardening/scope.md) | 06, 07, 08, 09, 10, 11, 12 | All integrated owner regressions current | All integrated browser surfaces | Not Started |
| 14 | [Disposable-stack product journeys and NFR proof](14-disposable-product-journeys-nfr/scope.md) | 13 | BUG-070 before API-backed acceptance; all applicable owner packets for their rows | Authenticated journey matrix, validate telemetry, NFR-106-001/002 | Not Started |
| 15 | [Readiness and acceptance projection integration](15-readiness-acceptance-projection/scope.md) | 14 | BUG-032 SCOPE-04 and BUG-102 SCOPE-04 after real journey evidence | Settings, client availability, Admin Acceptance | Not Started |
| 16 | [Final cross-product acceptance handoff](16-final-acceptance-handoff/scope.md) | 15 | BUG-102 SCOPE-05 and BUG-032 SCOPE-05 | Full scenario register, content-free owner handoff | Not Started |

## Scope Ordering Rationale

The first three scopes establish the shared capability foundation before any concrete renderer or domain overlay. SCOPE-106-04 treats the shared shell and browser harness as protected high-fan-out infrastructure, so independent canaries and rollback precede cutover. SCOPE-106-06 through SCOPE-106-12 are vertical composition slices over owner-delivered behavior, which avoids a horizontal page-by-page rewrite and allows each destination to stay truthfully unavailable until its entry gates close. SCOPE-106-13 hardens those implemented slices, SCOPE-106-14 produces real journey evidence, and only then may BUG-032/BUG-102 owner projections enter SCOPE-106-15 and the final SCOPE-106-16 handoff.

## Work-Route Ownership And Partial-Supersession (2026-07-24 harden)

- **Work group ownership.** "Work" is a route-free navigation group with a null parent href. Spec 106 owns the group's composition, label, and readiness-driven visibility as part of the shared shell (grounded in the `spec.md` UI Scenario Matrix rows "Work / Lists|Meals|Expenses", UC-106-010, and `design.md` Honest Integration Constraints). Catalog registration and navigation cutover live in SCOPE-106-02 and SCOPE-106-05; leaf composition lives in SCOPE-106-09.
- **Leaf route ownership.** Spec 106 owns the Lists, Meals/Recipes, and Expenses browser routes and user journeys, composed over the existing owner domain APIs. Those domain APIs and their owners are unchanged and are never reimplemented here. A leaf without a registered browser route and complete owner journey stays Unavailable with a null href.
- **Non-conflict with 092 and 100.** The Work grouping is a readiness-driven navigation-catalog decision covered by the declared partial-supersession of spec 100's fixed assistant-first inventory (`state.json.predecessorContractDisposition[100].superseded`). The certified artifacts of specs 100 and 092 are not edited. Cards (spec 092 domain, SCOPE-106-10) is a separate top-level surface, not a Work leaf, so there is no Work/Cards ownership conflict.
- **Mutation trust binding.** Every shell-reached Work mutation binds to BUG-070-001 `MutationTrustGuard` (server-validated session-bound anti-CSRF/Origin 403-before-mutation proof). Spec 106 consumes that enforcement and does not implement it.

## Scenario Authority And Planning Counts

- The analyst-owned `spec.md` User Scenarios define the authoritative meanings of `SCN-106-001` through `SCN-106-022`. Design-local technical prose that reused those IDs (for example the scope-local `SCN-106-003`/`SCN-106-010` coverage tags) is treated as implementation detail and does not redefine the analyst scenarios. `SCN-106-016` through `SCN-106-022` were added by the 2026-07-24T05:46 spec revision and were mapped in `scenario-manifest.json` to existing scopes and already-planned tests during the 2026-07-24 reconciliation.
- `scenario-manifest.json` maps all 22 analyst scenarios and all 72 `UX-E2E-106-*` contracts through `plannedTests`; no not-yet-authored file is listed under `linkedTests`.
- The 16 scope Test Plans contain 144 rows: 72 stable UX-E2E rows and 72 unit, integration, e2e-api, e2e-ui, functional, stress, load, canary, rollback, privacy, and owner-evidence rows.
- The scope DoD sections contain exactly 144 matching unchecked test-evidence items. Stress/load rows occur only in SCOPE-106-14 and are tied to NFR-106-001 and NFR-106-002.

## Consumer Impact Sweep

Every scope that changes shell, route labels, grouping, active-state behavior, assets, or compatibility must inventory and validate server navigation, PWA navigation, Card local navigation, breadcrumbs, redirects, native forms, HTMX targets, PWA request helpers, service-worker assets, web manifest shortcuts, generated catalog consumers, API clients, browser history, deep links, stable test hooks, docs, config, tests, and release/acceptance manifests. No current URL is renamed by inference. The old arrays, copied token sources, and stale references may be removed only after every first-party consumer has moved and the compatibility regressions pass.

## Shared Infrastructure Impact And Rollback

SCOPE-106-01 through SCOPE-106-05 modify protected shared assets, catalog/config compilation, auth/session presentation, renderer heads, navigation, service-worker cache identity, and Playwright bootstrap assumptions. Their scope packets must enumerate ordering, first-paint timing, session/context hydration, audience filtering, storage boundaries, route-active semantics, HTMX/PWA request behavior, Card PRG behavior, and settled markers. Independent canaries must exercise unchanged downstream journeys rather than validate only new fixtures. Rollback is one immutable asset/renderer release pointer swap; it does not rebuild, down-migrate, mutate domain rows, rewrite readiness evidence, restore optimistic static navigation, or edit a deployment target.

## Horizontal Plan Check

The active DAG is vertical after the three necessary shared-foundation scopes: each overlay scope delivers one complete user-facing composition over an owner contract and includes its own live browser regression. There are not three consecutive database-only, service-only, API-only, or UI-only delivery scopes; spec 106 introduces no business database layer and no product-data API layer.
