# BUG-001 — Duplicate URL share discards new context server-side

> **Parent feature:** [specs/008-telegram-share-capture](../../)
> **Parent scope:** Scope 1 — Enhanced Share-Sheet URL Capture
> **Filed by:** `bubbles.workflow` (bugfix-fastlane)
> **Filed at:** 2026-04-26
> **Severity:** P1 — user-visible credibility regression (bot lies)
> **Status:** open

---

## Symptom

When a user re-shares an already-captured URL with new context text, the Telegram bot replies:

```
. Already saved: "<title>" — updated with new context
```

(see [internal/telegram/share.go:168-177](../../../../internal/telegram/share.go))

…but the capture API duplicate path at [internal/api/capture.go:107-116](../../../../internal/api/capture.go) returns `409 DUPLICATE_DETECTED` immediately and never invokes any context-merge logic. No `MergeContext` / `UpdateContext` helper exists in `internal/db/` or `internal/pipeline/`. The newly supplied context is silently discarded.

This violates spec [scenario SC-TSC04](../../spec.md) (`"merges the new context with the existing artifact"`) and the corresponding user-validation acceptance item (currently marked `VERIFIED FAIL`).

## Reproduction

1. POST `/api/capture` with `{"url": "https://example.com/article", "context": "first context"}` → 200 OK with new artifact ID `A`.
2. POST `/api/capture` with `{"url": "https://example.com/article", "context": "second context"}` → 409 with `existing_artifact_id: A`.
3. SELECT `metadata` FROM artifacts WHERE id = A → `NULL` (no contexts retained).

Expected: artifact `A` stores both context strings.

## Root cause

`internal/pipeline/processor.go::Process` calls `DedupCheck` and short-circuits with `*DuplicateError` on hit. Nothing in that error path consults `req.Context` or persists it onto the existing artifact. The bot's "updated with new context" reply was implemented before the server-side merge, making the reply structurally untruthful.

## Fix outcome (option (a) per request)

Server-side context append onto the existing artifact:

- New helper `pipeline.MergeUserContext(ctx, exec, existingArtifactID, newContext)` writes the new context into `artifacts.metadata->'user_contexts'` (JSONB array) on the duplicate target.
- `Processor.Process` invokes the helper when `DedupCheck` returns a `DuplicateError` AND `req.Context != ""`, before re-raising the duplicate error so the API still responds `409 DUPLICATE_DETECTED` (no behavior change to the API contract — only the side-effect on the existing row).
- New unit test asserts the helper executes the expected `UPDATE` with the new context as the appended array element.
- New integration test (under `//go:build integration`) POSTs the same URL twice with different context strings and asserts `metadata->'user_contexts'` contains both values in submission order.

## Acceptance scenarios

```gherkin
Scenario: BUG-001-A Duplicate URL with new context appends to existing artifact metadata
  Given an artifact A exists for URL "https://example.com/article" with no user_contexts
  When the capture API receives POST /api/capture with the same URL and context "for the team meeting"
  Then the response is 409 DUPLICATE_DETECTED with existing_artifact_id = A
  And artifact A's metadata.user_contexts contains "for the team meeting"

Scenario: BUG-001-B Multiple re-shares accumulate contexts in submission order
  Given an artifact A already has user_contexts = ["first"]
  When the capture API receives a duplicate POST with context "second"
  Then artifact A's metadata.user_contexts equals ["first", "second"]

Scenario: BUG-001-C Duplicate URL with no new context does not modify metadata
  Given an artifact A exists for URL "https://example.com/article"
  When the capture API receives a duplicate POST with empty context
  Then the response is 409 DUPLICATE_DETECTED
  And artifact A's metadata.user_contexts is unchanged
```

## Adversarial regression

The unit test `TestMergeUserContext_AppendsContextToMetadata` asserts that the helper executes a SQL `UPDATE artifacts ... metadata ... user_contexts ...` with the new context and existing artifact ID as bound parameters. Before this fix, no such helper exists — the test would not even compile, proving the regression cannot be reintroduced without removing the helper.

## Out of scope

- Bot UX changes
- New API surface
- Schema migration (uses existing `metadata JSONB` column)
- Bug-002 (separate report)
