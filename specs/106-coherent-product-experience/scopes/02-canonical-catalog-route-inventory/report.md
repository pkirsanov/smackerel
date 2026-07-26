# Report: SCOPE-106-02 Canonical Catalog And Exact Route Inventory

Links: [scope.md](scope.md) | [spec.md](../../spec.md) | [scope index](../_index.md)

## Summary

**In Progress — slice 1 of a multi-slice scope (TAKEOVER continuation to unblock spec-107).**

This slice builds the CHECK-ONLY foundation of the canonical catalog + its two
fast, no-live-stack tests:

- One typed `ProductExperienceCatalog` (`internal/experience/catalog.go`): a
  content-free surface identity + route-binding model (stable ID, label, kind,
  parent, order, capability ID, audiences, exact href or `""`, current paths,
  renderer support, local-view identity, discoverability policy). No session
  scope, evidence ID, user content, or readiness fact enters the type.
- A required `product_experience` compiled-config block in `config/smackerel.yaml`
  (SST) wired through `config generate` so the catalog is **generated**
  (`internal/experience/catalog.gen.json`, embedded via `//go:embed`), not
  handwritten. The handwritten `appShellNav` extras (`internal/web/appshell.go`)
  and `appnav.js::ITEMS` (`web/pwa/lib/appnav.js`) stay ACTIVE and untouched —
  the generated catalog is added ALONGSIDE them; cutover is a later scope.
- `ExperienceRouteValidator.Validate(catalog, inventory)` returning a resolved
  `RouteInventory` or `*F106RouteDrift` — rejecting duplicate IDs, cycles,
  unknown parent/capability/audience, active-leaf-without-route, group-with-href,
  guessed (unregistered) route, and unregistered current path.
- Consumer-inventory tooling (`internal/experience/consumer_inventory.go`) that
  reads the REAL server nav, PWA nav, web-manifest shortcuts, and service-worker
  static assets, and runs a blocking stale-reference scan proving every consumer
  destination resolves to a real registered route or a real PWA static asset.

**Grounded on real routes — no invented endpoints.** Every bound href was
inventoried from the actual Chi router (`internal/api/router.go`), the card web
routes (`internal/web/cardrewards.go`), and the real `web/pwa/` pages.
`Lists`, `Meals/Recipes`, and `Expenses` have API-only surfaces (`/api/lists`,
`/api/mealplan/*`, `/api/expenses/*`) with **no registered browser page**, so
their catalog leaves stay UNAVAILABLE with a null (`""`) href — no parent route
or endpoint is guessed. `Graph` (`/knowledge/graph`) is left null-href pending
spec 105 registration.

## Decision Record

- **Generated, not handwritten.** The catalog SST lives in
  `config/smackerel.yaml::product_experience`; `config generate` extracts it and
  writes `internal/experience/catalog.gen.json`, embedded by the Go package via
  `//go:embed`. This proves the catalog is a compiled-config artifact, not a Go
  literal.
- **Null-href representation.** JSON has no ergonomic "null" through the existing
  hand-rolled YAML->JSON extractor, so an absent binding is the empty string
  `""`. The validator enforces the semantics: an ACTIVE `linked_leaf` MUST have a
  non-empty registered href; a `route_group`, an unavailable leaf, and a
  dependency-pending leaf MUST have `""`.
