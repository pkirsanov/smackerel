# Report: BUG-073-UPSTREAM-API-GAP

## Summary

Bug filed 2026-06-03 by `bubbles.plan` to formally route the spec 073 Scope 5
upstream backend API gap. No implementation, test, or validation phases have
run — this packet exists to surface the blocker for operator triage.

## Discovery Evidence

Verified by grep of `internal/api/router.go` against keywords
`topic|people|person|place|time|graph|edge|wiki|annotation|artifact`. None of
the eight JSON endpoints required by SCN-073-B01..B05 exist today. Only
adjacent surface is `/topics` (server-rendered HTML via
`deps.WebHandler.TopicsPage`) which is the wrong shape.

## Routing

Status: `open`. Severity: `blocker`. Owner: needs operator triage to assign the
eight endpoints to specific upstream spec(s) across:

- `internal/topics` (endpoints 1, 2)
- `internal/intelligence` (endpoints 3, 4)
- `internal/knowledge` + spec 011 maps connector (endpoints 5, 6)
- `internal/knowledge` (endpoint 7)
- `internal/graph` (endpoint 8 — universal cross-link contract)

See `bug.md` for the full endpoint table and `spec.md` for acceptance criteria.

## Next Required Owner

null — operator triage required. No autonomous follow-up.

## Resolution — 2026-06-04

Bug resolved. Upstream blockers cleared:

- **spec 080 (Knowledge Graph Public API)** shipped at commit `98c16290`,
  status `done`. All 8 required JSON endpoints are live and scope-gated by
  the `knowledge-graph:read` claim:
  - `GET /api/topics`, `GET /api/topics/{id}`
  - `GET /api/people`, `GET /api/people/{id}`
  - `GET /api/places`, `GET /api/places/{id}`
  - `GET /api/time?from=&to=`
  - `GET /api/graph/edges?source={kind:id}`
- **spec 027 Scope 9 (Annotation Editing API)** shipped at commit
  `e6ccdb2a`, status `done`. The SCN-073-B06 inline annotation entry point
  wires to real endpoints (`SCN-027-71..74`).

Spec 073 Scope 5 unblocked: status flipped Not started → In progress;
DoD "BLOCKED on upstream API gap" suffixes removed; planning-side ceiling
on the parent spec lifted specs_hardened → done. Scope 5 is now ready for
`bubbles.implement` dispatch under the existing Implementation Plan and
Test Plan (TP-073-25..31) in
[`../../scopes.md`](../../scopes.md#scope-5-knowledge-graph-browse-surface-graph-browse-surface).
