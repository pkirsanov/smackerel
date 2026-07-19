# Bug Fix Design: BUG-038-002

## Root Cause Analysis

### Investigation Summary

The failing test performs synchronous Drive scan and extraction through the real PostgreSQL stores, then confirms both artifacts through `consumers.LoadDriveArtifact` before calling the live core API. Fresh-stack passes and broad-run failures made persistence, semantic/text mode, query collision, identity, audience, and cleanup the discriminating hypotheses. A controlled twenty-row exact-title contaminant proved the fixed generic query was the deciding variable.

### Discriminating Hypotheses

| Hypothesis | Evidence that confirms it | Evidence that rejects it |
|---|---|---|
| Nonempty semantic candidates suppress lexical Drive matches | Direct DB text query finds both IDs; live response reports semantic mode with other IDs | Vector query contains both IDs or text search cannot find them |
| Scan/extract persistence is incomplete | Drive/artifact rows or searchable content are absent after `ProcessPending` returns | Rows and matching text are present in the same transaction-visible database |
| Query terms do not match the persisted search document | PostgreSQL `websearch_to_tsquery` does not match either row | Both rows match title/content directly |
| Auth, tenant, audience, or provider identity excludes rows | Authenticated identity/audience predicates reject the fixture owners | Search query has no such predicate or aligned fixture identity restores both rows |
| Cleanup from a neighboring test removes rows | Exact IDs disappear before the API request | Exact IDs remain present through request completion |

### Root Cause

The test query, not scan/extract persistence, was non-isolated. `tomato salad` is a generic fixed phrase evaluated against one shared E2E database. Search is correctly bounded to 20 rows; earlier-package exact-title matches can occupy all 20 slots. A fresh stack passed because it had no competing rows. Adding twenty exact-title contaminants reproduced the original `google=false mem=false` result while the next observability test remained healthy. This rules out eventual consistency, provider identity, audience filtering, cleanup deletion, and core instability for BUG-038-002.

### Impact Analysis

- Affected components: Drive cross-feature E2E fixture/query isolation.
- Affected data: disposable test artifacts only.
- Affected users: contributors and release validation; production search behavior is unchanged.

## Fix Design

### Solution Approach

1. Generate a collision-resistant alphanumeric term per test run.
2. Include that exact term in both Google and memdrive fixture titles/content.
3. Query the exact term through the authenticated live API.
4. Keep twenty generic `Tomato salad` contenders in the same database so removing the unique-term isolation makes the regression fail again.
5. Preserve exact artifact ID, provider metadata, consumer-surface, and cleanup assertions.

### Regression Design

- Preserve the live two-provider scenario and exact provider assertions.
- Add twenty earlier-package exact-title contenders so a regression to the generic query fails at the bounded result window.
- Assert direct persisted state before the live request so persistence and retrieval failures are classified separately.
- Assert response `search_mode`, exact IDs, and provider metadata without mocks or interception.

## Change Boundary

Allowed surfaces after root-cause confirmation:

- `tests/e2e/drive/drive_cross_feature_e2e_test.go`
- this `BUG-038-002` packet

Excluded surfaces:

- provider/audience policy weakening
- Drive scan package serialization
- synthesis and assistant packets
- deploy adapters, manifests, secrets, `knb`, and release-train bundles

## Complexity Tracking

None - the smallest viable fix isolates the test query and retains an adversarial shared-corpus collision fixture.
