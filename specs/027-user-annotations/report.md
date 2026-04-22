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
| I3 | `Summary` struct missing `TotalEvents` and `LastAnnotated` fields from design doc | Low | `internal/annotation/types.go`, migration | **Resolved in Reconciliation Pass (2026-04-22)** — added columns to materialized view and fields to Go struct |

### Verification

- `./smackerel.sh test unit` — all passed (Go 41 packages + Python 236 tests)
- `./smackerel.sh check` — config in sync, env_file drift guard OK

---

## Security Pass (2026-04-22)

**Trigger:** `security-to-doc` child workflow of stochastic-quality-sweep.

### Security Scan Surface

Reviewed all code files owned by spec 027:
- `internal/annotation/types.go`, `parser.go`, `store.go` — data model, parser, database CRUD
- `internal/api/annotations.go` — REST API handlers (POST/GET/DELETE)
- `internal/api/search_annotations.go` — search intent detection and annotation boost
- `internal/telegram/annotation.go` — Telegram reply-to annotation, `/rate` command, disambiguation
- `internal/intelligence/annotations.go` — NATS subscriber, relevance score updates
- `internal/api/router.go` — route registration, auth middleware

### Findings & Fixes

| # | Finding | Severity | OWASP | Location | Fix |
|---|---------|----------|-------|----------|-----|
| SEC-027-001 | `CreateAnnotation` and `RecordTelegramMessageArtifact` POST handlers decode request body via `json.NewDecoder` without `http.MaxBytesReader` — allows unbounded request bodies, enabling memory exhaustion DoS. Other POST handlers in the codebase (`CaptureHandler`, `BookmarkImportHandler`, `ExpenseHandler`) all enforce `MaxBytesReader`. | High | A05:2021 Security Misconfiguration | `internal/api/annotations.go:35`, `internal/api/annotations.go:134` | Added `r.Body = http.MaxBytesReader(w, r.Body, maxAnnotationBodySize)` (64 KB limit) to both handlers |
| SEC-027-002 | No annotation text length validation — freeform `text` field accepted at any length. Combined with SEC-027-001 fix, bodies are capped at 64 KB, but a dedicated text length limit provides defense-in-depth against storage amplification. | Medium | A03:2021 Injection (storage) | `internal/api/annotations.go:50` | Added `maxAnnotationTextLen = 2000` constant and validation before parsing |
| SEC-027-003 | `DeleteTag` API accepts tag from URL parameter without pattern validation — the parser uses `[\w-]+` regex for tags, but the API endpoint accepted any string including special characters. | Low | A03:2021 Injection | `internal/api/annotations.go:216` | Added `validTagRe` pattern check (`[\w-]+`) on the `{tag}` URL parameter before passing to store |

### Not Actionable (Reviewed, No Issue)

- **SQL injection:** All database queries use parameterized queries (`$1`, `$2`, etc.) — no string concatenation in SQL.
- **Authentication:** All annotation endpoints registered inside `bearerAuthMiddleware` group in `router.go` — requires valid auth token.
- **CORS:** Controlled by SST config (`cors.allowed_origins` in `smackerel.yaml`), not hardcoded.
- **Annotation parser ReDoS:** Regex patterns in `parser.go` are simple and bounded — no catastrophic backtracking risk.
- **NATS event deserialization:** `SubscribeAnnotations` unmarshals NATS messages into typed `annotation.Annotation` struct — no arbitrary code execution.
- **Telegram input:** Reply-to annotation text goes through the same `annotation.Parse()` pure function — no special Telegram-specific injection risk.
- **Internal endpoints:** `/internal/telegram-message-artifact` endpoints are behind auth middleware — no unauthenticated access.
- **Relevance score clamping:** `updateRelevanceFromAnnotation` clamps score to `[0, 1]` — no overflow or unbounded scoring.

### Security Tests Added

| Test | File | Finding | Assertion |
|------|------|---------|-----------|
| `TestCreateAnnotation_OversizedBody` | `internal/api/annotations_test.go` | SEC-027-001 | 70 KB body → 400 Bad Request |
| `TestCreateAnnotation_TextTooLong` | `internal/api/annotations_test.go` | SEC-027-002 | 2001-char text → 400 with "too long" |
| `TestCreateAnnotation_TextAtLimit` | `internal/api/annotations_test.go` | SEC-027-002 | 2000-char text → 201 Created |
| `TestRecordTelegramMessageArtifact_OversizedBody` | `internal/api/annotations_test.go` | SEC-027-001 | 70 KB body → 400 Bad Request |
| `TestDeleteTag_InvalidTagFormat` | `internal/api/annotations_test.go` | SEC-027-003 | Special chars in tag → 400; valid tags → 200 |

