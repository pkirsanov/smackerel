# Execution Report: BUG-025-001 Knowledge stats empty-store 500

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Return valid stats for an empty knowledge store - 2026-04-27

### Summary
- Bug packet created by `bubbles.bug` during 039 e2e blocker packetization.
- No production code, test code, parent 025 artifacts, or 039 certification fields were modified by this packetization pass.
- This packet separates the empty-store stats failure from the external URL E2E failure so each root cause can be fixed and tested independently.

### Completion Statement
BUG-025-001 is fixed, verified, and closed. The empty-store stats query no longer scans NULL into a string, the focused E2E stats endpoint regression passes, the adversarial live PostgreSQL regression covers the no-`knowledge_concepts` case, and the later c6d2b26 broad E2E baseline is green.

### Evidence Provenance
The workflow supplied the failing e2e signature. Source inspection through IDE tools found a likely empty-store NULL scan path in `internal/knowledge/store.go::GetStats`. Later implementation evidence captured the runtime reproduction and red-stage output.

### Bug Reproduction - Before Fix
No terminal command was executed in the packetization pass. The targeted red output was later captured in the implementation evidence below.

Packetization notes recorded the workflow observation `Knowledge stats returns 500 on empty store` and the source-inspection hypothesis that `internal/knowledge/store.go::GetStats` could scan NULL from an empty `knowledge_concepts` scalar subquery into `PromptContractVersion`.

### Test Evidence
No tests were run by `bubbles.bug` for this packet. Red-stage and green-stage evidence is recorded in the implementation and validation sections below.

### Change Boundary
Allowed implementation surfaces:
- `internal/knowledge/store.go`
- `internal/knowledge/store_test.go`
- Focused integration/E2E test additions for empty-store stats

Protected surfaces for this bug:
- Knowledge synthesis prompt behavior
- External URL capture/extraction behavior, which is tracked separately in `BUG-025-002-knowledge-e2e-external-url`
- Recommendation engine feature 039 artifacts and certification fields

## Implementation Evidence - 2026-04-28

### Root Cause
`internal/knowledge/store.go::GetStats` selected the latest prompt contract version with an inner `COALESCE` inside a scalar subquery. When `knowledge_concepts` had no rows, the scalar subquery still produced NULL, and pgx could not scan that NULL into `KnowledgeStats.PromptContractVersion string`. That store error propagated through `internal/api/knowledge.go::KnowledgeStatsHandler` as HTTP 500.

The fix moves the empty-result handling to the outer expression: `COALESCE((SELECT prompt_contract_version FROM knowledge_concepts ORDER BY updated_at DESC LIMIT 1), '')`.

The lint-report branch was also narrowed so only `pgx.ErrNoRows` means "no lint stats yet"; other DB errors still return an error.

### Red Proof
**Phase:** implement  
**Command:** `timeout 3600 ./smackerel.sh test e2e --go-run TestKnowledgeStore_TablesExist`  
**Exit Code:** 1  
**Claim Source:** executed

```text
=== RUN   TestKnowledgeStore_TablesExist
knowledge_store_test.go: expected 200, got 500: {"error":{"code":"INTERNAL_ERROR","message":"Failed to get knowledge stats"}}
--- FAIL: TestKnowledgeStore_TablesExist
FAIL
Exit Code: 1
```

### Changes
| File | Change |
|---|---|
| `internal/knowledge/store.go` | Outer `COALESCE` for empty prompt-contract scalar subquery; only `pgx.ErrNoRows` is tolerated for missing lint reports. |
| `tests/integration/knowledge_stats_test.go` | New live PostgreSQL regression for an empty knowledge store, including no `knowledge_concepts` rows. |
| `tests/e2e/knowledge_store_test.go` | Broad E2E keeps the HTTP 200 assertion and now verifies the stats endpoint response contract without assuming global empty-store state after previous broad-suite scenarios. Required numeric fields must be present and non-negative, `last_synthesis_at` must be present as `null` or an RFC3339 timestamp, and `prompt_contract_version` must be present and non-null. |