- **`asset_manifest` intentionally omitted from this block.** SCOPE-106-01 serves
  its `ExperienceAssetManifest` from Go (`internal/web/experience_assets.go`); no
  static `web/pwa/assets/*.json` file exists, so referencing one here would be a
  fabricated path. The appearance + asset-manifest linkage stays with the
  appearance foundation scope; slice 1 keeps the block to `schema_version` +
  `surfaces` (the catalog's own concern).
- **Change boundary honored.** Only `internal/experience/**`,
  `config/smackerel.yaml`, its generator `scripts/commands/config.sh`, and this
  scope's artifacts were touched. Active nav/route/handler code is unchanged.

## Completion Statement

Not complete. SCOPE-106-02 is **In Progress**. This is slice 1 of a multi-slice
scope: it delivers the check-only foundation (typed catalog, generated
compiled-config block, route validator, consumer-inventory tooling) and its two
fast tests (XP106-02-U unit, XP106-02-C consumer/check). The three live lanes
remain honestly `[ ]` for slice 2: XP106-02-I (integration — catalog vs real
route inventories on the live stack), XP106-02-A (e2e-api), and XP106-02-W
(e2e-ui). Renderer-projection parity (SCN-106-003 "labels...agree") and the
shadow/rollback canaries are later scopes (SCOPE-106-04/05).

<!-- SLICE-1-EVIDENCE-BEGIN -->

## Test Evidence

Slice 1 executed the two fast check-only lanes below (XP106-02-U unit, XP106-02-C
consumer/check). The three live lanes (XP106-02-I integration, XP106-02-A e2e-api,
XP106-02-W e2e-ui) are honestly unchecked pending slice 2.

### XP106-02-U

**Claim Source:** executed — `./smackerel.sh test unit --go --go-run 'TestProductExperienceCatalog'`, current session 2026-07-26.

`TestProductExperienceCatalogRejectsCyclesDuplicatesGuessedRoutesAndUnknownCapabilities`
(`internal/experience/catalog_test.go`) ran under the `-run` selector and passed:
`internal/experience` reports `ok ... 0.005s` (real tests ran — NOT `[no tests to
run]`), and the whole `go test ./...` sweep finished OK with zero FAIL.

```text
$ ./smackerel.sh test unit --go --go-run 'TestProductExperienceCatalog'
[go-unit] applying -run selector: TestProductExperienceCatalog
[go-unit] starting go test ./...
+ go test -run TestProductExperienceCatalog -count=1 ./...
ok      github.com/smackerel/smackerel/cmd/core 0.179s [no tests to run]
ok      github.com/smackerel/smackerel/internal/api     0.129s [no tests to run]
ok      github.com/smackerel/smackerel/internal/experience      0.005s
ok      github.com/smackerel/smackerel/internal/web     0.175s [no tests to run]
ok      github.com/smackerel/smackerel/web/pwa/tests    0.005s [no tests to run]
[go-unit] go test ./... finished OK
```

### XP106-02-C

**Claim Source:** executed — `./smackerel.sh check` and `./smackerel.sh test unit --go --go-run 'TestExperienceConsumerInventory'`, current session 2026-07-26.

`./smackerel.sh check` (config-validate + SST sync + env-file drift + scenario-lint)
is clean, and the consumer-inventory functional test
`TestExperienceConsumerInventoryContainsNoStaleNavigationRedirectManifestServiceWorkerOrTestTarget`
(`internal/experience/consumer_inventory_test.go`) runs green under the `-run`
selector: `internal/experience` reports `ok ... 0.012s` (real test ran) and the
sweep finished OK.

```text
$ ./smackerel.sh check
config-validate: <repo-root>/config/generated/dev.env.tmp.2561990 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK

$ ./smackerel.sh test unit --go --go-run 'TestExperienceConsumerInventory'
[go-unit] applying -run selector: TestExperienceConsumerInventory
[go-unit] starting go test ./...
+ go test -run TestExperienceConsumerInventory -count=1 ./...
ok      github.com/smackerel/smackerel/internal/experience      0.012s
[go-unit] go test ./... finished OK
```

<!-- SLICE-1-EVIDENCE-END -->

## Planned Test References

**Claim Source:** not-run

Slice 2 lanes (XP106-02-I integration, XP106-02-A e2e-api, XP106-02-W e2e-ui)
require the live disposable stack and the renderer projections that slice 1 does
not build. Their concrete files/titles are in `scope.md` and root
`test-plan.json` and are not execution evidence.

## Uncertainty Declarations

- `Lists`, `Meals/Recipes`, and `Expenses` are bound as UNAVAILABLE leaves with
  `""` href because the real router registers only their `/api/*` surfaces and no
  browser page + complete journey exists yet (SCOPE-106-09 owns that).
- `Graph` is `""`-href pending spec 105 registration of `/knowledge/graph`.

## Scenario Contract Evidence

See `scenario-manifest.json` and `test-plan.json` at the spec root. SCN-106-003
is exercised for its catalog/validator + no-guessed-href + unavailable-Work
clauses by XP106-02-U and XP106-02-C in this slice; the renderer-projection-parity
clause is proven by slice 2 (XP106-02-I/A/W).

## Coverage Report

No runtime coverage threshold is claimed for this foundation slice.

## Lint/Quality

**Claim Source:** executed — `./smackerel.sh lint`, current session 2026-07-26.

`./smackerel.sh lint` = `go vet ./...` (go-lint.sh) + Python `ruff` + web asset
validation; it exited 0 (`prev lint exit=0`). `go vet ./...` emitted zero findings
(silent success), so the new `internal/experience` package vets clean as part of a
fully-clean tree-wide vet. The Python and web lanes reported (pip-install noise
elided):

```text
$ ./smackerel.sh lint
...
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
  OK: web/extension/manifest.json
=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/pwa/sw.js
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)

Web validation passed
```

No FOREIGN `lint` (go vet) offender surfaced this run. (The two previously-recorded
FOREIGN pre-existing `gofmt` offenders — `internal/api/graphapi/activation.go` and
`internal/web/handler_test.go` — are a `format --check` concern, not surfaced by
`lint`; they remain untouched and are not this scope's to fix.)

## Validation Summary

No validation or certification result is claimed.

## Audit Verdict

No audit verdict is claimed.
