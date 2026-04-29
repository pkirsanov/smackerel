# BUG-001 Design — Server-side context merge on duplicate capture

## Current Truth

- `Processor.Process` ([internal/pipeline/processor.go](../../../../internal/pipeline/processor.go)) returns `*DuplicateError` immediately when `DedupCheck` reports a hit. No code path examines `req.Context` afterwards.
- `CaptureHandler` ([internal/api/capture.go](../../../../internal/api/capture.go)) translates the `*DuplicateError` to `409 DUPLICATE_DETECTED` with `existing_artifact_id` and `title` — no merge.
- The artifacts table already has `metadata JSONB` ([internal/db/migrations/001_initial_schema.sql:62](../../../../internal/db/migrations/001_initial_schema.sql)). No schema change required.
- The Telegram bot ([internal/telegram/share.go:168-177](../../../../internal/telegram/share.go)) advertises a merge that does not happen. Fixing the server side aligns code with the user-visible promise.

## Change shape

1. **New file `internal/pipeline/merge.go`:**
   - Exports a small `Execer` interface satisfied by `*pgxpool.Pool`.
   - Exports `MergeUserContext(ctx, exec Execer, artifactID, newContext string) error`.
   - SQL: `UPDATE artifacts SET metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{user_contexts}', COALESCE(metadata->'user_contexts', '[]'::jsonb) || jsonb_build_array($1::text), true), updated_at = NOW() WHERE id = $2`.
   - No-op (returns `nil`) when `newContext == ""` or `artifactID == ""`.

2. **Modify `internal/pipeline/processor.go::Process`:**
   - On `DedupCheck` returning `*DuplicateError`: if `req.Context != ""` and `p.DB != nil`, call `MergeUserContext(ctx, p.DB, dupErr.ExistingID, req.Context)`. Failures are logged with `slog.Warn` but do NOT alter the `*DuplicateError` returned to the caller (preserves API contract — duplicate is duplicate).

3. **New unit test `internal/pipeline/merge_test.go`:**
   - Fake `Execer` records `(sql, args, calls)`.
   - `TestMergeUserContext_AppendsContextToMetadata`: verifies one call with SQL substring `metadata`, `user_contexts`, and args `[newContext, artifactID]`.
   - `TestMergeUserContext_NoOpOnEmptyContext`: zero calls.
   - `TestMergeUserContext_NoOpOnEmptyArtifactID`: zero calls.
   - `TestMergeUserContext_PropagatesExecError`: returned error wraps the underlying error.

4. **New integration test `tests/integration/capture_duplicate_context_test.go`** (`//go:build integration`):
   - Inserts a stub artifact for URL `https://example.com/bug001-<testID>` with `metadata = NULL`.
   - Calls `MergeUserContext` directly against the live pool (twice, two different contexts).
   - Asserts `SELECT metadata->'user_contexts' FROM artifacts WHERE id = $1` returns the JSON array `["first context", "second context"]` in submission order.

## Backward compatibility

- API responses unchanged: still `409 DUPLICATE_DETECTED`.
- Bot reply unchanged.
- No new config keys.
- No schema migration.
- Artifacts whose `metadata` was previously `NULL` get a fresh `{"user_contexts": [...]}` object on first merge. Existing keys in `metadata` are preserved by `jsonb_set`.

## Rejected alternatives

- **Add a dedicated `user_contexts` TEXT[] column.** Heavier schema change for a Scope-1 bug fix; metadata JSONB already provides the storage idiom used elsewhere (e.g., expense tracking).
- **Make the bot reply truthful instead (option (b)).** Rejected — spec promises merge; we should make the spec true.
- **Propagate the merge error to the API caller as 5xx.** Rejected — the duplicate is the primary signal; failed merge is a degraded but non-fatal side effect, logged for ops.