### Targeted Green Proof
**Phase:** implement  
**Claim Source:** executed

```text
$ timeout 600 ./smackerel.sh test unit --go
ok      github.com/smackerel/smackerel/internal/knowledge 0.011s
Exit Code: 0
```

```text
$ timeout 1200 ./smackerel.sh test integration
=== RUN   TestKnowledgeStats_EmptyStoreReturnsZeroValues
--- PASS: TestKnowledgeStats_EmptyStoreReturnsZeroValues (0.55s)
```

The integration suite exited 1 because of unrelated existing failures outside this bug scope:

```text
--- FAIL: TestNATS_PublishSubscribe_Artifacts
create consumer: nats: API error: code=400 err_code=10100 description=filtered consumer not unique on workqueue stream
--- FAIL: TestNATS_PublishSubscribe_Domain
create consumer: nats: API error: code=400 err_code=10100 description=filtered consumer not unique on workqueue stream
--- FAIL: TestNATS_Chaos_MaxDeliverExhaustion
expected 0 messages after MaxDeliver exhaustion, got 1
--- FAIL: TestDriveConnectorsEndpoint_LiveStackReturnsNeutralProviderList
status = 404 (want 200); body=404 page not found
Exit Code: 1
```

```text
$ timeout 3600 ./smackerel.sh test e2e --go-run TestKnowledgeStore_TablesExist
go-e2e: applying -run selector: TestKnowledgeStore_TablesExist
=== RUN   TestKnowledgeStore_TablesExist
	knowledge_store_test.go:43: knowledge stats: concepts=0 entities=0 synthesized=0 pending=0 contract=
--- PASS: TestKnowledgeStore_TablesExist (0.06s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.069s
Exit Code: 0
```

### Repo Checks
**Phase:** implement  
**Claim Source:** executed

```text
$ timeout 300 ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 0, rejected: 0
scenario-lint: OK
Exit Code: 0
```

```text
$ timeout 600 ./smackerel.sh format --check
42 files left unchanged
Exit Code: 0
```

### Broad E2E
**Phase:** implement  
**Command:** `timeout 3600 ./smackerel.sh test e2e`  
**Exit Code:** 1  
**Claim Source:** executed

The broad E2E run reached the Go E2E package after the shell scenarios had already exercised state-mutating graph, search, capture, import, and connector flows. The repaired `TestKnowledgeStore_TablesExist` passed in that broad order with seeded knowledge state (`edges=3`, `failed=2`), proving the broad test no longer depends on a globally empty store while still asserting HTTP 200 and a valid stats response shape. The command exited 1 from sibling E2E failures outside BUG-025-001; the later c6d2b26 baseline below cleared the broad-suite gate.

```text
=== RUN   TestKnowledgeStore_TablesExist
	knowledge_store_test.go:77: knowledge stats: concepts=0 entities=0 edges=3 completed=0 pending=0 failed=2 contract=
--- PASS: TestKnowledgeStore_TablesExist (0.05s)
--- FAIL: TestE2E_DomainExtraction (90.29s)
	domain_e2e_test.go:121: domain extraction not completed within 90s timeout - last domain_status= (pipeline or ML sidecar may not support domain extraction)
--- FAIL: TestKnowledgeSynthesis_PipelineRoundTrip (0.30s)
	knowledge_synthesis_test.go:38: capture returned 422: {"error":{"code":"EXTRACTION_FAILED","message":"content extraction failed: HTTP 404 fetching https://example.com/synthesis-e2e-test"}}
--- FAIL: TestOperatorStatus_RecommendationProvidersEmptyByDefault (0.05s)
	operator_status_test.go:28: status page missing Recommendation Providers block
FAIL    github.com/smackerel/smackerel/tests/e2e        168.493s
Exit Code: 1
```

