# Design: 074 Capture-as-Fallback Cross-Cutting Policy

Owner: `bubbles.design`
Workflow mode: `product-to-planning`
Status ceiling for this pass: `specs_hardened`
Source requirements: [spec.md](spec.md)

## Design Brief

**Current State.** The facade already returns `AssistantResponse{Status: saved_as_idea, CaptureRoute: true}` for low-confidence turns, and Telegram invokes its capture hook. Specs 061, 064, 066, 068, and 069 rely on capture-as-fallback, but trigger conditions, provenance, dedup, abandoned-clarification handling, and telemetry are not centralized.

**Target State.** Add a transport-neutral policy module that determines fallback eligibility, writes or dedupes one Idea artifact through the existing capture path, stores provenance/dedup metadata, emits metrics and IntentTrace links, and returns one canonical saved-as-idea acknowledgement shape to all transports.

**Patterns to Follow.** Keep Idea artifacts in the existing `artifacts` graph, use `CaptureRoute` and `StatusSavedAsIdea`, preserve spec 008 explicit capture as distinct provenance, and follow fail-loud config loading patterns from `internal/config/assistant.go`. Store per-user dedup metadata outside raw telemetry so cross-user dedup is impossible.

**Patterns to Avoid.** Do not add a new artifact type, infer tags/topics/categories at capture time, dedup explicit captures against fallback captures, add a disable key, or hide missing dedup/abandonment config with runtime values.

**Resolved Decisions.** Package name is `internal/assistant/capturefallback`. Dedup is strict normalized-text equality in v1. Normalization policy is explicitly configured as `nfkc_casefold_ws_v1`. Metadata lives in `artifact_capture_policy`. Fallback `content_hash` includes user, provenance, normalized hash, and dedup bucket so the existing global artifact hash does not merge cross-user or explicit/fallback captures.

**Open Questions.** Optional `View recent ideas` placement is transport-specific and belongs to renderers; the acknowledgement shape is fixed here.

## Overview

Capture-as-fallback is a runtime guarantee: when no user-facing scenario handles a turn, the user's thought is preserved as an Idea without asking them to organize it.

| Outcome | Artifact Behavior | User Response |
|---------|-------------------|---------------|
| created | one new fallback Idea | saved-as-idea acknowledgement |
| dedup_hit | no new artifact; existing fallback Idea linked | same shape with `already_captured=true` |
| capture_failed | no artifact | soft error with trace id |
| not_eligible | no policy action | normal facade response path |

## Architecture

```text
Facade / compiler / open-knowledge result
  -> capturefallback.Policy.Decide
  -> normalize text with configured policy
  -> dedup lookup by user + provenance + normalized hash + time bucket
  -> create Idea through capture writer OR return dedup hit
  -> persist capture policy metadata
  -> emit counter + IntentTrace idea_artifact_id/cause
  -> return canonical AssistantResponse capture acknowledgement
```

| Component | Location | Responsibility |
|-----------|----------|----------------|
| Policy | `internal/assistant/capturefallback` | trigger contract, dedup, provenance, acknowledgement |
| Store | `internal/assistant/capturefallback/postgres` | dedup lookup and metadata persistence |
| Capture writer | existing capture/pipeline path | create Idea artifact |
| Facade hook | `internal/assistant` | call policy for low/unrouted/no-ground/abandoned cases |
| Trace hook | spec 071 `IntentTrace` | set cause and `idea_artifact_id` |
| Metrics | assistant metrics | counters and failure observability |

## Capability Foundation

The reusable capability is `CaptureFallbackPolicy`: a transport-neutral policy that decides when and how a user turn becomes an Idea.

| Contract | Responsibility | Consumers |
|----------|----------------|-----------|
| `Policy.Decide` | classify trigger cause and eligibility | facade, open-knowledge integration, compiler timeout sweep |
| `Policy.Capture` | normalize, dedup, write Idea, persist metadata | facade |
| `DedupStore` | per-user strict equality within configured window | policy |
| `AcknowledgementRenderer` | produce canonical response shape | all transport adapters |
| `TelemetryEmitter` | counter and IntentTrace links | dashboards and replay |

