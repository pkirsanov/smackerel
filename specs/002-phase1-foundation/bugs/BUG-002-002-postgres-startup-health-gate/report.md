# Execution Report: BUG-002-002 Postgres Startup Health Gate

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Scope: Restore postgres readiness and persistence lifecycle evidence - 2026-04-27

### Summary

This packet classifies the canonical E2E blocker as Phase 1 Foundation bug work. No runtime source, Compose, CLI, test, or generated config files were changed by this packet.

### Completion Statement

The documentation packet is complete enough to route ownership. The bug itself remains `in_progress`; no implementation, test execution, validation certification, or fixed-state claim is made by this packet.

### Finding Classification

**Claim Source:** interpreted (provided by `bubbles.workflow` / test-owner verification)

```text
Command: ./smackerel.sh test e2e
Exit: 1
Failure ordering: before the Go E2E block could execute tests/e2e/capture_process_search_test.go
Scenario: SCN-002-004: Data persistence across restarts
Test file: tests/e2e/test_persistence.sh
Failure point: Inserting test artifact...
Error: service "postgres" is not running
Consequence: BUG-031-003 cannot receive post-fix live-stack evidence; specs/039-recommendations-engine full-delivery remains blocked.
```

### Ownership Classification

**Claim Source:** interpreted (workspace artifact inspection)

```text
Owner feature: specs/002-phase1-foundation
Owner scope: Scope 1 Project Scaffold
Owner scenario: SCN-002-004 Data persistence across restarts
Owner test: tests/e2e/test_persistence.sh
Related downstream blocked bug: specs/031-live-stack-testing/bugs/BUG-031-003-capture-processing-timeout
Related blocked continuation: specs/039-recommendations-engine
```

### Workspace Inspection Evidence

**Claim Source:** interpreted (read-only workspace inspection by this packet)

```text
docker-compose.yml postgres healthcheck currently uses:
  test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}"]

smackerel.sh up currently uses:
  smackerel_compose "$TARGET_ENV" up -d

tests/e2e/lib/helpers.sh::e2e_wait_healthy currently accepts:
  curl -sf --max-time 3 "$CORE_URL/api/health"

tests/e2e/run_all.sh currently has an inline curl-only Phase 1 wait:
  curl -sf --max-time 3 "$CORE_URL/api/health"

tests/e2e/test_persistence.sh currently uses fixed waits before postgres mutation:
  sleep 20
  Inserting test artifact...
  docker compose exec --interactive=false -T postgres psql ... INSERT INTO artifacts ...
  sleep 20 after restart before verifying persistence
```

### Prior Diagnostic Lead

**Claim Source:** interpreted (prior repository artifact, not treated as current proof)

```text
specs/038-cloud-drives-integration/report.md Round 9 recorded a previous postgres cold-start readiness flake with three contributing surfaces:
1. ./smackerel.sh up invoked docker compose up -d without --wait.
2. postgres healthcheck used pg_isready without TCP host/port and could pass during initdb unix-socket transition.
3. e2e_wait_healthy accepted any /api/health 2xx response, including degraded health.

The same prior artifact recorded a minimum viable fix:
1. smackerel.sh up passes --wait --wait-timeout 180.
2. docker-compose.yml postgres healthcheck forces TCP pg_isready and adds startup tolerance.
3. tests/e2e/lib/helpers.sh requires both API health and psql SELECT 1.
4. tests/e2e/run_all.sh delegates Phase 1 boot readiness to the hardened helper.

A later specs/038-cloud-drives-integration state entry says those fixes appeared un-applied during cross-cutting churn. This packet uses that only as a lead.
```

### Test Evidence

**Claim Source:** not-run

```text
No tests were executed by bubbles.bug for this documentation-only packet.
The implementation owner must capture pre-fix red evidence before code changes, then post-fix green evidence after the fix.
Required red evidence: SCN-002-004 or a focused lifecycle canary fails before the fix when postgres is not running, unhealthy, or unable to complete SELECT 1.
Required adversarial evidence: the readiness gate rejects stopped/unhealthy postgres and does not silently pass.
Required green evidence: SCN-002-004 inserts a unique artifact, restarts without deleting the test postgres volume, and reads count=1 after restart.
Required broader evidence: ./smackerel.sh test e2e no longer aborts at SCN-002-004 with service "postgres" is not running.
```

