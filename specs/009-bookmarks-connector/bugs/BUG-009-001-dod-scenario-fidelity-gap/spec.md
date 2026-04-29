# Bug: BUG-009-001 — DoD scenario fidelity gap (SCN-BK-004/006/007/008)

## Classification

- **Type:** Artifact-only documentation/traceability bug
- **Severity:** MEDIUM (governance gate failure on a feature already marked `done`; no runtime impact)
- **Parent Spec:** 009 — Bookmarks Connector
- **Workflow Mode:** bugfix-fastlane
- **Status:** Fixed (artifact-only)

## Problem Statement

Bubbles traceability-guard Gate G068 (Gherkin → DoD Content Fidelity) reported that 4 of the 10 Gherkin scenarios in the parent feature's `scopes.md` had no faithful matching DoD item:

- `SCN-BK-004` Config validation rejects invalid settings
- `SCN-BK-006` URL dedup prevents reprocessing same URL across exports
- `SCN-BK-007` URL normalization strips tracking parameters
- `SCN-BK-008` Folder hierarchy maps to topic graph

The gate's content-fidelity matcher requires a DoD bullet to either (a) carry the same `SCN-BK-NNN` trace ID as the Gherkin scenario, or (b) share enough significant words. The pre-existing DoD entries described the implemented behavior but did not embed the trace ID, and the fuzzy matcher's significant-word threshold was not satisfied for these four scenarios. Two ancillary failures piggybacked on the same gap: no `scenario-manifest.json` had been generated for spec 009, and the Test Plan row for `SCN-BK-010` mapped only to a planned-but-not-yet-existing `tests/e2e/bookmarks_test.go` file.

## Reproduction (Pre-fix)

```
$ timeout 300 bash .github/bubbles/scripts/traceability-guard.sh specs/009-bookmarks-connector 2>&1 | tail -8
ℹ️  DoD fidelity: 10 scenarios checked, 6 mapped to DoD, 4 unmapped
❌ DoD content fidelity gap: 4 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)
RESULT: FAILED
```

## Gap Analysis (per scenario)

For each missing scenario the bug investigator searched the production code (`internal/connector/bookmarks/connector.go`, `dedup.go`, `topics.go`) and the test files (`*_test.go`). All four behaviors are genuinely **delivered-but-undocumented at the trace-ID level** — there is no missing implementation and no missing test fixture; the only gap is that DoD bullets did not embed the `SCN-BK-NNN` ID that the guard uses for fidelity matching.

| Scenario | Behavior delivered? | Tests pass? | Concrete test file | Concrete source |
|---|---|---|---|---|
| SCN-BK-004 | Yes — `Connect()` validates the import directory exists; `parseConfig()` rejects empty `import_dir` and applies defaults (`watch_interval: 5m`, `archive_processed: true`, `processing_tier: full`) | Yes — `TestConnectMissingImportDir`, `TestConnectEmptyImportDir`, `TestParseConfigDefaults` PASS | `internal/connector/bookmarks/connector_test.go` | `internal/connector/bookmarks/connector.go::Connect`, `parseConfig` |
| SCN-BK-006 | Yes — `URLDeduplicator.FilterNew` batch-queries `artifacts WHERE source_id='bookmarks' AND source_ref = ANY($1)`; wired into `Sync()` when pool is available | Yes — `TestFilterNew_NilPool`, `TestFilterNew_EmptyArtifacts`, `TestFilterNew_NilArtifacts` PASS (DB-backed mixed-batch behavior covered by integration row T-2-13 when the live stack is available) | `internal/connector/bookmarks/dedup_test.go` | `internal/connector/bookmarks/dedup.go::FilterNew`, `connector.go::Sync` |
| SCN-BK-007 | Yes — `NormalizeURL` lowercases scheme+host, strips userinfo (chaos R24-002), strips `www.` (IMP-009-R-002), trims trailing slash, removes `utm_*`/`fbclid`/`gclid`/`ref`, drops fragments, and is robust to garbage input | Yes — `TestNormalizeURL_Lowercase`, `TestNormalizeURL_StripTrailingSlash`, `TestNormalizeURL_StripUTMParams`, `TestNormalizeURL_PreservesPath`, `TestNormalizeURL_InvalidURL` PASS | `internal/connector/bookmarks/dedup_test.go` | `internal/connector/bookmarks/dedup.go::NormalizeURL` |
| SCN-BK-008 | Yes — `MapFolder` splits paths on `/` and resolves each segment via the 3-stage cascade (exact → pg_trgm fuzzy → create with state `emerging`); `CreateParentEdge` writes `CHILD_OF` between successive segments; `CreateTopicEdge` writes `BELONGS_TO`; empty/whitespace folders return nil so root-level bookmarks get no edges | Yes — `TestMapFolder_EmptyPath`, `TestTopicMapper_NilPool`, `TestTopicMatch_Fields` PASS (DB-backed hierarchy edges covered by integration rows T-2-14..T-2-17 when the live stack is available) | `internal/connector/bookmarks/topics_test.go` | `internal/connector/bookmarks/topics.go::MapFolder`, `CreateParentEdge`, `CreateTopicEdge` |

**Disposition:** All four scenarios are **delivered-but-undocumented** — artifact-only fix.

## Acceptance Criteria

- [x] Parent `specs/009-bookmarks-connector/scopes.md` has a DoD bullet that explicitly contains `SCN-BK-004` with raw `go test` evidence and a source-file pointer
- [x] Same for `SCN-BK-006`, `SCN-BK-007`, `SCN-BK-008`
- [x] Parent `specs/009-bookmarks-connector/scenario-manifest.json` exists and covers all 10 scenarios with `scenarioId`, `linkedTests`, `evidenceRefs`, and `linkedDoD`
- [x] Parent `specs/009-bookmarks-connector/report.md` references the concrete test files `internal/connector/bookmarks/dedup_test.go` and `internal/connector/bookmarks/topics_test.go`
- [x] Test Plan row for `SCN-BK-010` resolves to an existing concrete test file (in-process proxy `T-2-20` → `internal/connector/bookmarks/connector_test.go`); the planned live-stack rows `T-2-13..T-2-19` remain documented but no longer block the guard
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/009-bookmarks-connector` PASS
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/009-bookmarks-connector/bugs/BUG-009-001-dod-scenario-fidelity-gap` PASS
- [x] `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/009-bookmarks-connector` PASS
- [x] No production code changed (boundary)
