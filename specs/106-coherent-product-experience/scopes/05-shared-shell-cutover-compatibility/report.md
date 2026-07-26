# Report: SCOPE-106-05 Shared Shell Cutover And Compatibility

Links: [scope.md](scope.md) | [spec.md](../../spec.md) | [scope index](../_index.md)

## Summary
In Progress. ONE additive foundation slice is delivered: the generated NAVIGATION
PROJECTION data model (XP106-05-U) the shared-shell cutover will LATER consume. It
touches NO live renderer, route, or session mechanism. Everything else in this
scope — the actual atomic cutover of the server/PWA/Card navigation authorities
and the SESSION scenarios — remains unchecked and coupled-forward.
## Decision Record
The scope owns atomic cutover and compatibility, not domain repair. This slice
delivers only the safe, additive projection model; the live-renderer cutover is a
later slice and the SESSION scenarios SCN-106-001/002 stay entry-gated on the
coordinator's BUG-070-001 production unified session.
## Completion Statement
Not complete. Status is In Progress. Only XP106-05-U (the additive
navigation-projection foundation) is proven with current-session evidence; every
cutover, deep-link-live, and session DoD item remains unchecked.
## Code Diff Evidence
Additive, allowlist-only. Two NEW files under `internal/experience/`:
`navigation_projection.go` (the `NavigationProjection`, route-authorization
resolver, and `CompatibilityMap` — pure, deterministic, fail-closed; composes
with the committed `ExperienceProjection` + generated catalog, edits none of
them) and `navigation_projection_test.go` (XP106-05-U). No live renderer
(`internal/web/**`, `web/pwa/**`) and no committed foundation file was modified.
## Test Evidence

### XP106-05-U

Additive foundation slice (XP106-05-U only): the generated NAVIGATION PROJECTION
data model (`internal/experience/navigation_projection.go`) the shared-shell
cutover will LATER wire into the server/PWA/Card renderers. The test derives
purely from the real generated catalog + the real registered router routes (no
mocks of internal code); it fails if the active hierarchy, audience gating, route
authorization, or compatibility map is wrong, and adversarially proves the
compatibility map invents no `/today` / `/work` / `/sources` / first-child
fallback and points at no missing route (every target is cross-checked against
the real router route literals + PWA tree via the in-package `resolves` helper).

**Command (run from `<repo-root>/`):**

```text
./smackerel.sh test unit --go --go-run 'TestGeneratedNavigationActiveHierarchyAudienceRoutesAndCompatibilityMap' --verbose
```

**Raw output — the focused package result (sibling packages report "no tests to run" for this regex):**

```text
=== RUN   TestGeneratedNavigationActiveHierarchyAudienceRoutesAndCompatibilityMap
=== RUN   TestGeneratedNavigationActiveHierarchyAudienceRoutesAndCompatibilityMap/ActiveHierarchy
=== RUN   TestGeneratedNavigationActiveHierarchyAudienceRoutesAndCompatibilityMap/AudienceAndRoutes
=== RUN   TestGeneratedNavigationActiveHierarchyAudienceRoutesAndCompatibilityMap/CompatibilityMap
=== RUN   TestGeneratedNavigationActiveHierarchyAudienceRoutesAndCompatibilityMap/Deterministic
=== RUN   TestGeneratedNavigationActiveHierarchyAudienceRoutesAndCompatibilityMap/FailClosed
--- PASS: TestGeneratedNavigationActiveHierarchyAudienceRoutesAndCompatibilityMap (0.00s)
    --- PASS: TestGeneratedNavigationActiveHierarchyAudienceRoutesAndCompatibilityMap/ActiveHierarchy (0.00s)
    --- PASS: TestGeneratedNavigationActiveHierarchyAudienceRoutesAndCompatibilityMap/AudienceAndRoutes (0.00s)
    --- PASS: TestGeneratedNavigationActiveHierarchyAudienceRoutesAndCompatibilityMap/CompatibilityMap (0.00s)
    --- PASS: TestGeneratedNavigationActiveHierarchyAudienceRoutesAndCompatibilityMap/Deterministic (0.00s)
    --- PASS: TestGeneratedNavigationActiveHierarchyAudienceRoutesAndCompatibilityMap/FailClosed (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/experience      0.016s
```

Supporting checks (same session): `./smackerel.sh check` passed (config in sync,
scenario-lint OK) and `gofmt -l internal/experience/navigation_projection.go
internal/experience/navigation_projection_test.go` printed nothing (both files
gofmt-clean).