### Verification

- `./smackerel.sh test unit` — all passed (Go 41 packages + Python 236 tests)
- `./smackerel.sh build` — clean build

---

## Reconciliation Pass (2026-04-22)

**Trigger:** `reconcile-to-doc` child workflow of stochastic-quality-sweep.

### Audit Method

Full claimed-vs-implemented audit of all 8 scopes against:
- Go source code in `internal/annotation/`, `internal/api/`, `internal/telegram/`, `internal/intelligence/`
- SQL migration in `internal/db/migrations/001_initial_schema.sql` and `archive/016_user_annotations.sql`
- Wiring in `cmd/core/main.go` and `internal/api/router.go`
- Config in `config/smackerel.yaml` and `config/nats_contract.json`

### Drift Found & Fixed

| # | Finding | Severity | Location | Fix |
|---|---------|----------|----------|-----|
| R2-001 | DoD Scope 1 claims materialized view has `total_events` and `last_annotated` columns — but these were missing from SQL definition and Go `Summary` struct | High | `001_initial_schema.sql`, `archive/016_user_annotations.sql`, `internal/annotation/types.go`, `internal/annotation/store.go` | Added `COUNT(*)::INTEGER AS total_events` and `MAX(a2.created_at) AS last_annotated` to materialized view; added `TotalEvents int` and `LastAnnotated *time.Time` to `Summary` struct; updated `GetSummary` query to scan the new fields |

### Stale Report Entries Corrected

- Report's "Acceptable Drift" section previously listed "`Summary` struct missing `TotalEvents` and `LastAnnotated` fields" and Improvement Pass I3 noted it as deferred. This is now resolved by R2-001 above.
- Report's "Acceptable Drift" section previously listed "`AnnotationQuerier` interface not defined" — this was already fixed by Improvement Pass I1 and is no longer accurate.

### Verified Surfaces (No Drift)

| Surface | Status |
|---------|--------|
| `internal/annotation/types.go` — all 6 annotation types, 6 interaction types, 3 channels, `Annotation`, `Summary`, `ParsedAnnotation`, `AnnotationQuerier` | Matches design |
| `internal/annotation/parser.go` — `Parse()` with rating, tags, tag removal, interaction detection, freeform notes | Matches design |
| `internal/annotation/store.go` — `Create`, `CreateFromParsed`, `GetHistory`, `GetSummary`, `DeleteTag`, `RefreshSummary`, `RecordMessageArtifact`, `ResolveArtifactFromMessage`, NATS publish | Matches design |
| `internal/api/annotations.go` — POST/GET annotations, GET summary, DELETE tag, POST/GET internal telegram mapping | Matches scopes |
| `internal/api/search_annotations.go` — `parseAnnotationIntent`, `applyAnnotationBoost` | Matches design |
| `internal/telegram/annotation.go` — `handleReplyAnnotation`, `handleRate`, disambiguation store, confirmation formatting | Matches design |
| `internal/telegram/mapping.go` — `recordMessageArtifact`, `resolveArtifactFromMessage`, `replyWithMapping` | Matches design |
| `internal/intelligence/annotations.go` — `SubscribeAnnotations`, `updateRelevanceFromAnnotation`, `annotationRelevanceDelta`, `ResurfacingCandidates` | Matches design |
| `cmd/core/main.go` wiring — `annotation.NewStore`, `deps.AnnotationHandlers`, `intEngine.SubscribeAnnotations` | Wired correctly |
| `internal/api/router.go` — all 7 annotation routes registered | Matches design |
| `config/smackerel.yaml` — `annotations:` section with matview timeout, limits, boost coefficients | Present and correct |
| `config/nats_contract.json` — `annotations.created` subject | Present and correct |
| `internal/nats/client.go` — `SubjectAnnotationsCreated` constant | Matches contract |

### Verification

- `./smackerel.sh build` — clean build
- `./smackerel.sh test unit` — all passed (Go 41 packages + Python 238 tests)
