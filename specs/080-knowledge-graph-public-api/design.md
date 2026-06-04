# Design: 080 Knowledge Graph Public API

**Status:** draft (analyst bootstrap; design refinement owned by bubbles.design)
**Implements:** [spec.md](spec.md)

---

## 1. Architecture Overview

A new HTTP surface lives in `internal/api/graphapi/` (a new package).
It exposes 8 read-only handlers that compose the existing primitives
in `internal/topics`, `internal/intelligence`, `internal/knowledge`,
and `internal/graph`. No new persistent storage. No new background
jobs. Pure read projection.

```
PWA (spec 073)
   │  HTTPS + Bearer (spec 044) + "knowledge-graph:read" scope (spec 060)
   ▼
internal/api/router  (chi/gin)
   │
   ├─ scope_middleware ("knowledge-graph:read")
   ▼
internal/api/graphapi
   ├─ topics_handler.go     (GET /api/topics, GET /api/topics/{id})
   ├─ people_handler.go     (GET /api/people, GET /api/people/{id})
   ├─ places_handler.go     (GET /api/places, GET /api/places/{id})
   ├─ time_handler.go       (GET /api/time)
   ├─ edges_handler.go      (GET /api/graph/edges)
   ├─ crosslink.go          (shared {targetKind,targetId,targetLabel,reason})
   ├─ cursor.go             (opaque cursor encode/decode)
   ├─ limits.go             (SST-bound limits from config)
   └─ errors.go             (uniform 4xx body shape)
        │
        ▼
internal/topics ── internal/intelligence ── internal/knowledge ── internal/graph
                                (Postgres)
```

Rationale for one package (vs one file per resource at the api root):
the 8 handlers share the cross-link shape, cursor codec, error
envelope, and reason-resolver. Co-locating them in `graphapi` keeps the
shared contract enforceable by Go's package boundary.

---

## 2. Cross-Link Contract

Every response that surfaces a relationship between two graph nodes
MUST use this exact shape:

```json
{
  "targetKind":  "topic | person | place | artifact",
  "targetId":    "<stable id>",
  "targetLabel": "<human-readable display label>",
  "reason":      "<server-derived natural-language explanation>"
}
```

- `targetKind` is a closed enum.
- `targetId` is stable across requests.
- `targetLabel` is the same display string the resource's own detail
  endpoint would return.
- `reason` is server-derived from `internal/graph` edge metadata.
  Client renders verbatim — no parsing, no re-ranking, no scenario
  branching.

### Reason taxonomy (initial)

A finite, server-side enum of templated strings:

| Edge metadata signal | Reason string template |
|----------------------|------------------------|
| Topic co-occurrence  | `"shares topic <topic label>"` |
| Artifact mention     | `"mentioned in <artifact title>"` |
| Place co-occurrence  | `"same place <place label>"` |
| Person co-occurrence | `"co-occurs with <person displayName>"` |
| Same-day capture     | `"captured on <YYYY-MM-DD>"` |

New reason categories require a server-side change. The client MUST
NOT add new categories. This is the trust artifact (P8).

---

## 3. Endpoint Schemas

All list endpoints return:

```json
{
  "items": [ ... ],
  "nextCursor": "<opaque-or-empty-string>"
}
```

### `GET /api/topics`
- Query: `cursor?`, `limit?` (clamped against `list_max_limit`).
- Item: `{id, label, linkedArtifactCount, peopleCount, placeCount}`.

### `GET /api/topics/{id}`
- Item: `{id, label, linkedArtifacts:[CrossLink], relatedPeople:[CrossLink], relatedPlaces:[CrossLink]}`.

### `GET /api/people`
- Item: `{id, displayName, artifactCount}`.

### `GET /api/people/{id}`
- Item: `{id, displayName, artifactTimeline:[{artifactId, title, capturedAt}], relatedTopics:[CrossLink], relatedPlaces:[CrossLink]}`.

### `GET /api/places`
- Item: `{id, displayName, location:{lat, lon} | null, artifactCount}`.

### `GET /api/places/{id}`
- Item: `{id, displayName, location:{lat, lon} | null, linkedArtifacts:[CrossLink]}`.

### `GET /api/time?from=<iso>&to=<iso>`
- Required: `from`, `to` (RFC3339). Window ≤ 365 days.
- Body: `{days: [{date: "YYYY-MM-DD", artifacts:[{id, kind, title, capturedAt}]}], nextCursor}`.

### `GET /api/graph/edges?source={kind}:{id}`
- Required: `source`. `kind ∈ {artifact, topic, person, place}`.
- Body: `{items: [CrossLink], nextCursor}`.