### Routing Contract

**Claim Source:** interpreted

```text
Recommended owner: bubbles.devops
Reason: the fix touches shared live-stack lifecycle, Docker Compose health, repo CLI startup semantics, and E2E harness readiness.
Owner spec: specs/002-phase1-foundation
Scenario refs: SCN-002-004, SCN-002-BUG-002-001, SCN-002-BUG-002-002, SCN-002-BUG-002-003, SCN-002-BUG-002-004
Required boundaries: protect dev persistent volumes, use disposable test storage, do not edit config/generated, keep runtime operations under ./smackerel.sh, avoid hardcoded fallback config.
```

## DevOps Execution Evidence - 2026-04-27

### Implementation Summary

The original documentation-only report section remains as historical routing evidence. This section records the subsequent `bubbles.devops` implementation and validation pass for the live-stack lifecycle/readiness contract.

**Phase:** devops
**Claim Source:** interpreted

```text
Implemented readiness repair within the declared bug boundary:
- config/smackerel.yaml: added SST-managed runtime.compose_wait_timeout_s: 180.
- scripts/commands/config.sh: required and emitted COMPOSE_WAIT_TIMEOUT_S from SST.
- docker-compose.yml: postgres healthcheck now requires TCP pg_isready plus psql SELECT 1.
- smackerel.sh: up now regenerates config, requires COMPOSE_WAIT_TIMEOUT_S, and runs compose up -d --wait --wait-timeout.
- smackerel.sh: canonical E2E lifecycle block now runs tests/e2e/test_postgres_readiness_gate.sh.
- tests/e2e/lib/helpers.sh: e2e_wait_healthy now requires authenticated /api/health, required service statuses for postgres/nats/ml_sidecar, and direct postgres SELECT 1.
- tests/e2e/run_all.sh: shared-stack readiness now delegates to e2e_wait_healthy.
- tests/e2e/test_persistence.sh: fixed sleeps removed; insert and restart verification wait on the hardened helper and assert count=1.
- tests/e2e/test_postgres_readiness_gate.sh: added adversarial stopped-postgres canary.

No config/generated files were edited by hand. Test cleanup used the disposable test stack with --env test; no broad Docker prune was run.
```

### Static, Build, Unit, And Lint Evidence

**Phase:** devops
**Claim Source:** executed

```text
Command: timeout 60 ./smackerel.sh config generate
Exit Code: 0
Observed: command completed successfully; no terminal output was emitted.

Command: timeout 60 ./smackerel.sh --env test config generate
Exit Code: 0
Observed: command completed successfully; no terminal output was emitted.

Command: timeout 120 ./smackerel.sh check
Exit Code: 0
Observed: command completed successfully.

Command: timeout 600 ./smackerel.sh format --check
Exit Code: 0
Observed: formatter completed successfully; Python formatting ended with 41 files left unchanged.

Command: timeout 1200 ./smackerel.sh build
Exit Code: 0
Observed: runtime images built successfully through the repo CLI.

Command: timeout 600 ./smackerel.sh test unit
Exit Code: 0
Observed: Go unit packages passed; Python suite reported 345 passed, 1 warning.

Command: timeout 600 ./smackerel.sh lint
Exit Code: 0
Observed: lint completed successfully.
```

### Regression Quality Guard Evidence

**Phase:** devops
**Command:** `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/lib/helpers.sh tests/e2e/run_all.sh tests/e2e/test_persistence.sh tests/e2e/test_postgres_readiness_gate.sh`
**Exit Code:** 0
**Claim Source:** executed

```text
============================================================
  BUBBLES REGRESSION QUALITY GUARD
  Repo: /home/philipk/smackerel
  Timestamp: 2026-04-27T18:43:31Z
  Bugfix mode: true
============================================================

ℹ️  Scanning tests/e2e/lib/helpers.sh
✅ Adversarial signal detected in tests/e2e/lib/helpers.sh
ℹ️  Scanning tests/e2e/run_all.sh
ℹ️  Scanning tests/e2e/test_persistence.sh
ℹ️  Scanning tests/e2e/test_postgres_readiness_gate.sh

============================================================
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 4
  Files with adversarial signals: 1
============================================================
```