## DevOps Port-Conflict Repair Evidence - 2026-04-28

### Root Cause
The original failing listener on `127.0.0.1:45002` was no longer present after project-scoped cleanup, so no long-lived non-Smackerel host process could be proven as the source. The reproducible lifecycle issue was in the test-stack harness surface: `smackerel.sh` did not run a test-project cleanup before `up` attempted fixed host-port binds, collision reporting was left to Docker's low-level bind error, and the top-level parser ignored post-command global flags such as `down --volumes`, so documented cleanup forms could silently preserve disposable state.

### Changes
| File | Change |
|---|---|
| `smackerel.sh` | Parse global flags before or after the command token so `./smackerel.sh --env test down --volumes` removes disposable test volumes. |
| `smackerel.sh` | Before `--env test up`, run project-scoped `docker compose down --remove-orphans` through `smackerel_compose`, then preflight configured host ports from generated env before Compose publishes them. |
| `smackerel.sh` | Report unavailable fixed test ports with the config key, bind address, port, and OS error, e.g. `ML_HOST_PORT=45002 on 127.0.0.1:45002`. |

No generated config was edited. No broad Docker prune was used. Dev persistent volumes were not removed; only disposable `smackerel-test-*` volumes were removed through explicit `--env test down --volumes` cleanup.

### Verification Evidence
**Phase:** devops  
**Claim Source:** executed

```text
$ timeout 120 ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 0, rejected: 0
scenario-lint: OK
Exit Code: 0
```

```text
$ timeout 300 ./smackerel.sh --env test down --volumes
[+] Running 2/2
 ✔ Volume smackerel-test-nats-data      Removed
 ✔ Volume smackerel-test-postgres-data  Removed
Exit Code: 0
```

```text
$ timeout 360 ./smackerel.sh --env test up
Preparing disposable test stack...
[+] Running 7/7
 ✔ Network smackerel-test_default             Created
 ✔ Volume "smackerel-test-postgres-data"      Created
 ✔ Volume "smackerel-test-nats-data"          Created
 ✔ Container smackerel-test-postgres-1        Healthy
 ✔ Container smackerel-test-nats-1            Healthy
 ✔ Container smackerel-test-smackerel-ml-1    Healthy
 ✔ Container smackerel-test-smackerel-core-1  Healthy
Exit Code: 0
```

```text
$ timeout 360 ./smackerel.sh --env test down --volumes
[+] Running 7/7
 ✔ Container smackerel-test-smackerel-ml-1    Removed
 ✔ Container smackerel-test-smackerel-core-1  Removed
 ✔ Container smackerel-test-postgres-1        Removed
 ✔ Container smackerel-test-nats-1            Removed
 ✔ Volume smackerel-test-nats-data            Removed
 ✔ Volume smackerel-test-postgres-data        Removed
 ✔ Network smackerel-test_default             Removed
Exit Code: 0
```

```text
$ timeout 1200 ./smackerel.sh test e2e --go-run TestKnowledgeStore_TablesExist
Preparing disposable test stack...
[+] Running 7/7
 ✔ Container smackerel-test-smackerel-core-1  Healthy
 ✔ Container smackerel-test-smackerel-ml-1    Healthy
go-e2e: applying -run selector: TestKnowledgeStore_TablesExist
=== RUN   TestKnowledgeStore_TablesExist
		knowledge_store_test.go:43: knowledge stats: concepts=0 entities=0 synthesized=0 pending=0 contract=
--- PASS: TestKnowledgeStore_TablesExist (0.05s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.063s
Exit Code: 0
```

### Collision Diagnostic Proof
**Phase:** devops  
**Claim Source:** executed

A short-lived local listener was bound to the generated ML test host port, then `test up` was run again. Startup failed before Compose bind attempts and named the colliding config key.

A local listener command printed `Serving HTTP on 127.0.0.1 port 45002`, which set up the collision used by the next `./smackerel.sh --env test up` proof.