Foundation-owned behavior:

- No eligible turn may skip capture.
- Exactly one fallback Idea is created per non-dedup fallback decision.
- Explicit and fallback provenance never dedup together.
- Dedup is scoped per user.
- No inferred organization metadata is attached at capture time.

### Variation Axes

| Axis | Values | Foundation-Owned? |
|------|--------|-------------------|
| Cause | `unrouted`, `open_knowledge_no_ground`, `clarify_abandoned`, `compiler_error` | Yes |
| Provenance | `capture-as-fallback`, `capture-explicit` | Yes |
| Dedup state | new, same-window hit, outside-window new | Yes |
| Transport render | Telegram, HTTP, WhatsApp, web, Android | Shape yes, layout no |
| Normalization policy | `nfkc_casefold_ws_v1` initially | Yes |

## Concrete Implementations

### Policy Package

Package: `internal/assistant/capturefallback`.

```go
type Cause string
const (
    CauseUnrouted Cause = "unrouted"
    CauseOpenKnowledgeNoGround Cause = "open_knowledge_no_ground"
    CauseClarifyAbandoned Cause = "clarify_abandoned"
    CauseCompilerError Cause = "compiler_error"
)

type Request struct {
    UserID string
    Transport string
    TransportMessageID string
    OriginalText string
    Cause Cause
    TraceID string
    AbandonedClarification bool
    OccurredAt time.Time
}
```

### Explicit Capture Amendment

Spec 008 explicit capture writes `provenance="capture-explicit"`. It does not call the fallback dedup path and is not deduped against fallback Ideas.

## Data Model

Fallback Ideas remain rows in `artifacts` with the existing Idea artifact type.

```sql
CREATE TABLE IF NOT EXISTS artifact_capture_policy (
    artifact_id                  TEXT        PRIMARY KEY REFERENCES artifacts(id) ON DELETE CASCADE,
    user_id                      TEXT        NOT NULL,
    provenance                   TEXT        NOT NULL CHECK (provenance IN ('capture-as-fallback', 'capture-explicit')),
    fallback_cause               TEXT        CHECK (fallback_cause IN ('unrouted', 'open_knowledge_no_ground', 'clarify_abandoned', 'compiler_error')),
    normalized_text_hash         TEXT        NOT NULL,
    dedup_bucket_start           TIMESTAMPTZ,
    dedup_window_seconds         INTEGER,
    source_turn_id               TEXT        NOT NULL,
    intent_trace_id              TEXT,
    abandoned_clarification      BOOLEAN     NOT NULL,
    already_captured_source_id   TEXT,
    schema_version               INTEGER     NOT NULL,
    created_at                   TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_capture_fallback_dedup
    ON artifact_capture_policy (user_id, provenance, normalized_text_hash, dedup_bucket_start)
    WHERE provenance = 'capture-as-fallback';

CREATE INDEX IF NOT EXISTS idx_capture_policy_trace
    ON artifact_capture_policy (intent_trace_id)
    WHERE intent_trace_id IS NOT NULL;
```

For explicit captures, `dedup_bucket_start` is null and the unique fallback index does not apply.

## API/Contracts

No public endpoint is introduced.

Trigger table:

| Trigger | Cause | Capture Text |
|---------|-------|--------------|
| router no match / low band | `unrouted` | current user text |
| open-knowledge no ground | `open_knowledge_no_ground` | original user text |
| clarification timeout | `clarify_abandoned` | original pre-clarification prompt |
| compiler failure after accepted turn | `compiler_error` | original user text when policy permits capture |

Acknowledgement shape:

```json
{
  "status": "saved_as_idea",
  "body": "Saved as an idea.",
  "capture_ack": {
    "schema_version": "v1",
    "provenance": "capture-as-fallback",
    "idea_artifact_id": "artifact-id",
    "already_captured": false,
    "trace_id": "intent-trace-id"
  }
}
```

Dedup contract:

| Case | Result |
|------|--------|
| same user, same normalized text, same dedup bucket, fallback provenance | one Idea; second response has `already_captured=true` |
| same user, same text, outside dedup bucket | new Idea |
| different users, same text | separate Ideas |
| explicit capture and fallback capture, same text | separate Ideas |

