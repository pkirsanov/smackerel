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