```text
$ timeout 120 ./smackerel.sh --env test up
Preparing disposable test stack...
ERROR: Smackerel host port preflight failed after project-scoped cleanup.
Unavailable test port(s):
	- ML_HOST_PORT=45002 on 127.0.0.1:45002: [Errno 98] Address already in use
Stop the non-Smackerel listener or stale container using the port, then retry.
Exit Code: 1
```

After killing the short-lived listener, the final project-scoped cleanup and port check succeeded.

```text
$ timeout 360 ./smackerel.sh --env test down --volumes
Exit Code: 0

$ ss -ltnp 'sport = :45002'
State   Recv-Q   Send-Q     Local Address:Port     Peer Address:Port  Process
Exit Code: 0
```

### Validation Evidence - 2026-04-30
**Phase:** validate
**Phase Agent:** bubbles.validate
**Executed:** YES
**Command:** existing BUG-025-001 report evidence review plus c6d2b26 broad E2E baseline evidence from `specs/039-recommendations-engine/report.md`
**Exit Code:** c6d2b26 broad baseline 0; not rerun during metadata-only closeout
**Claim Source:** interpreted from existing executed evidence
**Interpretation:** The BUG-025-001 implementation evidence proves the fixed behavior directly: pre-fix focused E2E reproduced the empty-store stats HTTP 500, post-fix focused E2E passed with HTTP 200 and zero/default stats on a fresh disposable stack, and the live PostgreSQL regression asserts zero counts plus an explicit empty prompt contract version when `knowledge_concepts` has no rows. The implementation-stage broad E2E command showed `TestKnowledgeStore_TablesExist` passing before unrelated sibling failures. Feature 039 validation evidence later records the c6d2b26 baseline with `timeout 3600 ./smackerel.sh test e2e` exit 0, shell E2E 34/34 passed, and Go E2E packages passed. No broad E2E rerun was needed for this metadata-only closeout.

```text
BUG-025-001 focused green evidence:
$ timeout 3600 ./smackerel.sh test e2e --go-run TestKnowledgeStore_TablesExist
go-e2e: applying -run selector: TestKnowledgeStore_TablesExist
=== RUN   TestKnowledgeStore_TablesExist
	knowledge_store_test.go:43: knowledge stats: concepts=0 entities=0 synthesized=0 pending=0 contract=
--- PASS: TestKnowledgeStore_TablesExist (0.06s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.069s
Exit Code: 0

BUG-025-001 adversarial integration evidence:
=== RUN   TestKnowledgeStats_EmptyStoreReturnsZeroValues
--- PASS: TestKnowledgeStats_EmptyStoreReturnsZeroValues (0.55s)

c6d2b26 broad E2E baseline evidence from specs/039-recommendations-engine/report.md:
Command: timeout 3600 ./smackerel.sh test e2e
Exit Code: 0
Shell e2e phase: Total: 34, Passed: 34, Failed: 0
Go e2e packages passed.
```

### Audit Evidence - 2026-04-30
**Phase:** audit
**Phase Agent:** bubbles.validate
**Executed:** YES
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/025-knowledge-synthesis-layer/bugs/BUG-025-001-knowledge-stats-empty-store`
**Exit Code:** 0
**Claim Source:** executed
**Interpretation:** The canonical packet-level governance check passed after promotion to `done`, confirming the artifact set, checked DoD evidence formatting, repo-CLI command discipline, and anti-fabrication evidence checks are mechanically clean.

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/025-knowledge-synthesis-layer/bugs/BUG-025-001-knowledge-stats-empty-store
Detected state.json status: done
DoD completion gate passed for status 'done' (all DoD checkboxes are checked)
Top-level status matches certification.status
Required specialist phase 'test' found in execution/certification phase records
Phase-scope coherence verified (Gate G027)
No repo-CLI bypass detected in report.md command evidence
All 17 evidence blocks in report.md contain legitimate terminal output
Artifact lint PASSED.
```