### Focused Adversarial Readiness Evidence

**Phase:** devops
**Command:** `timeout 300 bash tests/e2e/test_postgres_readiness_gate.sh`
**Exit Code:** 0
**Claim Source:** executed

```text
=== SCN-002-BUG-002-001: Readiness gate rejects stopped postgres ===
[+] Running 7/7
 ✔ Network smackerel-test_default             Created                      0.7s
 ✔ Volume "smackerel-test-nats-data"          Created                      0.0s
 ✔ Volume "smackerel-test-postgres-data"      Created                      0.0s
 ✔ Container smackerel-test-nats-1            Healthy                     12.0s
 ✔ Container smackerel-test-postgres-1        Healthy                     12.0s
 ✔ Container smackerel-test-smackerel-ml-1    Healthy                     23.3s
 ✔ Container smackerel-test-smackerel-core-1  Healthy                     21.8s
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
Stopping postgres to force a readiness failure...
[+] Stopping 1/1
 ✔ Container smackerel-test-postgres-1  Stopped                            1.3s
Waiting for services to be healthy (max 8s)...
FAIL: Services did not become healthy within 8s
Last API health readiness error: service 'postgres' status is 'down', expected 'up'; payload={"status":"degraded",..."postgres":{"status":"down"}...}
Last postgres readiness error: service "postgres" is not running
PASS: SCN-002-BUG-002-001 (stopped postgres rejected, exit=1)
Cleaning up test stack...
```

### Focused Persistence Evidence

**Phase:** devops
**Command:** `timeout 300 bash tests/e2e/test_persistence.sh`
**Exit Code:** 0
**Claim Source:** executed

```text
=== SCN-002-004: Data persistence across restarts ===
Cleaning up test stack...
[+] Running 5/5
 ✔ Network smackerel-test_default             Created                      0.8s
 ✔ Container smackerel-test-nats-1            Healthy                     13.5s
 ✔ Container smackerel-test-postgres-1        Healthy                     13.5s
 ✔ Container smackerel-test-smackerel-ml-1    Healthy                     24.3s
 ✔ Container smackerel-test-smackerel-core-1  Healthy                     23.3s
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
Inserting test artifact...
Insert completed (INSERT01)
Insert verified (count=1)
Stopping services (preserving volumes)...
[+] Running 5/5
 ✔ Container smackerel-test-smackerel-core-1  Removed                      7.2s
 ✔ Container smackerel-test-smackerel-ml-1    Removed                     31.2s
 ✔ Container smackerel-test-postgres-1        Removed                      1.1s
 ✔ Container smackerel-test-nats-1            Removed                      1.8s
 ✔ Network smackerel-test_default             Removed                      0.8s
Restarting services...
[+] Running 5/5
 ✔ Network smackerel-test_default             Created                      0.6s
 ✔ Container smackerel-test-nats-1            Healthy                     10.6s
 ✔ Container smackerel-test-postgres-1        Healthy                     10.6s
 ✔ Container smackerel-test-smackerel-ml-1    Healthy                     15.0s
 ✔ Container smackerel-test-smackerel-core-1  Healthy                     15.5s
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
PASS: SCN-002-004 (data persisted, count=1)
Cleaning up test stack...
```

### Canonical E2E Evidence

**Phase:** devops
**Command:** `timeout 1800 ./smackerel.sh test e2e`
**Exit Code:** 124
**Claim Source:** interpreted
**Interpretation:** The canonical command did not complete, so no full-suite pass is claimed. It did run through the original `SCN-002-004` blocker, produced `PASS: SCN-002-004`, ran the stopped-postgres canary, and reached/passed the connector framework E2E section. The shell exit code `124` proves the outer timeout expired before the Go E2E block evidence appeared in the observed output.