## UI/UX

The acknowledgement is intentionally small and consistent.

| State | User Copy | Metadata |
|-------|-----------|----------|
| first capture | `Saved as an idea.` | `already_captured=false` |
| dedup hit | `Saved as an idea.` | `already_captured=true` |
| capture failure | safe error with trace id | no artifact id |

Transport renderers may add `View recent ideas` only where an authorized target already exists. They may not ask for tags, categories, or organization at capture time.

## Security/Compliance

- No disable key exists for capture-as-fallback.
- Dedup is per user; cross-user dedup queries are forbidden.
- Normalized text hashes use a required SST hash key, not a static literal.
- Raw text handling follows the source policy from specs 068 and 071.
- Explicit and fallback provenance are distinct persisted values.
- Telemetry labels use closed vocabularies only.

## Configuration And Migrations

Required SST keys:

| Key | Validation |
|-----|------------|
| `capture_as_fallback.dedup_window` | positive Go duration |
| `capture_as_fallback.clarify_abandon_timeout` | positive Go duration |
| `capture_as_fallback.normalization_policy` | closed value `nfkc_casefold_ws_v1` for v1 |
| `capture_as_fallback.dedup_hash_key` | non-empty secret |
| `capture_as_fallback.retention_audit_days` | integer `>= 1` for metadata audit retention decisions |

Missing keys fail startup. No key may disable fallback capture.

Migration adds `artifact_capture_policy` and indexes above. Specs 003 and 008 point to this table as the provenance/dedup authority.

## Observability

| Metric | Labels | Meaning |
|--------|--------|---------|
| `smackerel_capture_as_fallback_total` | `cause,outcome` | created, dedup_hit, capture_failed |
| `smackerel_capture_as_fallback_dedup_total` | `outcome` | dedup lookup results |
| `smackerel_capture_as_fallback_latency_seconds` | `outcome` | policy and write latency |
| `smackerel_capture_provenance_total` | `provenance` | explicit vs fallback capture counts |

IntentTrace fields: `capture_cause`, `idea_artifact_id`, and `final_response_status`. Structured logs name cause, outcome, trace id, and artifact id; raw user text is redacted.

## Testing Strategy

| Scenario | Test Type | Test Location | Assertion |
|----------|-----------|---------------|-----------|
| SCN-074-A01 | integration | `tests/integration/assistant/capture_fallback_policy_test.go` | unrouted turn creates one fallback Idea |
| SCN-074-A02 | integration | same | explicit capture provenance distinct and never deduped against fallback |
| SCN-074-A03 | unit + integration | `internal/assistant/capturefallback/dedup_test.go` | same-user same-text within window dedups |
| SCN-074-A04 | unit + integration | same | outside window creates second artifact |
| SCN-074-A05 | integration | same | cross-user same text creates separate artifacts |
| SCN-074-A06 | integration | `tests/integration/assistant/clarify_abandon_capture_test.go` | abandoned clarification captures original prompt |
| SCN-074-A07 | integration | `tests/integration/assistant/capture_trace_join_test.go` | counter and IntentTrace link artifact id |
| SCN-074-A08 | unit | `internal/config/capture_fallback_test.go` | missing SST key fails startup |
| SCN-074-A09 | guard/unit | `tests/integration/policy/capture_fallback_inviolable_test.go` | no disable key or suppression path exists |
| SCN-074-A10 | unit | `internal/assistant/capturefallback/payload_test.go` | no inferred tags/topics/categories at capture time |
| SCN-074-A11 | e2e-ui/e2e-api | cross-transport renderer tests | acknowledgement shape/copy identical |

## Risks & Open Questions

| Risk | Mitigation |
|------|------------|
| Existing global content hash collapses captures | Fallback content hash includes user/provenance/bucket inputs |
| Normalization changes break dedup history | Versioned normalization policy and stored schema_version |
| Capture failure loses trust | fail-loud user error with trace id and metric, no silent success |
| Transport renderers customize copy | golden cross-transport acknowledgement tests |
| Dedup hash key missing | startup fails before accepting turns |
