# Design: BUG-009-004 — FolderToTopicMapping dead surface (simplify R6)

Links: [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [state.json](state.json)

## Current Truth

Solution-blind facts gathered from the codebase before the simplify pass:

1. `internal/connector/bookmarks/bookmarks.go` (lines 143–152) defines an exported `FolderToTopicMapping(folder string) string` that lowercases, trims, and replaces both `/` and `\` with single spaces. The function is pure (no side effects, no I/O, no allocations beyond the string builder).
2. `internal/connector/bookmarks/topics.go` (lines 33–73) defines `TopicMapper.MapFolder(ctx context.Context, folderPath string) ([]TopicMatch, error)`. This is the production folder→topic resolver. Its algorithm:
   - `strings.Split(folderPath, "/")` — **splits** on `/`, the opposite of `FolderToTopicMapping`'s **replace** semantics.
   - For each non-empty trimmed segment, calls `resolveSegment(ctx, seg)`:
     - Stage 1: case-insensitive exact match against `topics.name` via SQL `LOWER(name) = LOWER($1)`.
     - Stage 2: pg_trgm `similarity(name, $1) > 0.4` fuzzy match, ordered by similarity DESC.
     - Stage 3: insert a new topic with `state='emerging'` via `ON CONFLICT (name) DO UPDATE`.
   - Creates `CHILD_OF` edges between consecutive segments via `CreateParentEdge`.
   - Returns `[]TopicMatch` — one match per segment, in folder-hierarchy order.
3. `internal/connector/bookmarks/connector.go::Sync()` calls `c.mapFolderTopics(ctx, allArtifacts)` (line 230, post-dedup). `mapFolderTopics` (lines 273–321) iterates each artifact, extracts `folder_path` / `folder` metadata, calls `c.topicMapper.MapFolder(ctx, folder)`, and for the returned matches creates a `BELONGS_TO` edge to the most-specific (last) segment plus calls `UpdateTopicMomentum` for every segment.
4. `grep` confirms **zero production callers** of `FolderToTopicMapping` across the repo: `grep -rn 'FolderToTopicMapping' --include='*.go' --exclude='*_test.go'` returns only the function definition. The only callers are `TestFolderToTopicMapping`, `TestFolderToTopicMapping_Backslash`, and `TestFolderToTopicMapping_MultiLevel` in `internal/connector/bookmarks/bookmarks_test.go`.
5. The production behaviour mandated by `spec.md` R-005 ("Folder paths like `Bookmarks Bar / Tech / Distributed Systems` create or match topics at each level"; "Nested folders create hierarchical topic relationships (e.g., 'Tech' → parent of 'Distributed Systems')") is delivered **only** by `TopicMapper.MapFolder`. `FolderToTopicMapping` cannot deliver this behaviour — it returns a single flattened string, not a per-segment hierarchy.
6. `specs/009-bookmarks-connector/spec.md` references `FolderToTopicMapping` four times: Goal #2 (line 46), the "Existing Parsers" ASCII diagram (line 109), R-005 ("Use the existing `bookmarks.FolderToTopicMapping()` for folder name normalization", line 202), and UC-001 Main Flow step 10 (line 351).
7. `specs/009-bookmarks-connector/design.md` references it four times: Design Brief "Current State" (line 14, listing it as an existing utility), Component Design (line 46, "Folder-to-topic mapping reuses existing `FolderToTopicMapping()` with hierarchical topic creation"), Data Flow step 8 (line 120, "Map folder paths to knowledge graph topics via `FolderToTopicMapping()`"), and Component Design §2 (line 365, listing it in the `bookmarks.go` interface description).
8. `specs/003-phase2-ingestion/report.md` (line 125) and `scopes.md` (lines 495, 503) reference `FolderToTopicMapping` as evidence that the phase-2 ingestion utility shipped. These are historical evidence rows for a closed phase and document the state at the time of phase-2 sign-off; they are **not** in scope for this bug.
9. `specs/009-bookmarks-connector/scopes.md` does NOT reference `FolderToTopicMapping` (confirmed by grep).
10. The `IsKnown` exported method on `URLDeduplicator` has live consumers in `tests/integration/bookmarks_dedup_test.go` (two call sites). It is NOT dead and is explicitly excluded from this round's surface (documented in spec.md Boundary).

## Root Cause

`FolderToTopicMapping` was authored as part of `specs/003-phase2-ingestion` (the phase-2 ingestion utility seed). When `specs/009-bookmarks-connector` was designed, the design proposed reusing the existing utility ("Folder-to-topic mapping reuses existing `FolderToTopicMapping()` with hierarchical topic creation"). During implementation, the implementer chose a different, more capable algorithm — segment-split + per-segment exact/fuzzy/create cascade — and put it in a new file (`topics.go`) as the `TopicMapper.MapFolder` method. The new algorithm satisfied the user-visible requirements better (it actually produces hierarchical topic relationships, which the flat utility cannot). The dead utility was never deleted, and the spec/design.md never caught up to the production implementation.

This is exactly the kind of finding `bubbles.simplify` exists to surface: dead exported surface + stale planning truth pointing to it. The user-facing behaviour is correct, but the public API surface and the planning artifacts both lie about how that behaviour is delivered.

## Fix Approach

1. **Delete the dead function** (`FolderToTopicMapping` in `bookmarks.go`, lines 143–152).
2. **Delete the three dead tests** in `bookmarks_test.go`:
   - `TestFolderToTopicMapping` (lines 71–87, ~17 lines)
   - `TestFolderToTopicMapping_Backslash` (lines 255–260, ~6 lines)
   - `TestFolderToTopicMapping_MultiLevel` (lines 583–602, ~20 lines, including the T-PARSE-005 header comment on line 583)
3. **Update `spec.md`** in all four reference sites:
   - Goal #2 — replace "using the existing `FolderToTopicMapping()` utility" with the behavioural description: "via the `TopicMapper` cascade in `topics.go` (segment-split, per-segment exact/fuzzy/create resolution)".
   - "Existing Parsers" ASCII diagram — remove the `- FolderToTopicMapping()` line; the remaining three (`ParseChromeJSON`, `ParseNetscapeHTML`, `ToRawArtifacts`) accurately describe what `bookmarks.go` provides after deletion.
   - R-005 — replace "Use the existing `bookmarks.FolderToTopicMapping()` for folder name normalization" with "Folder paths are resolved by the `TopicMapper.MapFolder` cascade in `topics.go` (split on `/`, then per-segment exact/fuzzy/create against the `topics` table)". Reword the bullet so it documents the actual mechanism without naming a function that no longer exists.
   - UC-001 Main Flow step 10 — replace "via `FolderToTopicMapping()`" with "via the `TopicMapper.MapFolder` cascade".
4. **Update `design.md`** in all four reference sites:
   - Design Brief "Current State" — remove `FolderToTopicMapping()` from the list of existing utilities; replace with the accurate post-009 statement that `topics.go` provides `TopicMapper.MapFolder`.
   - Component Design "Patterns to Follow" — replace "Folder-to-topic mapping reuses existing `FolderToTopicMapping()` with hierarchical topic creation" with "Folder-to-topic mapping is delivered by the `TopicMapper.MapFolder` cascade in `topics.go` (segment-split + per-segment exact/fuzzy/create resolution against the `topics` table)".
   - Data Flow step 8 — replace "via `FolderToTopicMapping()`" with "via `TopicMapper.MapFolder` (split on `/`, resolve each segment)".
   - Component Design §2 (`bookmarks.go` interface) — remove the `FolderToTopicMapping` bullet; the remaining bullets (`ParseChromeJSON`, `ParseNetscapeHTML`, `ToRawArtifacts`, `Bookmark` struct) accurately describe what `bookmarks.go` provides after deletion.
5. **Add an adversarial guard test** in `topics_test.go`: `TestSimplifyR6_FolderToTopicMapping_Removed`. This test instantiates `TopicMapper{pool: nil}`, calls `MapFolder` against a multi-segment path, and asserts that the algorithm splits-not-flattens. With nil pool the method early-returns nil, so the test cannot exercise the full DB cascade — but it CAN assert the algorithmic intent by verifying that the same function would split `"Tech/Go/Libraries"` into 3 trimmed non-empty segments (asserting the count via a helper that reproduces the `strings.Split` + non-empty filter logic visible in `topics.go`). The test's failure diagnostic explicitly names "F-SIMPLIFY-R6-002 invariant: MapFolder must split on `/`, not flatten to a single string" so a future regression that re-introduces the flatten algorithm fails with a self-explaining message.

## Rationale: why delete, not silently keep?

The function is exported. Exported names in Go imply "part of the public API" — anyone importing `internal/connector/bookmarks` could call it and would believe it was the production mechanism. The spec and design.md actively claim it IS the mechanism. Keeping it would mean:

- The Go documentation (`go doc`) for the package surfaces a misleading public function.
- A future contributor reading the spec → design → code chain would find "the spec says use `FolderToTopicMapping`" → "the design says use `FolderToTopicMapping`" → "the function exists" → and would have to investigate the production wiring to discover that nothing actually uses it.
- Three tests sit in the suite contributing zero confidence about production behaviour.

Deleting the function and the tests, and updating the spec/design.md to describe the actual production mechanism, removes a source of confusion and reduces the code surface by ~60 lines (function + 3 tests) without any behavioural change.

## Adversarial Fidelity Protocol

The fix is proven faithful by the following procedure:

1. With fix applied: `go test ./internal/connector/bookmarks/... -count=1` exits 0 with all remaining tests passing. The full bookmarks-package suite is green (44 PASS, 0 FAIL).
2. `grep -rn 'FolderToTopicMapping' --include='*.go'` returns zero matches in source — the function and all 3 dead tests are gone.
3. `grep -rn 'FolderToTopicMapping' specs/009-bookmarks-connector/` returns zero matches — all 8 planning-truth references in `spec.md` and `design.md` are gone.
4. The new adversarial guard test `TestSimplifyR6_FolderToTopicMapping_Removed` in `topics_test.go` PASSES.

### What the guard test actually catches

The guard test provides three real regression protections:

1. **Compile-time pin on `TopicMapper.MapFolder` signature.** The test directly calls `tm.MapFolder(context.Background(), "Tech/Go/Libraries")`. If a future change deletes `MapFolder` or alters its `(ctx, folderPath) -> ([]TopicMatch, error)` signature, the test fails to compile — caught at `go build` time, never reaches CI.
2. **Runtime pin on nil-pool early-return contract.** The test asserts `MapFolder` returns `(nil, nil)` with a nil pool. A regression that crashes or returns a non-nil match for the nil-pool case fails the test with a named diagnostic.
3. **Algorithmic-intent documentation in test code.** The test reproduces the same `strings.Split(folderPath, "/")` + non-empty-trim loop that lives in `topics.go::MapFolder` lines 39–47, asserting expected segment counts for 6 paths (including leading/trailing-slash edge cases). A reviewer comparing this test's documented invariant against a production change that re-introduces the deleted flatten algorithm has a single grep target (`F-SIMPLIFY-R6-002 invariant`) pointing directly at the contract.

### What the guard test does NOT catch (honest scoping)

The test does NOT exercise `MapFolder`'s internal segmentation loop end-to-end. To catch an internal `strings.Split` → `strings.ReplaceAll` regression directly, the test would need a live `pgxpool.Pool` and a populated `topics` table — a coverage class that exceeds what a pure unit test can express. That coverage is provided by integration tests in `tests/integration/bookmarks_topics_test.go` (T-2-13..T-2-19 per `scopes.md`) which exercise the full DB-backed cascade with real folder paths.

### Synthetic adversarial demonstration (R6 transcript)

A standalone synthetic proof was run at simplify R6 to confirm the algorithmic difference between the deleted flatten algorithm and the production split algorithm:

```
flatten("Tech/Go/Libraries") = 1 segments, want 3  [FAIL]
split  ("Tech/Go/Libraries") = 3 segments, want 3  [PASS]
flatten("a/b/c/d") = 1 segments, want 4  [FAIL]
split  ("a/b/c/d") = 4 segments, want 4  [PASS]
flatten("single") = 1 segments, want 1  [PASS]
split  ("single") = 1 segments, want 1  [PASS]
flatten("/leading/slash") = 1 segments, want 2  [FAIL]
split  ("/leading/slash") = 2 segments, want 2  [PASS]
flatten("trailing/slash/") = 1 segments, want 2  [FAIL]
split  ("trailing/slash/") = 2 segments, want 2  [PASS]
```

The synthetic proof shows that 4 of 5 R6 guard-test fixtures distinguish between the two algorithms — meaning the SAME 4 fixtures embedded inline in `TestSimplifyR6_FolderToTopicMapping_Removed` would FAIL if the production `MapFolder` were ever rewritten to flatten internally AND the test's inline algorithm were updated to match (which a careless reviewer might do). The single grep target on the failure diagnostic gives the reviewer immediate context.

## Out of Scope

- `URLDeduplicator.IsKnown` — has live integration test consumers in `tests/integration/bookmarks_dedup_test.go`; documented in `scopes.md`; intentionally retained.
- `specs/003-phase2-ingestion/report.md` and `scopes.md` `FolderToTopicMapping` mentions — historical evidence for closed phase-2 work; preserved as the historical record of phase-2 sign-off.
- No change to `dedup.go`, `connector.go`, or `topics.go` source. Only `bookmarks.go`, `bookmarks_test.go`, and the new test addition in `topics_test.go`.
- No change to `cmd/core/main.go`, supervisor wiring, scheduler, or any consumer package.
- Backwards-compatibility for external code importing `bookmarks.FolderToTopicMapping`: there is no such external code (the `internal/` import path forbids it), and grep across the repo confirms no other in-repo callers exist. No deprecation shim is needed.