```text
=== SCN-002-004: Data persistence across restarts ===
Cleaning up test stack...
[+] Running 5/5
 ✔ Network smackerel-test_default             Created                      0.6s
 ✔ Container smackerel-test-postgres-1        Healthy                     11.0s
 ✔ Container smackerel-test-nats-1            Healthy                     11.0s
 ✔ Container smackerel-test-smackerel-ml-1    Healthy                     15.3s
 ✔ Container smackerel-test-smackerel-core-1  Healthy                     15.8s
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
Inserting test artifact...
Insert completed (INSERT00)
Insert verified (count=1)
Stopping services (preserving volumes)...
Restarting services...
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
PASS: SCN-002-004 (data persisted, count=1)
Cleaning up test stack...
=== SCN-002-BUG-002-001: Readiness gate rejects stopped postgres ===
Stopping postgres to force a readiness failure...
FAIL: Services did not become healthy within 8s
Last API health readiness error: service 'postgres' status is 'down', expected 'up'; payload={"status":"degraded",..."postgres":{"status":"down"}...}
Last postgres readiness error: service "postgres" is not running
PASS: SCN-002-BUG-002-001 (stopped postgres rejected, exit=1)
...
=== Connector Framework E2E Tests ===
Test: sync_state table exists...
PASS: SCN-003-003: sync_state table exists
Test: Sync state CRUD...
PASS: SCN-001-013: Sync state round-trip verified
PASS: SCN-003-002: Cursor-based incremental sync state works
Test: Health endpoint shows service statuses...
  NATS status: up
PASS: SCN-001-020: Health reports NATS status correctly

=== Connector Framework E2E tests passed ===
echo $?
124
```

### Canonical E2E Uncertainty Declaration

**Phase:** devops
**Claim Source:** not-run

```text
What was attempted: timeout 1800 ./smackerel.sh test e2e
What was observed: the canonical command cleared SCN-002-004, cleared the stopped-postgres canary, reached connector framework, and then the outer timeout returned exit 124.
Why this is uncertain: no observed output showed the Go E2E block containing tests/e2e/capture_process_search_test.go, so Go-block reachability cannot be claimed from this run.
What would resolve this: execute the canonical E2E command with sufficient wall-clock budget, or use an approved repo-CLI invocation that reaches the Go E2E block and records current-session output.
```

### Cleanup Evidence

**Phase:** devops
**Command:** `timeout 60 ./smackerel.sh --env test down --volumes`
**Exit Code:** 0
**Claim Source:** executed

```text
Command completed successfully after the timed-out canonical run.
```

**Phase:** devops
**Command:** `docker ps -a`
**Exit Code:** 0
**Claim Source:** interpreted
**Interpretation:** The observed container listing contained no `smackerel-test-*` containers after `./smackerel.sh --env test down --volumes`; other non-Smackerel containers were present and were left untouched.

```text
CONTAINER ID   IMAGE                                                 COMMAND                  CREATED          STATUS                    PORTS        NAMES
8c6c3372b941   postgres:15-alpine                                    "docker-entrypoint.s..."   50 seconds ago   Up 50 seconds (healthy)   ...          guesthost-test-postgres-test
c76a20f74b7e   wanderaide-auth-service:latest                        "./service"              3 minutes ago    Up 3 minutes (healthy)    ...          wanderaide-services-auth-service
...
No NAMES entries beginning with smackerel-test- were present in the observed docker ps -a output.
```

### DevOps Completion Statement

**Claim Source:** interpreted

```text
Root cause addressed at the shared lifecycle/test-harness layer: startup now waits on Compose health, postgres health proves TCP/query readiness, and E2E readiness requires authenticated health plus direct postgres SELECT 1 before persistence can proceed.

SCN-002-004 status: fixed in focused execution and passed inside the canonical E2E output before the command timed out.

Canonical suite status: not a full pass. The command timed out with exit 124 after connector framework; Go E2E block reachability is not proven by this evidence.
```

## DevOps Follow-up Evidence - 2026-04-28

### Summary

No runtime, Compose, harness, or generated config files were changed in this follow-up. Current focused and canonical evidence shows the previously reported BUG-002-002 failures are not reproducing: `test_persistence.sh` passes, the stopped-Postgres readiness canary passes, and the broad command reaches the Go E2E block. The broad suite still exits non-zero due to failures outside the Postgres startup health gate.

### Focused Persistence Proof

**Phase:** devops
**Command:** `timeout 360 bash tests/e2e/test_persistence.sh`
**Exit Code:** 0
**Claim Source:** executed