**Scope boundary:** XP106-05-U ONLY. The live-renderer cutover (Implementation
Plan #1/#2/#5/#7), the deep-link-live rows (XP106-05-I/A/C, UX-E2E-106-005..008),
and the SESSION scenarios SCN-106-001/002 (XP106-05-I/A auth, UX-E2E-106-001..004)
remain entry-gated on the coordinator's BUG-070-001 production unified session and
stay unchecked / coupled-forward.

### XP106-05-C

XP106-05-C (SCN-106-015) — the SHELL-CUTOVER consumer-inventory STALENESS GUARD.
It is a STATIC/functional check (no live stack, no renderer change): it reads the
real generated catalog, the real registered router route literals, and the real
PWA file tree directly. It scans the UNION of (a) every existing first-party
consumer reference — server nav, PWA nav, web-manifest start/share/shortcut URLs,
service-worker precache list, catalog hrefs + preserved current paths — AND (b)
every destination the cutover itself will emit — the generated navigation
projection's `SupportedPaths` (proven audience-invariant) and the
`CompatibilityMap` source + target set — and asserts every one resolves to a REAL
registered browser route or real PWA static asset, with NO stale, invented,
first-child-fallback, out-of-inventory, or missing-route target. It is
deterministic and fail-closed, with two adversarial guards: a fabricated
unregistered route is flagged stale (non-tautological), and no
`/today` / `/work` / `/sources` / `/admin` / `/knowledge/graph` appears in the
projection or the compat map. Coverage honesty: `docs/*.md` and test-file route
references — and likewise breadcrumb links, native `<form>` actions, HTMX
targets, and request-client URLs — are NOT independently enumerated here; each
resolves to one of the SAME catalog-bound registered routes / navigation
authorities proven stale-free above, so proving those authoritative surfaces
stale-free proves those references non-stale BY CONSTRUCTION (there is no clean
committed route list for prose/heuristic references without false positives).
Stable `data-*` hooks are content-free DOM markers, not routes, and are out of
scope for a route-staleness scan. No coverage is fabricated.

**Outcome (a): PASS — the current consumer inventory + the cutover navigation
projection + the compatibility map carry ZERO stale first-party references.**

**Declared command (`./smackerel.sh check`, run from `<repo-root>/`) — proves the
config/SST surface is untouched by this test-only change:**

```text
config-validate: <repo-root>/config/generated/dev.env.tmp.3105864 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
```

**Functional assertion — the go-test lane that actually executes the staleness
guard, run from `<repo-root>/`:**

```text
./smackerel.sh test unit --go --go-run 'TestShellCutoverLeavesNoStaleNavigationRedirectManifestServiceWorkerDocOrTestTarget' --verbose
```

```text
=== RUN   TestShellCutoverLeavesNoStaleNavigationRedirectManifestServiceWorkerDocOrTestTarget
    consumer_inventory_test.go:154: cutover consumer inventory: 53 references across 8 surfaces, 0 stale
    consumer_inventory_test.go:187: cutover navigation projection: 13 supported paths, audience-invariant, all resolve and are catalog-inventoried
    consumer_inventory_test.go:236: cutover compatibility map: 13 entries, every source+target resolves and every target is projection-bound
    consumer_inventory_test.go:288: XP106-05-C: shell cutover leaves zero stale navigation/redirect/manifest/service-worker/doc/test targets across 53 consumer refs, 13 supported paths, 13 compat entries
--- PASS: TestShellCutoverLeavesNoStaleNavigationRedirectManifestServiceWorkerDocOrTestTarget (0.03s)
PASS
ok      github.com/smackerel/smackerel/internal/experience      0.042s
[go-unit] go test ./... finished OK
```

Same session: `./smackerel.sh format --check` reported `75 files already
formatted` — the edited `internal/experience/consumer_inventory_test.go` is
gofmt-clean and appears in no reformat list.

**Scope boundary:** XP106-05-C is the STATIC/functional consumer-inventory
staleness guard ONLY (composed from the committed SCOPE-02 consumer inventory +
the SCOPE-05 navigation projection; it edits no live renderer and no committed
foundation `.go` file). It is DISTINCT from the LIVE deep-link rows XP106-05-I
(integration) and XP106-05-A (e2e-api), which — together with the SESSION
scenarios SCN-106-001/002 and the live-renderer cutover — remain entry-gated on
the coordinator's BUG-070-001 production unified session and stay unchecked /
coupled-forward.

### server-nav-cutover

**Outcome: PARITY PRE-CHECK FAILED → SERVER APP-SHELL NAV CUTOVER NOT PERFORMED.**
This is a READ-ONLY pre-check result; no live renderer, test, DoD item, or
`state.json` status was changed by this slice.

This slice attempted the SCOPE-106-05 server app-shell nav cutover (Impl Plan #1):
switch the hardcoded server `{{define "app-shell-nav"}}` partial
(`internal/web/appshell.go`) to render its links by iterating the committed
`NavigationProjection`'s ordered visible nav, byte-identical to the current nav.
Per the cutover discipline — parity FIRST, never force a cutover if the projection
does not byte-match the current nav — a READ-ONLY parity pre-check was run BEFORE
touching any renderer. It does NOT hold: the projection's ordered visible nav for
a representative authenticated `daily_user` differs from the current hardcoded
partial across set, order, label, AND stable `data-nav` key. The cutover was
therefore NOT performed and no renderer was touched.

**Read-only pre-check (run from `<repo-root>/`):**

```text
$ sed -n '31p' internal/web/appshell.go | grep -oE 'href="[^"]*" data-nav="[^"]*">[^<]*'
href="/assistant" data-nav="assistant">Assistant
href="/" data-nav="search">Search
href="/knowledge" data-nav="knowledge">Knowledge
href="/cards" data-nav="cards">Cards
href="/notifications" data-nav="notifications">Notifications
href="/settings" data-nav="settings">Settings

$ jq -r '[.surfaces[] | select(.parent_id=="") | select(.audiences|index("daily_user"))] | sort_by(.order)[] | "order=\(.order) id=\(.id) label=\(.label) href=\(if .href=="" then "(none/group)" else .href end) policy=\(.readiness_discoverability_policy)"' internal/experience/catalog.gen.json
order=10 id=assistant label=Assistant href=/assistant policy=ready_when_journey_ready
order=20 id=capture label=Capture href=/pwa/ policy=ready_when_journey_ready
order=30 id=search label=Search href=/ policy=ready_when_journey_ready
order=40 id=today label=Today href=/digest policy=ready_when_journey_ready
order=50 id=knowledge label=Knowledge href=/knowledge policy=ready_when_journey_ready
order=60 id=work label=Work href=(none/group) policy=route_free_group
order=70 id=cards label=Cards href=/cards policy=ready_when_journey_ready
order=80 id=recommendations label=Recommendations href=/recommendations policy=ready_when_journey_ready
order=90 id=sources label=Sources href=(none/group) policy=route_free_group
order=100 id=activity label=Activity href=/notifications policy=ready_when_journey_ready
order=110 id=settings label=Settings href=/settings policy=ready_when_journey_ready
```

**Exact mismatch (decisive across every dimension the parity contract names):**

- **SET:** the projection's `daily_user` visible top level adds `capture`
  (`/pwa/`), `today` (`/digest`), `recommendations` (`/recommendations`), and the
  two route-free groups `work` + `sources` that the current 6-link server partial
  does NOT contain.
- **ORDER:** the shared targets are interleaved with those extras (`capture`
  between assistant↔search; `today` between search↔knowledge; `recommendations`
  between cards↔activity), so even the shared subset is not in the partial's
  order.
- **LABEL + stable KEY:** `/notifications` is catalog surface `activity` labeled
  **"Activity"**, whereas the current partial renders it with
  `data-nav="notifications"` labeled **"Notifications"** — the wayfinding label
  AND the stable hook key differ.
- **RENDERER AUTHORING SIGNAL:** the catalog marks `assistant`
  `renderer_support=["pwa"]` and `cards` `renderer_support=["card"]`, yet the
  current server partial renders both server-side; `renderer_support` is not even
  carried on `ProjectedSurface`, so the projection cannot reproduce a server-only
  subset. This confirms the generated catalog was authored as the FUTURE unified
  IA, not as a byte-copy of the current transitional server nav.

**Disposition:** catalog/projection ↔ current-server-nav RECONCILIATION finding
owned by SCOPE-106-02 (canonical catalog + route inventory), NOT something
SCOPE-05 may paper over by force-fitting the renderer or weakening a test. The
server cutover stays BLOCKED until the generated catalog and the current server
partial are reconciled to one agreed nav (either the catalog is adjusted to
preserve the current transitional server nav, or the server nav is intentionally
migrated to the new IA under its own guarded slice with updated canaries). No DoD
row is checked by this slice; no forbidden file (070 login shell `internal/api/**`,
`web/pwa/**` app code, the Card renderer chrome, `internal/experience/*.go`
non-test) was touched.
## Planned Test References
**Claim Source:** not-run
Planned execution uses `./smackerel.sh`; the concrete not-yet-authored files and titles are listed in `scope.md` and root `test-plan.json` and are not execution evidence.
## Uncertainty Declarations
BUG-070 completion is required before pickup.
## Scenario Contract Evidence
See `scenario-manifest.json` and `test-plan.json` at the spec root.
## Coverage Report
No runtime coverage is claimed.
## Lint/Quality
No scope execution quality result is claimed.
## Validation Summary
No validation or certification result is claimed.
## Audit Verdict
No audit verdict is claimed.
