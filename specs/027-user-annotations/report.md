# Execution Report: 027 — User Annotations & Interaction Tracking

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Spec 027 introduces a universal annotation model for ratings, notes, tags, and interaction tracking on any artifact. All 8 scopes completed. Reconciliation pass (2026-04-21) fixed drift in wiring, NATS event publication, DeleteTag endpoint, and config SST.

---

## Scope Evidence

### Scope 1 — DB Migration
- Migration `016_user_annotations.sql` creates `annotations` table, `telegram_message_artifacts` mapping, and `artifact_annotation_summary` materialized view.
- Migration archived into consolidated schema at `internal/db/migrations/archive/016_user_annotations.sql`.
- Uses TEXT columns for annotation_type/interaction_type instead of design-specified PostgreSQL enums (functionally equivalent, simpler migration).

### Scope 2 — Annotation Types & Parser
- `internal/annotation/types.go` defines `AnnotationType`, `InteractionType`, `SourceChannel`, `Annotation`, `Summary` structs with Go constants.
- `internal/annotation/parser.go` parses freeform text into structured annotations: star ratings (1-5), hashtag tags, tag removal (`#remove-xxx`), interaction markers, and freeform notes.

### Scope 3 — Annotation Store
- `internal/annotation/store.go` provides PostgreSQL CRUD (`Create`, `CreateFromParsed`, `GetHistory`, `GetSummary`, `DeleteTag`), materialized view refresh, Telegram message-artifact mapping, and NATS event publication on `annotations.created`.
- Store accepts NATS client for event fan-out to intelligence engine.

### Scope 4 — REST API Endpoints
- `POST /api/artifacts/{id}/annotations`, `GET /api/artifacts/{id}/annotations`, `GET .../summary`, and `DELETE /api/artifacts/{id}/tags/{tag}` in `internal/api/annotations.go`.
- Handlers use `AnnotationHandlers` struct pattern (acceptable deviation from design's `Dependencies` method pattern).
- Wired into `Dependencies.AnnotationHandlers` and registered in `internal/api/router.go`.

### Scope 5 — Telegram Message-Artifact Mapping
- `internal/telegram/mapping.go` provides `recordMessageArtifact` and `resolveArtifactFromMessage` via internal API endpoints.
- `internal/api/annotations.go` exposes `POST/GET /internal/telegram-message-artifact`.

### Scope 6 — Telegram Annotation Handler
- `internal/telegram/annotation.go` implements `handleReplyAnnotation` (reply-to flow), `handleRate` (command with disambiguation), confirmation formatting.

### Scope 7 — Search Extension
- `internal/api/search_annotations.go` implements `parseAnnotationIntent` for detecting annotation-filtered queries ("my top rated", hashtag filters, interaction phrases).
- `applyAnnotationBoost` adjusts similarity scores based on rating and usage.
- Integrated into `internal/api/search.go` search pipeline.

### Scope 8 — Intelligence Integration
- `internal/intelligence/annotations.go` implements `SubscribeAnnotations` (NATS subscriber) and `updateRelevanceFromAnnotation` (relevance score delta).
- Wired in `cmd/core/main.go` at startup.

---

## Reconciliation (2026-04-21)

### Drift Found & Fixed

| Finding | Severity | Fix |
|---------|----------|-----|
| `AnnotationHandlers` not wired in `cmd/core/main.go` — annotation API was dead code | High | Added `annotation.NewStore()` + `deps.AnnotationHandlers` wiring in `main.go` |
| `DeleteTag` method missing from `store.go` and no handler/route | Moderate | Added `Store.DeleteTag()`, `AnnotationHandlers.DeleteTag()` handler, `DELETE` route |
| NATS event publication missing — `Store` had no NATS client, `annotations.created` never published | High | Added `NATS *smacknats.Client` to Store, publish loop in `CreateFromParsed` and `DeleteTag` |
| No `annotations:` config section in `config/smackerel.yaml` | Low | Added `annotations:` section with matview timeout, history limits, relevance boost coefficients |
| `AnnotationQuerier` interface not defined | Low | Acceptable — concrete `*Store` via `AnnotationHandlers` struct works for current architecture |
| Migration in `archive/` not active dir | None | Intentional — schema consolidated into main migration |

### Acceptable Drift (Not Fixed)

- API handlers use `AnnotationHandlers` struct instead of methods on `Dependencies` — cleaner separation of concerns.
- Migration uses TEXT columns instead of PostgreSQL enums — simpler, functionally equivalent.
- `Summary` struct missing `TotalEvents` and `LastAnnotated` fields from design doc — requires schema migration (see Improvement Pass I3).

### Verification

- `./smackerel.sh test unit` — 236 passed
- `./smackerel.sh check` — config in sync
- `./smackerel.sh config generate` — clean

---

## Simplification Pass (2026-04-21)

**Trigger:** `simplify-to-doc` child workflow of stochastic-quality-sweep.

### Findings & Fixes

| # | Finding | Location | Fix |
|---|---------|----------|-----|
| S1 | `regexp.MustCompile(\s+)` recompiled on every call to `Parse()` and `parseAnnotationIntent()` | `internal/annotation/parser.go:83`, `internal/api/search_annotations.go:55` | Pre-compiled as package-level vars `whitespaceRe` and `whitespaceNormRe` |
| S2 | Duplicate interaction phrase list in `splitRateArgs()` — hardcoded `[]string{"made it", ...}` duplicates knowledge in `annotation.interactionMap` | `internal/telegram/annotation.go:315` | Exported `annotation.InteractionPhrases()` function; `splitRateArgs` now calls it instead of maintaining a separate list |

### Not Actionable (Reviewed, No Change Needed)

- `CreateFromParsed` makes N individual `INSERT` statements per parsed component (max ~5 per annotation). Batch INSERT would save round-trips but N is always small; the transactional clarity of individual inserts is preferable at this scale.
- `humanizeInteraction()` in telegram package is a display-layer reverse map — separate from the parser's detection map. Not a true duplication since they serve different purposes (display vs. parsing).

### Verification

- `./smackerel.sh test unit` — all passed (Go + Python)
- `./smackerel.sh lint` — all checks passed

---

## Improvement Pass (2026-04-21)

**Trigger:** `improve-existing` child workflow of stochastic-quality-sweep.

### Findings & Fixes

| # | Finding | Severity | Location | Fix |
|---|---------|----------|----------|-----|
| I1 | Missing `AnnotationQuerier` interface — design doc and scopes.md specify it; API handlers hold concrete `*Store` instead of interface, preventing unit testing without DB | Moderate | `internal/annotation/types.go`, `internal/api/annotations.go` | Added `AnnotationQuerier` interface to `types.go` with all 6 store methods; changed `AnnotationHandlers.Store` from `*annotation.Store` to `annotation.AnnotationQuerier`; added compile-time assertion `var _ AnnotationQuerier = (*Store)(nil)` |
| I2 | `GetSummary` swallows all errors — connection failures, timeout, permission errors all return empty summary instead of surfacing the error | Moderate | `internal/annotation/store.go:176` | Now uses `errors.Is(err, pgx.ErrNoRows)` to distinguish "not found" (returns empty summary) from real errors (returns error to caller) |
| I3 | `Summary` struct missing `TotalEvents` and `LastAnnotated` fields from design doc | Low | `internal/annotation/types.go`, migration | Noted — requires schema migration to add columns to materialized view; design-to-implementation drift documented, no code change |

### Verification

- `./smackerel.sh test unit` — all passed (Go 41 packages + Python 236 tests)
- `./smackerel.sh check` — config in sync, env_file drift guard OK
