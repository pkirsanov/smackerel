# SCOPE-106-02: Canonical Catalog And Exact Route Inventory

Links: [spec.md](../../spec.md) | [design.md](../../design.md) | [scope index](../_index.md) | [report.md](report.md)

**Status:** Not Started
**Scope-Kind:** runtime-behavior
**Tags:** foundation:true
**Depends On:** SCOPE-106-01

## Outcome

One generated `ProductExperienceCatalog` is the presentation identity and route-binding source for every renderer. It inventories current route, navigation, capability, audience, renderer, deep-link, service-worker, manifest, API-client, and test consumers without inventing endpoints or treating route presence as readiness.

## Gherkin Scenarios

```gherkin
Scenario: SCN-106-003 Navigation inventories agree
  Given the current server routes PWA pages Card routes and capability declarations are inventoried
  When the catalog and renderer projections are generated
  Then every active leaf has one exact registered browser route and one stable surface identity
  And labels order hierarchy current paths audience and renderer support agree
  And route-free groups have no guessed href
  And Lists Meals and Expenses remain unavailable until exact existing API and browser-route ownership is proven
```

## Implementation Plan

1. Add the required `product_experience` compiled-config block and typed catalog with stable ID, label, kind, parent, order, capability ID, audience, exact href or null, current paths, renderer support, local-view identity, and discoverability policy.
2. Generate the PWA catalog and server projection inputs from the same compiled catalog; handwritten `appShellNav` extras and `appnav.js::ITEMS` remain active only until shadow parity and cutover scopes.
3. Build `ExperienceRouteValidator` over the actual server router inventory, PWA static-page inventory, Card deep links, web manifest, service worker, redirects, and stable test hooks. Reject duplicates, cycles, unknown parents/capabilities/audiences, leaf-without-route, group-with-href, and unregistered current path.
4. Produce an exact API and browser-route inventory for Lists, Meals/Recipes, and Expenses from current router/contracts and their owning features. Record only observed methods/paths/owners; absent browser route or incomplete workflow remains an unavailable leaf with null href.
5. Preserve current bindings from design: `/digest`, `/assistant`, `/pwa/`, `/`, `/knowledge` plus existing Wiki paths, `/cards` and Card children, `/recommendations`, `/pwa/connectors.html`, existing Photos children, `/notifications`, `/settings`, and authorized current admin/model tools. Bind `/knowledge/graph` only after spec 105 registers it.
6. Define generated golden projection fixtures containing content-free IDs/order/parents/hrefs only; no session scope, evidence ID, user content, readiness fact, or target detail enters the catalog.

## Consumer Impact Sweep

Inventory server navigation, PWA navigation, Card local navigation, breadcrumbs, redirects, deep links, native forms, HTMX targets, request helpers, web manifest shortcuts, service-worker assets, browser history, generated clients, stable test hooks, docs, config, and acceptance manifests. Every rename or grouping change retains the existing URL or an explicit compatible redirect; stale-reference scans are blocking.

## Shared Infrastructure Impact And Rollback

The config compiler and route inventory are protected shared surfaces. Canaries prove existing route registration, config generation, PWA static assets, Card routes, auth routes, and non-UI runtime startup before generated consumers activate. Rollback disables generated consumption and restores the prior renderer release while preserving the catalog artifact for diagnosis; it never guesses routes or activates an unbound leaf.

## Change Boundary

**Allowed:** product-experience config/compiler/types, route and consumer inventory tooling, generated content-free renderer catalog, exact current route manifests, focused tests.

**Excluded:** new browser routes or product-data APIs, domain handler changes, readiness derivation, shell cutover, Lists/Meals/Expenses implementation, foreign packets, spec 079, deployment, knb, CCManager, and release claims.

## Test Plan

| ID | Test Type | Category | File/Location | Scenario | Exact Test Title / Behavior | Command | Live System |
|---|---|---|---|---|---|---|---|
| XP106-02-U | Unit | `unit` | `internal/experience/catalog_test.go` | SCN-106-003 | `TestProductExperienceCatalogRejectsCyclesDuplicatesGuessedRoutesAndUnknownCapabilities` | `./smackerel.sh test unit --go` | No |
| XP106-02-I | Integration | `integration` | `tests/integration/experience/route_inventory_test.go` | SCN-106-003 | `TestCatalogMatchesRealServerPWAAndCardRouteInventoriesExactly` | `./smackerel.sh test integration` | Yes |
| XP106-02-A | E2E API regression | `e2e-api` | `tests/e2e/product_experience_catalog_e2e_test.go` | SCN-106-003 | `Generated catalog binds only registered authorized browser destinations and route-free groups` | `./smackerel.sh test e2e` | Yes |
| XP106-02-W | E2E UI regression | `e2e-ui` | `web/pwa/tests/coherent_catalog.spec.ts` | SCN-106-003 | `catalog projection exposes exact hierarchy while unbound Work leaves have no fabricated link` | `./smackerel.sh test e2e-ui` | Yes |
| XP106-02-C | Consumer regression | `functional` | `internal/experience/consumer_inventory_test.go` | SCN-106-003 | `TestExperienceConsumerInventoryContainsNoStaleNavigationRedirectManifestServiceWorkerOrTestTarget` | `./smackerel.sh check` | No |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-106-003 Navigation inventories agree`: one required catalog generates equivalent server/PWA labels, order, hierarchy, current paths, exact registered destinations, audience, renderer support, and active state; route-free groups and unproven Work leaves have no guessed href.
- [ ] Exact consumer inventory and compatibility mapping preserve all current URLs, forms, deep links, hooks, assets, clients, docs, and tests.
- [ ] Lists, Meals, and Expenses have observed API/browser-route ownership or remain unavailable with null href; no endpoint or parent route is guessed.
- [ ] Catalog/config canaries and rollback protect current routing without creating a second readiness authority.

#### Test Evidence - 5 Rows / 5 Items

- [ ] XP106-02-U passes with current-session evidence in `report.md#xp106-02-u`.
- [ ] XP106-02-I passes against actual route inventories in `report.md#xp106-02-i`.
- [ ] XP106-02-A passes through real route/auth behavior in `report.md#xp106-02-a`.
- [ ] XP106-02-W passes without interception in `report.md#xp106-02-w`.
- [ ] XP106-02-C passes the complete consumer/stale-reference scan in `report.md#xp106-02-c`.

#### Build Quality Gate

- [ ] Config generation, exact route inventory, source locking, consumer trace, no-invented-endpoint scan, check, lint, format, artifact lint, traceability, rollback, and directly affected route/config documentation checks pass with zero warnings.
