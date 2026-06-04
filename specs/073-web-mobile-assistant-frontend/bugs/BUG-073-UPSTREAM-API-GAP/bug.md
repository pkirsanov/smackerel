# BUG-073-UPSTREAM-API-GAP: Spec 073 Scope 5 blocked by missing backend JSON APIs

**Status:** Resolved — Upstream resolution shipped: spec 080 (commit 98c16290) + spec 027 Scope 9 (commit e6ccdb2a). Both upstream specs certified done.
**Severity:** Blocker (Scope 5 cannot proceed in-repo)
**Reported:** 2026-06-03
**Reporter:** bubbles.plan (via owner directive)
**Owner:** Needs operator triage to assign to specific upstream spec(s)
**Affected feature:** `specs/073-web-mobile-assistant-frontend/` — SCOPE-073-05 (Knowledge Graph Browse Surface)
**Affected scenarios:** SCN-073-B01, SCN-073-B02, SCN-073-B03, SCN-073-B04, SCN-073-B05, SCN-073-B06

## Summary

Spec 073 Scope 5 (Knowledge Graph Browse Surface) cannot proceed because the
backend JSON APIs the wiki PWA must consume do not exist. Per Scope 5's own
Uncertainty Declaration and Implementation Plan, the prescribed exit path is to
"stop and route a finding to the owning spec instead of hand-rolling client
types". This bug packet formalizes that route.

## Verification

`grep -nE "r\.(Get|Post|Mount)" internal/api/router.go` against the keywords
`topic|people|person|place|time|graph|edge|wiki|annotation|artifact` confirmed
none of the eight wiki-consumed JSON endpoints exist today. The only adjacent
surface is `/topics` (server-rendered HTML via `deps.WebHandler.TopicsPage`)
which is the wrong shape (HTML, no graph edges, no people/place counts).

## Missing Endpoints

| # | Endpoint | Consuming Scenarios | Candidate Owning Module |
|---|---|---|---|
| 1 | `GET /api/topics` — index with `{linkedArtifactCount, peopleCount, placeCount}` | SCN-073-B01 | NEW spec extending `internal/topics` |
| 2 | `GET /api/topics/{id}` — topic detail with linked artifacts, related people, related places | SCN-073-B01 | Same as #1 |
| 3 | `GET /api/people` — index of intelligence-layer-derived people with `artifactCount` | SCN-073-B02 | NEW spec under `internal/intelligence` |
| 4 | `GET /api/people/{id}` — person page with artifact timeline, related topics, related places | SCN-073-B02 | Same as #3 |
| 5 | `GET /api/places` — index from maps connector + artifact-derived locations | SCN-073-B03 | NEW spec spanning `internal/knowledge` + maps connector (spec 011) |
| 6 | `GET /api/places/{id}` — place page with location + linked artifacts | SCN-073-B03 | Same as #5 |
| 7 | `GET /api/time?from=&to=` — artifacts grouped by day for calendar-style scroll | SCN-073-B04 | NEW spec under `internal/knowledge` |
| 8 | `GET /api/graph/edges?source={kind:id}` — universal cross-link `{targetKind, targetId, targetLabel, reason}` | SCN-073-B05 (also feeds B01..B04 "Related" sections) | NEW spec under `internal/graph` |

## What Exists Today (Seams)

- `/api/artifact/{id}` (`deps.ArtifactDetailHandler`) — single artifact, not graph-derived.
- `/api/artifacts/{id}/domain` (`deps.DomainDataHandler`).
- `/api/knowledge/{concepts,concepts/{id},entities,entities/{id},lint,stats}`.
- `/api/intelligence/{expertise,learning-paths,subscriptions,serendipity,content-fuel,quick-references,monthly-report,seasonal-patterns}`.
- Server-rendered HTML at `/topics` (`deps.WebHandler.TopicsPage`) — wrong shape.

## Exit Condition

Spec 073 Scope 5 ships when the eight endpoints above exist and are reachable
from the live PWA. Until then, Scope 5 remains `Not started` and spec 073 stays
at `specs_hardened`. SCN-073-B06 (inline annotation entry point) is unaffected
by this bug — it already gracefully degrades when spec 027 SCOPE-9
(`SCN-027-71..74`) is unavailable.

## Routing Decision

Operator triage required. The eight endpoints span at least four candidate
owning modules (`internal/topics`, `internal/intelligence`,
`internal/knowledge`, `internal/graph`) and one connector (spec 011 maps).
Operator must decide whether to file one omnibus spec, one spec per module,
or fold the contracts into existing related specs.

## Cross-References

- Parent spec: [`../../spec.md`](../../spec.md), [`../../scopes.md`](../../scopes.md) → `## Scope 5 — Upstream Blocker (Route Required)`
- Backend router under inspection: `internal/api/router.go`
- Server-rendered topics page (wrong shape): `deps.WebHandler.TopicsPage`
