# Spec: BUG-073-UPSTREAM-API-GAP — Backend JSON APIs for wiki/graph-browse surface

## Expected Behavior

The backend MUST expose eight JSON API endpoints that the spec 073 Scope 5 wiki
PWA surface consumes. Each endpoint MUST return JSON (not server-rendered HTML)
and MUST be reachable from the live stack via the same-origin HttpOnly session
cookie path established by spec 070.

## Acceptance Criteria

1. **AC-1 (Topics index):** `GET /api/topics` returns a JSON index of topics
   with `{id, label, linkedArtifactCount, peopleCount, placeCount}` per entry.
   Consumed by SCN-073-B01.
2. **AC-2 (Topic detail):** `GET /api/topics/{id}` returns a JSON topic detail
   payload with linked artifacts, related people, and related places. Consumed
   by SCN-073-B01.
3. **AC-3 (People index):** `GET /api/people` returns a JSON index of
   intelligence-layer-derived people with `{id, label, artifactCount}` per
   entry. Consumed by SCN-073-B02.
4. **AC-4 (Person detail):** `GET /api/people/{id}` returns a JSON person page
   payload with artifact timeline, related topics, and related places. Consumed
   by SCN-073-B02.
5. **AC-5 (Places index):** `GET /api/places` returns a JSON index of places
   from the maps connector (spec 011) plus artifact-derived locations. Consumed
   by SCN-073-B03.
6. **AC-6 (Place detail):** `GET /api/places/{id}` returns a JSON place page
   payload with map-derived location and linked artifacts. Consumed by
   SCN-073-B03.
7. **AC-7 (Time view):** `GET /api/time?from=<iso>&to=<iso>` returns artifacts
   grouped by day for calendar-style scroll. Consumed by SCN-073-B04.
8. **AC-8 (Cross-link edges):** `GET /api/graph/edges?source={kind:id}` returns
   a universal cross-link contract with `{targetKind, targetId, targetLabel,
   reason}` per edge. `reason` strings MUST be server-supplied and rendered
   verbatim by the client (no client-side derivation). Consumed by SCN-073-B05
   and the "Related" sections of all SCN-073-B01..B04 detail pages.
9. **AC-9 (Auth parity):** All eight endpoints honor the same-origin HttpOnly
   session cookie path used by `/api/assistant/turn`. No JS-visible bearer
   fallback for web.
10. **AC-10 (Routing assignment):** Each endpoint is assigned to a specific
    upstream owning spec (operator triage). Once landed, spec 073 Scope 5 can
    be unblocked and dispatched to `bubbles.implement`.

## Out of Scope

- Implementing the wiki PWA client (owned by spec 073 Scope 5).
- The annotation entry point endpoints (owned by spec 027 SCOPE-9 — SCN-027-71..74).
- Changing the shape of existing `/api/knowledge/{concepts,entities,...}` or
  `/api/intelligence/...` endpoints.

## Cross-References

- Blocked spec: `specs/073-web-mobile-assistant-frontend/spec.md`
- Blocked scope: `specs/073-web-mobile-assistant-frontend/scopes.md` → Scope 5
- Blocked scenarios: SCN-073-B01, SCN-073-B02, SCN-073-B03, SCN-073-B04,
  SCN-073-B05, SCN-073-B06
- Bug summary: `bug.md`
- Backend router under inspection: `internal/api/router.go`