```text
=== SCN-002-004: Data persistence across restarts ===
Preparing disposable test stack...
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
Inserting test artifact...
Insert completed (INSERT01)
Insert verified (count=1)
Stopping services (preserving volumes)...
Restarting services...
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
PASS: SCN-002-004 (data persisted, count=1)
Cleaning up test stack...
FOCUSED_PERSISTENCE_EXIT=0
```

### Focused Adversarial Readiness Proof

**Phase:** devops
**Command:** `timeout 360 bash tests/e2e/test_postgres_readiness_gate.sh`
**Exit Code:** 0
**Claim Source:** executed

```text
=== SCN-002-BUG-002-001: Readiness gate rejects stopped postgres ===
Preparing disposable test stack...
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
Stopping postgres to force a readiness failure...
Waiting for services to be healthy (max 8s)...
FAIL: Services did not become healthy within 8s
Last API health readiness error: service 'postgres' status is 'down', expected 'up'; payload={"status":"degraded",..."postgres":{"status":"down"}...}
Last postgres readiness error: service "postgres" is not running
PASS: SCN-002-BUG-002-001 (stopped postgres rejected, exit=1)
Cleaning up test stack...
FOCUSED_READINESS_EXIT=0
```

### Repo Check Evidence

**Phase:** devops
**Command:** `timeout 120 ./smackerel.sh check`
**Exit Code:** 0
**Claim Source:** executed

```text
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 0, rejected: 0
scenario-lint: OK
CHECK_EXIT=0
```

### Canonical Broad E2E Evidence

**Phase:** devops
**Command:** `timeout 3600 ./smackerel.sh test e2e`
**Exit Code:** 1
**Claim Source:** executed

```text
Running isolated lifecycle shell E2E: test_persistence.sh
=== SCN-002-004: Data persistence across restarts ===
Insert completed (INSERT01)
Insert verified (count=1)
PASS: SCN-002-004 (data persisted, count=1)

Running isolated lifecycle shell E2E: test_postgres_readiness_gate.sh
=== SCN-002-BUG-002-001: Readiness gate rejects stopped postgres ===
FAIL: Services did not become healthy within 8s
Last API health readiness error: service 'postgres' status is 'down', expected 'up'; payload={"status":"degraded",..."postgres":{"status":"down"}...}
Last postgres readiness error: service "postgres" is not running
PASS: SCN-002-BUG-002-001 (stopped postgres rejected, exit=1)

Shell E2E Test Results:
  Total:  34
  Passed: 32
  Failed: 2
  FAIL: test_digest_telegram.sh (exit=1)
  FAIL: test_topic_lifecycle.sh (exit=1)

Go E2E reached tests/e2e/... and passed TestE2E_CaptureProcessSearch_AdversarialEmptyStatus.
Go E2E failures observed:
  FAIL: TestE2E_DomainExtraction (domain extraction not completed within 90s timeout; last domain_status=)
  FAIL: TestOperatorStatus_RecommendationProvidersEmptyByDefault (status page missing Recommendation Providers block)

FAIL: go-e2e (exit=1)
BROAD_E2E_EXIT=1
```

### Cleanup Evidence

**Phase:** devops
**Command:** `timeout 180 ./smackerel.sh --env test down --volumes`
**Exit Code:** 0
**Claim Source:** executed

```text
CLEANUP_EXIT=0
```

### DevOps Follow-up Completion Statement

**Phase:** devops
**Claim Source:** interpreted

```text
BUG-002-002 readiness/persistence blocker status from this follow-up: no current DevOps-owned blocker reproduced. The focused persistence script and stopped-Postgres canary passed, and the canonical broad command reached the Go E2E block instead of aborting at SCN-002-004 or SCN-002-BUG-002-001.

Remaining broad failures are outside this Postgres startup health-gate packet:
- test_digest_telegram.sh: SCN-002-032 digest delivery not tracked.
- test_topic_lifecycle.sh: duplicate key value violates unique constraint topics_name_key for name=pricing.
- TestE2E_DomainExtraction: domain extraction did not complete within 90s.
- TestOperatorStatus_RecommendationProvidersEmptyByDefault: status page missing Recommendation Providers block.
```
