# Bug Fix Design: BUG-025-006 Knowledge lint zero-scale pass

## Root Cause Analysis

### Investigation Summary

The dirty test file was inspected without execution. The target function now connects to disposable PostgreSQL and NATS, constructs production lint, runs it, and validates the latest endpoint report.

The function does not insert artifacts before lint. It does not count artifacts before lint.

`internal/knowledge/lint.go` shows that an empty artifact set is not itself an error. The linter can complete its checks and store a valid empty report when dependencies are healthy.

### Root Cause

The scale claim exists only in the test name and comments. No executable precondition binds `1000-artifact scale` to database state.

The dirty changes fix a separate false-green mechanism by replacing skips with failures. They do not establish the workload cardinality.

### Impact Analysis

- Affected component: `TestKnowledge_LintAt1000ArtifactScale`.
- Affected assurance: parent `R-2506` five-minute lint budget at 1000 artifacts.
- Affected data: disposable test rows only.
- Runtime impact: unknown because the declared scale has not been exercised by this test.
- User impact: no direct runtime defect is established. Release confidence is overstated.

## Fix Design

### Ownership Model

The `artifacts` table has no user-owner column. Use the indexed `source_id` column as the test-run ownership key.

Generate a fresh run token with the repository's existing unique-ID facility. Prefix it with `test-b025006-knowledge-lint-`.

Use that token as `source_id`. Include the token and an ordinal in each artifact ID and content hash.

### Seed Operation

Add one focused helper in `tests/stress/knowledge_stress_test.go`. It must insert exactly 1000 rows in a transaction or one `INSERT ... SELECT generate_series` statement.

Every row must set these values explicitly:

- unique `id`
- `artifact_type = note`
- synthetic title and content
- unique `content_hash`
- shared run-owned `source_id`
- `processing_status = completed`
- `synthesis_status = completed`
- `synthesis_at`, `created_at`, and `updated_at`

Completed synthesis status avoids unrelated retry publication. The production linter still runs all six checks and stores the real report.

### Cardinality Gate

Add a helper that returns the owned row count, distinct ID count, and distinct content-hash count.

The primary test must require all three counts to equal 1000 before creating the production linter. This ordering prevents a zero-scale pass.

The error must report expected and actual counts without printing artifact content.

### Lint Result Gate

Retain the production `KnowledgeStore`, `Linter`, PostgreSQL pool, NATS client, and `RunLint` call.

Retain existing endpoint status, report ID, report freshness, and duration assertions. Capture the returned report ID for cleanup.

Assert both timing clocks:

- measured `RunLint` wall duration is at most five minutes
- persisted report `duration_ms` is between zero and five minutes inclusive

### Cleanup Gate

Register cleanup before inserting any row. This guarantees partial seed failures still enter cleanup.

Cleanup must use bounded contexts. It must delete artifacts by exact `source_id` and the report by exact report ID.

Cleanup must then query both owned row sets. A delete error, count error, or nonzero count must call `t.Errorf` and fail the test.

Do not log and ignore cleanup failures. Do not truncate tables.

The stress stack remains disposable. Row cleanup proves ownership discipline inside that disposable environment and does not replace stack teardown.

### Fail-Loud Preservation

Do not alter the dirty replacements of `t.Skip` and `t.Skipf`. Missing configuration, transport failures, unavailable knowledge endpoints, and unexpected statuses remain direct failures.

### No Production Substitutes

Do not add a fake linter, mock store, canned report, result cache, production fixture, default, or fallback.

Synthetic artifacts remain inside the stress test. Production source and configuration remain unchanged.

## Exact Change Boundary

### Allowed During Implementation

- `tests/stress/knowledge_stress_test.go`
- `specs/025-knowledge-synthesis-layer/bugs/BUG-025-006-knowledge-lint-zero-scale/**`

### Excluded

- `internal/knowledge/**`
- `internal/api/**`
- `internal/db/migrations/**`
- `config/**`
- `config/generated/**`
- `docker-compose*.yml`
- `deploy/**`
- every other test file
- parent spec 025 certification fields

## Regression Test Design

### Red Stage

Add the exact cardinality query and assertion before adding the seed helper. Run `./smackerel.sh test stress` on the disposable stack.

The target test must fail before `RunLint` with expected count 1000 and actual owned count zero. Record the real failure output.

### Green Stage

Add the atomic seed. Run the same command.

The target test must log the proven owned cardinality, execute production lint, validate the fresh report, satisfy both duration checks, and leave zero owned residue.

### Adversarial Regression

Add `TestKnowledge_LintScaleCardinalityGuardRejectsZeroAndDrift` under the stress build tag.

The test must exercise zero, 999, and 1001 observed counts against an expected count of 1000. Each case must return a cardinality error.

The exact 1000 case must pass the guard. This catches removal, weakening to nonzero, and off-by-one regressions.

The primary live test remains the proof that database counts drive the guard before production lint.

### Cleanup Regression

The primary test must assert zero owned artifacts and zero owned report rows from cleanup. A residue assertion is mandatory evidence for completion.

### Broader Regression

Run the complete repository stress lane. Confirm the existing concept-query, search, health, and other knowledge stress cases remain fail loud and green.

## Alternative Approaches Considered

1. Count all artifacts in the database. Rejected because unrelated stress fixtures could make the count ambiguous.
2. Add an `owner_id` column. Rejected because this is a test-integrity defect and the indexed `source_id` already supports exact ownership.
3. Mock production lint. Rejected because it would not prove the live lint path or database timing.
4. Seed pending artifacts. Rejected because retry publication would mix backlog throughput with the lint-scale contract.
5. Accept any positive count. Rejected because the contract requires exactly 1000 artifacts.
6. Rely only on disposable stack teardown. Rejected because teardown cannot prove test-owned cleanup behavior.

## Complexity Tracking

None - the simplest viable fix is one owned seed, one cardinality gate, production lint, and fail-loud cleanup in the existing stress file.