---

## 4. Graph Edge Resolution

`internal/graph` already stores edges between graph nodes with
metadata tags (topic-id, artifact-id, place-id, person-id, capture
date). The `graphapi.resolveEdges(sourceKind, sourceID)` helper:

1. Loads edges from `internal/graph` where `source = (kind, id)`.
2. For each edge, resolves `targetLabel` via the target resource's
   label provider (`topics.LabelOf`, `people.DisplayNameOf`, etc.).
3. Renders `reason` via the reason taxonomy lookup above.
4. Returns `[]CrossLink` sorted by strength descending (server policy;
   client must NOT re-sort).

If `internal/graph` lacks any of the metadata signals required, that
gap is a design follow-up: graph metadata MUST be sufficient to render
reasons — client may not infer them.

---

## 5. Cursor Design

Cursors are opaque base64url strings encoding
`{resource, lastSortKey, lastID}` plus a version byte. Server validates
on decode; invalid → 400 (SCN-080-11). Cursors are NOT row offsets —
they survive concurrent inserts without skipping or duplicating rows.

A single `cursor.go` implements encode/decode for all resources.

---

## 6. Configuration (SST, fail-loud)

New section in `config/smackerel.yaml`:

```yaml
knowledge_graph_api:
  list_default_limit: 50
  list_max_limit: 200
  time_window_max_days: 365
  edges_default_limit: 100
  edges_max_limit: 500
```

Loaded into `internal/config` with the standard fail-loud validator:
missing field → startup error; non-positive value → startup error. No
in-source defaults (smackerel-no-defaults).

---

## 7. Auth & Scope

- Bearer middleware: existing spec 044 chain.
- Scope middleware: existing spec 060 chain configured with the new
  surface `knowledge-graph:read`.
- Add `"knowledge-graph:read"` to
  [`internal/auth/scopes.go`](../../internal/auth/scopes.go) →
  `RegisteredScopeSurfaces`.
- Token issuance flow already supports new scopes by virtue of the
  closed-set allowlist (no other code change required).
- 401 (no bearer) and 403 (bearer without scope) follow the existing
  envelope shape used by spec 027 annotation endpoints.

---

## 8. Error Shape (uniform)

```json
{
  "error": {
    "code": "invalid_cursor | invalid_window | invalid_kind | missing_param | limit_exceeded | unauthenticated | forbidden",
    "message": "<human-readable>",
    "field": "<optional field name>"
  }
}
```

Single `graphapi.writeError` helper enforces the shape across all 8
handlers.

---

## 9. Performance Budgets

- List endpoints p95 < 200ms on warmed cache.
- Detail endpoints p95 < 100ms on warmed cache.
- `/api/time` p95 < 300ms for a 30-day window with 1k artifacts.
- `/api/graph/edges` p95 < 150ms for a typical artifact (≤ 50 edges).

Measurement: `./smackerel.sh test stress` against the live stack.

---

## 10. Test Strategy (high-level; scope owner expands)

- **Unit:** cursor codec round-trip; reason renderer for each
  taxonomy entry; limit clamp behaviour; window-validation edge cases;
  scope-middleware integration.
- **Integration:** seed Postgres with a synthetic graph; hit each of
  the 8 endpoints; assert schema-pinned JSON; assert reason strings
  match the server taxonomy.
- **E2E (live stack):** call each endpoint with a real bearer +
  `knowledge-graph:read` scope; assert 200 shape; call without scope
  → 403; call without bearer → 401; SCN-080-11/12/13/14/15 adversarial
  cases.
- **Cross-spec:** spec 073 Scope 5 PWA integration test renders a topic
  page using only `/api/topics/{id}` and a "Related" panel using only
  `/api/graph/edges` → asserts no second helper call needed.

---

## 11. Out-of-Scope (design level)

- Write paths.
- Search / FTS.
- Localization of `reason`.
- Native mobile concerns (covered by spec 073 follow-on).
- Real-time push (no SSE/WebSocket in this contract).

---

## 12. Open Design Questions (for bubbles.design)

- Should `graphapi` live under `internal/api/graphapi/` or be split as
  `internal/api/topicsapi/`, `internal/api/peopleapi/`, etc.? Bootstrap
  recommendation: one package, because the cross-link contract is
  shared and must not drift.
- Does `internal/graph` already carry enough metadata to render every
  reason in §2's taxonomy? Spike before implementation.
- Should cursors include a server-issued HMAC to prevent tampering?
  Bootstrap recommendation: yes, behind the same secret used by
  spec 027 cursors.
