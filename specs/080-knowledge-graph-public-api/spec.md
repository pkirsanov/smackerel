# Feature: 080 Knowledge Graph Public API

**Status:** in_progress (analyst bootstrap; ceiling = `done`)
**Workflow Mode:** `full-delivery`
**Release Train:** `next` (default-off on `mvp`)
**Owner Directive (2026-06-03):** Ship the 8 missing JSON endpoints
that the wiki/graph-browse PWA frontend in
[spec 073 Scope 5](../073-web-mobile-assistant-frontend/scopes.md#scope-5-knowledge-graph-browse-surface-graph-browse-surface)
requires. Without these endpoints the frontend cannot render the
graph; client-side re-derivation of cross-link reasons is forbidden
(spec 073 Containment Rule).

**Depends On:**
[spec 044 — Per-User Bearer Auth](../044-per-user-bearer-auth/spec.md),
[spec 060 — Bearer Auth Scope Claim](../060-bearer-auth-scope-claim/spec.md).
**Consumed By:**
[spec 073 Scope 5 — Knowledge Graph Browse Surface](../073-web-mobile-assistant-frontend/scopes.md#scope-5-knowledge-graph-browse-surface-graph-browse-surface)
(unblocks
[`bugs/BUG-073-UPSTREAM-API-GAP`](../073-web-mobile-assistant-frontend/bugs/BUG-073-UPSTREAM-API-GAP/)).
**Reuses:**
[`internal/topics/`](../../internal/topics/),
[`internal/intelligence/`](../../internal/intelligence/) (people),
[`internal/knowledge/`](../../internal/knowledge/) (artifacts, places),
[`internal/graph/`](../../internal/graph/) (edges).

---

## 1. Problem Statement

Spec 073 Scope 5 (Knowledge Graph Browse Surface) is `blocked` because
the PWA wiki UI needs JSON read-paths over the knowledge graph that do
not exist:

1. `GET /api/topics`
2. `GET /api/topics/{id}`
3. `GET /api/people`
4. `GET /api/people/{id}`
5. `GET /api/places`
6. `GET /api/places/{id}`
7. `GET /api/time?from=<iso>&to=<iso>`
8. `GET /api/graph/edges?source={kind:id}`

The underlying primitives exist in `internal/topics`, `internal/intelligence`,
`internal/knowledge`, and `internal/graph`, but there are no public
routes that compose them into shapes the frontend can render. The PWA
must NOT re-derive cross-link reasons client-side (spec 073
Containment Rule); reasons MUST be server-derived.

This spec ships all 8 endpoints as one coherent contract because they
share the explainable cross-link shape
`{targetKind, targetId, targetLabel, reason}`. Splitting them across
multiple specs would let the cross-link shape drift between resources.

---

## 2. Outcome Contract

**Intent:** The 8 endpoints above expose the existing knowledge graph
as a stable, authenticated, paginated JSON API. Every cross-link in
any response carries a server-derived, human-readable `reason` that
the client renders verbatim. The PWA in spec 073 Scope 5 ships
against this API without any scenario branching or graph re-derivation
in the client.

**Success Signal:**
- All 8 endpoints return 200 with schema-pinned JSON for an
  authenticated caller whose bearer token carries the
  `knowledge-graph:read` scope.
- Every cross-link in every response has a non-empty `targetKind`,
  `targetId`, `targetLabel`, and `reason` field.
- Spec 073 Scope 5 PWA renders topics, people, places, time, and an
  artifact's "Related" section by calling only these 8 endpoints with
  no additional helper routes and no client-side joins across them.
- Live integration tests against the running stack hit each endpoint
  and assert the cross-link contract.

**Hard Constraints:**
- Cross-link `reason` MUST be derived server-side from
  `internal/graph` edge metadata. No client-side re-derivation, no
  ranking, no scenario branching.
- Every endpoint requires bearer auth (spec 044) AND the
  `knowledge-graph:read` scope claim (spec 060). Missing or wrong
  scope returns 403.
- Every list endpoint returns `{items: [...], nextCursor: "<opaque-or-empty>"}`.
  Cursors are opaque server-issued strings (not row offsets).
- All pagination defaults and upper bounds are read from
  `config/smackerel.yaml` under `knowledge_graph_api:` with fail-loud
  SST (no in-source defaults; missing config → startup failure).
- The `/api/time` endpoint requires BOTH `from` and `to`; 400 if
  either is missing or unparseable; 400 if `to - from > 365 days`.
- `/api/graph/edges` requires `source={kind}:{id}` where `kind ∈ {artifact, topic, person, place}`;
  400 for any other kind.

**Failure Condition:**
- Frontend would have to perform a second call to render the
  cross-link reason → contract has failed.
- A scope-less bearer token can read knowledge-graph content → fails
  spec 060 enforcement.
- Two endpoints disagree on the cross-link shape → fails the
  single-contract rationale.

---

## 3. Product Principle Alignment

Smackerel principle references from
[docs/Product-Principles.md](../../docs/Product-Principles.md):

| Principle | How this spec aligns |
|-----------|----------------------|
| **P2 Vague In, Precise Out** | The PWA gets explainable, fully-resolved cross-link rows from a single call. The user does not type field names; the server returns the precise shape. |
| **P5 One Graph, Many Views** | These endpoints are READ projections of the SAME underlying knowledge graph (`internal/graph`, `internal/topics`, `internal/intelligence`, `internal/knowledge`). No parallel store. The contract preserves graph identity (`targetKind`, `targetId`) so the client navigates one graph through many views. |
| **P8 Trust Through Transparency** | Every cross-link carries `reason` — the audit-grade why-this-related string. Reason is the trust artifact: the client cannot lie about why two nodes connect because the server owns the reason text. |

No principle deviations.

---

## 4. Domain Capability Model (AN5)

This spec ships ONE capability — "Knowledge Graph Read API" — with
ONE provider (the Go core runtime over Postgres). AN5 capability-first
proportionality triggers do NOT apply (no second provider, no
adapter/strategy/plugin language, no shared variant). Below is the
single-capability behavior vocabulary the design must obey.

**Domain primitives:**
- `Topic` (id, label, linkedArtifactCount, peopleCount, placeCount)
- `Person` (id, displayName, artifactCount)
- `Place` (id, displayName, location, artifactCount)
- `Artifact` reference (id, kind, title, capturedAt)
- `CrossLink` (targetKind, targetId, targetLabel, reason)

**Lifecycle states:** read-only projections; no mutation.

**Relationships:** every detail endpoint composes its primitive with a
collection of `CrossLink` rows derived from `internal/graph` edges.

**Server-derived reason taxonomy** (initial; design may extend with a
finite enum):
- `"shares topic <label>"`
- `"mentioned in <artifact title>"`
- `"same place <label>"`
- `"co-occurs with <person displayName>"`
- `"captured on <date>"`

Client renders the `reason` string verbatim. No client lookup tables.

---

## 5. Actors & Personas

| Actor | Description | Goals | Permissions |
|-------|-------------|-------|-------------|
| **PWA wiki user (via spec 073)** | Authenticated user browsing the wiki surface in the web client. | Browse topics/people/places/time; open a node; see explainable cross-links. | Bearer token (spec 044) with `knowledge-graph:read` scope (spec 060). |
| **Future native mobile** | iOS/Android client built on spec 073's shared mobile foundation. | Identical wiki behavior to PWA. | Same bearer + scope. |
| **Third-party API consumer** | External tool reading the graph through bearer-issued tokens. | Programmatic read of the user's knowledge graph. | Same bearer + scope; scope can be issued narrowly. |
| **Backend reason resolver** (server-internal) | `internal/graph` helper that turns edge metadata into the `reason` string. | Produce stable, human-readable, audit-grade reason text. | N/A — internal. |

---

## 6. Use Cases

### UC-080-001: List topics
- **Actor:** PWA wiki user
- **Preconditions:** Authenticated bearer with `knowledge-graph:read`.
- **Main flow:** Client calls `GET /api/topics` with optional `cursor` and `limit`. Server returns paginated `{items: [{id, label, linkedArtifactCount, peopleCount, placeCount}], nextCursor}`.
- **Postconditions:** Client renders the topics index of SCN-073-B01.

### UC-080-002: Topic detail with graph edges
- **Actor:** PWA wiki user
- **Preconditions:** Topic id from UC-080-001.
- **Main flow:** Client calls `GET /api/topics/{id}`. Server returns the topic plus `linkedArtifacts: [...]`, `relatedPeople: [...]`, `relatedPlaces: [...]`, each row containing the `{targetKind, targetId, targetLabel, reason}` cross-link shape.
- **Postconditions:** Client renders the topic page with explainable cross-links.

### UC-080-003: List people
- **Actor:** PWA wiki user
- **Main flow:** `GET /api/people` returns `{items: [{id, displayName, artifactCount}], nextCursor}`.

### UC-080-004: Person detail
- **Actor:** PWA wiki user
- **Main flow:** `GET /api/people/{id}` returns the person with `artifactTimeline: [...]`, `relatedTopics: [...]`, `relatedPlaces: [...]` (cross-link shape).

### UC-080-005: List places
- **Actor:** PWA wiki user
- **Main flow:** `GET /api/places` returns `{items: [{id, displayName, location, artifactCount}], nextCursor}`. Includes maps-connector places AND artifact-derived locations.

### UC-080-006: Place detail
- **Actor:** PWA wiki user
- **Main flow:** `GET /api/places/{id}` returns the place with `location` and `linkedArtifacts: [...]` (cross-link shape).

### UC-080-007: Time-window artifacts
- **Actor:** PWA wiki user
- **Main flow:** `GET /api/time?from=<iso>&to=<iso>` returns `{days: [{date: "YYYY-MM-DD", artifacts: [...]}], nextCursor}`. 400 if `from`/`to` missing/unparseable or window > 365 days.

### UC-080-008: Cross-link edges for a source node
- **Actor:** PWA wiki user / any rendering surface
- **Main flow:** `GET /api/graph/edges?source={kind}:{id}` returns `{items: [{targetKind, targetId, targetLabel, reason}], nextCursor}`. 400 on unknown `kind`. Server resolves edges from `internal/graph` and attaches the `reason` string.

---

## 7. Business Scenarios (Gherkin)

```gherkin
Scenario: SCN-080-01 — List topics returns counts and pagination cursor
  Given an authenticated caller with the "knowledge-graph:read" scope
  And the knowledge graph contains at least 3 topics with linked artifacts
  When the caller GETs /api/topics
  Then the response is 200
  And the body has an "items" array where each item has id, label,
    linkedArtifactCount, peopleCount, placeCount
  And the body has a "nextCursor" field (string; empty when no more pages)

Scenario: SCN-080-02 — Topic detail returns explainable cross-links
  Given an authenticated caller with the "knowledge-graph:read" scope
  And a topic with id "T123" exists with linked artifacts, people, and places
  When the caller GETs /api/topics/T123
  Then the response is 200
  And the body contains linkedArtifacts, relatedPeople, relatedPlaces
  And every cross-link row has non-empty targetKind, targetId,
    targetLabel, and a server-derived reason string

Scenario: SCN-080-03 — List people derived from intelligence layer
  Given an authenticated caller with the "knowledge-graph:read" scope
  And the intelligence layer has derived at least 2 people
  When the caller GETs /api/people
  Then the response is 200
  And each item has id, displayName, and artifactCount

Scenario: SCN-080-04 — Person detail returns timeline and related rows
  Given an authenticated caller with the "knowledge-graph:read" scope
  And a person with id "P5" appears in multiple artifacts
  When the caller GETs /api/people/P5
  Then the response is 200
  And artifactTimeline rows are ordered by capturedAt descending
  And relatedTopics and relatedPlaces use the cross-link shape with reason

Scenario: SCN-080-05 — List places merges maps-connector and artifact-derived
  Given an authenticated caller with the "knowledge-graph:read" scope
  And at least one place originates in the maps connector
  And at least one place was derived from artifact metadata
  When the caller GETs /api/places
  Then the response is 200
  And items include both place sources without duplicate ids

Scenario: SCN-080-06 — Place detail returns location and linked artifacts
  Given an authenticated caller with the "knowledge-graph:read" scope
  And a place with id "PL9" has linked artifacts
  When the caller GETs /api/places/PL9
  Then the response is 200
  And the body has a location object and a linkedArtifacts array
    using the cross-link shape with reason

Scenario: SCN-080-07 — Time window groups artifacts by day
  Given an authenticated caller with the "knowledge-graph:read" scope
  When the caller GETs /api/time?from=2026-05-01T00:00:00Z&to=2026-05-08T00:00:00Z
  Then the response is 200
  And the body has a "days" array of {date, artifacts[]} entries
  And every artifacts row is within the requested window

Scenario: SCN-080-08 — Graph edges return explainable cross-links
  Given an authenticated caller with the "knowledge-graph:read" scope
  And artifact "A42" has graph edges to topics, people, and places
  When the caller GETs /api/graph/edges?source=artifact:A42
  Then the response is 200
  And every item carries targetKind, targetId, targetLabel, and a
    non-empty reason derived from internal/graph edge metadata

Scenario: SCN-080-09 — Missing bearer token returns 401
  Given an unauthenticated caller
  When the caller GETs /api/topics
  Then the response is 401
  And the body contains no graph data

Scenario: SCN-080-10 — Bearer without knowledge-graph:read scope returns 403
  Given an authenticated caller whose token has only the "assistant.turn" scope
  When the caller GETs /api/people
  Then the response is 403
  And the body contains no graph data

Scenario: SCN-080-11 — Malformed cursor returns 400
  Given an authenticated caller with the "knowledge-graph:read" scope
  When the caller GETs /api/topics?cursor=not-a-real-cursor
  Then the response is 400
  And the body identifies the cursor field as invalid

Scenario: SCN-080-12 — Time window over 365 days returns 400
  Given an authenticated caller with the "knowledge-graph:read" scope
  When the caller GETs /api/time?from=2024-01-01T00:00:00Z&to=2026-01-02T00:00:00Z
  Then the response is 400
  And the body identifies the window as exceeding the 365-day limit

Scenario: SCN-080-13 — Time window with missing "to" returns 400
  Given an authenticated caller with the "knowledge-graph:read" scope
  When the caller GETs /api/time?from=2026-05-01T00:00:00Z
  Then the response is 400
  And the body identifies "to" as required

Scenario: SCN-080-14 — Unknown source kind on /api/graph/edges returns 400
  Given an authenticated caller with the "knowledge-graph:read" scope
  When the caller GETs /api/graph/edges?source=unicorn:X1
  Then the response is 400
  And the body lists the allowed kinds (artifact, topic, person, place)

Scenario: SCN-080-15 — Limit above configured max is clamped or rejected
  Given an authenticated caller with the "knowledge-graph:read" scope
  And config/smackerel.yaml sets knowledge_graph_api.list_max_limit = 200
  When the caller GETs /api/topics?limit=10000
  Then the response is 400
  And the body identifies the limit as exceeding the configured maximum
```

---

## 8. UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) consuming |
|----------|-------|-------------|-------|-------------------|---------------------|
| SCN-080-01 | PWA wiki user | Wiki → Topics tab | open tab | topic list with counts | spec 073 SCN-073-B01 |
| SCN-080-02 | PWA wiki user | Topics tab → select topic | tap topic | topic page with related people / places / artifacts | spec 073 SCN-073-B01 |
| SCN-080-03 | PWA wiki user | Wiki → People tab | open tab | people list with artifact counts | spec 073 SCN-073-B02 |
| SCN-080-04 | PWA wiki user | People tab → select person | tap person | person page with timeline + related | spec 073 SCN-073-B02 |
| SCN-080-05 | PWA wiki user | Wiki → Places tab | open tab | places list (maps + derived) | spec 073 SCN-073-B03 |
| SCN-080-06 | PWA wiki user | Places tab → select place | tap place | place page with location + artifacts | spec 073 SCN-073-B03 |
| SCN-080-07 | PWA wiki user | Wiki → Time tab | open tab | calendar-style day-grouped scroll | spec 073 SCN-073-B04 |
| SCN-080-08 | PWA wiki user | Any artifact / topic / person / place page | open node | "Related" section with explainable cross-links | spec 073 SCN-073-B05 |

---

## 9. Non-Functional Requirements

- **Performance:** p95 < 200ms for list endpoints; p95 < 100ms for
  detail endpoints with a warmed cache. Measured on the live stack
  with a representative graph.
- **Auth/security:** every endpoint behind spec 044 bearer auth and
  spec 060 scope middleware. New scope surface `knowledge-graph:read`
  added to `RegisteredScopeSurfaces` in
  [`internal/auth/scopes.go`](../../internal/auth/scopes.go).
- **SST (smackerel-no-defaults):** all pagination defaults, list
  upper bounds, and the time-range upper bound live in
  `config/smackerel.yaml` under `knowledge_graph_api:`. Startup MUST
  fail loud if any field is missing or non-positive.
- **Observability:** standard request log + per-endpoint metrics
  (request count, latency histogram, 4xx/5xx counters).
- **Backward compatibility:** the cross-link contract
  `{targetKind, targetId, targetLabel, reason}` is the single shape
  for all 8 endpoints; any future graph view MUST reuse it.

---

## 10. Release Train

**Train:** `next` (post-MVP).

Rationale: the sole consumer (spec 073 Scope 5) is `next` work. This
spec is default-OFF on the `mvp` train and default-ON on `next`. No
per-feature flag is required — gating happens via the
`knowledge-graph:read` scope claim. Issuing a token without that scope
on `mvp` means the endpoints are effectively unreachable to operators.

`flagsIntroduced: []`. If a gradual rollout is later desired, a flag
named `knowledge_graph_api_enabled` would be added through the
standard release-train flag-bundle process.

---

## 11. Open Questions

- Should `/api/time` accept a `groupBy=week|month` parameter (deferred
  — out of MVP scope; current contract is day-grouped only).
- Should `reason` strings be localized? Current contract is
  English-only and server-rendered; localization is a future spec.
- Should `/api/graph/edges` support multi-source batch lookups
  (`source=artifact:A,artifact:B`)? Deferred unless 073 Scope 5 demands
  it.

---

## 12. Out of Scope

- Mutations (write paths). All 8 endpoints are READ-only.
- Search / full-text query. That belongs to a future query spec.
- Native mobile client work — owned by spec 073 follow-on mobile delivery.
- Capture flows (owned by specs 033 / 058).
- Inline annotation entry point (owned by spec 027 Scope 9 + spec 073 SCN-073-B06).

---

## 13. References

- [spec 073 Scope 5 — Knowledge Graph Browse Surface](../073-web-mobile-assistant-frontend/scopes.md#scope-5-knowledge-graph-browse-surface-graph-browse-surface)
- [BUG-073-UPSTREAM-API-GAP](../073-web-mobile-assistant-frontend/bugs/BUG-073-UPSTREAM-API-GAP/)
- [spec 044 — Per-User Bearer Auth](../044-per-user-bearer-auth/spec.md)
- [spec 060 — Bearer Auth Scope Claim](../060-bearer-auth-scope-claim/spec.md)
- [`internal/auth/scopes.go`](../../internal/auth/scopes.go) (RegisteredScopeSurfaces)
- [`internal/graph/`](../../internal/graph/), [`internal/topics/`](../../internal/topics/), [`internal/intelligence/`](../../internal/intelligence/), [`internal/knowledge/`](../../internal/knowledge/)
- [docs/Product-Principles.md](../../docs/Product-Principles.md)
