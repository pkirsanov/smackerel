# Bug: BUG-045-002 — Chronic CI integration job failure on `main` persists despite BUG-045-001 validator routing fix

## Classification

- **Type:** CI environment mismatch / test-environment-isolation defect — the `integration` job in `.github/workflows/ci.yml` runs `go test -tags=integration ./tests/integration/...` against a CI runner that has ONLY `postgres` (GH Actions service container) + `nats` (docker run) live, but the integration test suite contains tests that were authored against the local `./smackerel.sh test integration` Compose stack (which brings up `postgres` + `nats` + `ollama` + `smackerel-core` + `smackerel-ml` together). BUG-045-001 fixed a real validator-routing defect (`internal/config/config.go::validateMLModelEnvelope` → `validateModelEnvelopes`) that surfaced during the same investigation, but BUG-045-001's `report.md` § Evidence 7 explicitly recorded an Uncertainty Declaration: *"the per-job logs require `gh auth login` to fetch ... `bubbles.design` or `bubbles.implement` should re-verify the exact job-log error message against the local reproduction during the implement/test phase to confirm root-cause attribution."* That re-verification never happened. The CI integration job continues to fail on every `main` push (20/20 most recent runs as of 2026-05-16T22:30Z), confirming that the validator fix — while correct on its own merits — was NOT the actual root cause of the chronic CI failure.
- **Severity:** P1 — HIGH. The same three observable impacts BUG-045-001 cited continue to hold:
  1. **`main` CI integration is red on every push.** `https://github.com/pkirsanov/smackerel/actions/workflows/ci.yml?query=branch%3Amain` shows 20/20 consecutive `failure` runs on `main` between `e809ff9a` (2026-05-14T17:20Z) and `5c8d857e` (2026-05-16T22:30Z). The most recent failing run is `https://github.com/pkirsanov/smackerel/actions/runs/25974673514`; the failing job is `integration` (id `76352925791`); the failing step is `Fail job if integration tests failed` (which is triggered by `if: steps.itest_step.outcome == 'failure'` per `.github/workflows/ci.yml` line ~327 — meaning the preceding `Run integration tests` step DID exit non-zero, but `continue-on-error: true` suppresses the conclusion).
  2. **Downstream `build-bundles` and `publish-build-manifest` cannot certify a green pipeline at any HEAD on `main`.** Even when the `build` job succeeds, the operator cannot point at a single `main` HEAD whose full CI pipeline passed. This breaks the spec 047 / spec 052 Build-Once Deploy-Many invariant that a `build-manifest-<sourceSha>.yaml` should only be produced for HEADs whose full validation chain passed.
  3. **BUG-045-001's certification verdict (`passed-with-known-drift`) is materially weakened.** BUG-045-001 was certified `done` at HEAD `5c8d857e` on 2026-05-17T06:30Z on the basis that local `./smackerel.sh test integration` exit 0 demonstrates the validator fix worked. That claim is true and unchanged. But BUG-045-001's stated success criterion in `spec.md` AC-1 — *"chronic CI integration failure on `main` resolves to green"* — is observably FALSE at the certified HEAD. This bug packet (BUG-045-002) is the explicit corrective: BUG-045-001 fixed the validator; BUG-045-002 fixes the actual CI failure.
- **Parent Spec:** 045 — Deploy Resource and Filesystem Hardening (`specs/045-deploy-resource-filesystem-hardening/`). This is a scoped follow-on to BUG-045-001 covering the work BUG-045-001 declared out of scope under the Uncertainty Declaration.
- **Workflow Mode:** `bugfix-fastlane` (per parent workflow orchestrator handoff)
- **Status:** Open
- **Discovered By:** 2026-05-16 `bubbles.bug` Phase 1 RCA following the `./smackerel.sh push origin main` for HEAD `5c8d857e` (this session)
- **Discovery Date:** 2026-05-16

## Summary

The CI workflow `https://github.com/pkirsanov/smackerel/actions/runs/25974673514` for HEAD `5c8d857e80a07f59600f51b9e9bce906814a6311` failed on the `integration` job (id `76352925791`) at the `Fail job if integration tests failed` step. The preceding `Run integration tests` step is configured with `continue-on-error: true` and therefore reports `conclusion: success` in the API even though its `outcome` is `failure` — the actual `go test -tags=integration ./tests/integration/...` exit code was non-zero.

The chronic-failure history at `https://github.com/pkirsanov/smackerel/actions/workflows/ci.yml?query=branch%3Amain` shows 20/20 consecutive `failure` runs on `main` going back to `e809ff9a` (2026-05-14T17:20Z) — three weeks of unbroken CI red on `main`. BUG-045-001 was opened at HEAD `de49b2f9` (2026-05-16T13:00Z) precisely because of this chronic failure, and was certified done at HEAD `5c8d857e` on 2026-05-17T06:30Z on the basis of local `./smackerel.sh test integration` exit 0. The actual CI failure continues unchanged.

The most likely root cause class (HYPOTHESIS — see "Reproduction" for the proof-path; per the Uncertainty Declaration below the cause cannot be definitively confirmed without log access):

The CI integration job (`.github/workflows/ci.yml` lines ~225–333) ships only two backing services to the test environment:

| Service       | How CI provides it                  | How local provides it                |
|---------------|-------------------------------------|--------------------------------------|
| postgres      | GH Actions `services:` block        | Compose container `postgres`         |
| nats          | `docker run --network host nats` with auth + JetStream | Compose container `nats`             |
| **ollama**    | **NOT PROVIDED**                    | Compose container `ollama` (pinned image, port 11434) |
| **smackerel-core** | **NOT PROVIDED**               | Compose container `smackerel-core` (built locally) |
| **smackerel-ml**   | **NOT PROVIDED**               | Compose container `smackerel-ml` (built locally) |

The local runner (`./smackerel.sh test integration`, smackerel.sh:687-723) invokes `tests/integration/test_runtime_health.sh` with `KEEP_STACK_UP=1` before running Go tests — bringing up the FULL Compose stack via `./smackerel.sh --env test up` (including ollama, smackerel-core, smackerel-ml). The CI runner skips this entirely. Any integration test that issues an HTTP request to `http://localhost:11434` (ollama), `http://localhost:8080` (smackerel-core), or `http://localhost:8081` (smackerel-ml), or that runs `docker exec smackerel-test-ollama-1 …`, or that depends on env state that the live stack populates, will fail in CI but pass locally.

The fact that BUG-045-001's local-only fix (validator routing) closed exit 0 locally but did NOT change the CI failure state proves the CI failure is in this class of environment mismatch, not in the validator chain.

## Discovery Brief

The 2026-05-16 push of HEAD `5c8d857e` (containing the BUG-045-001 fix + a chore commit + a Bubbles framework refresh) triggered CI run `25974673514`. The user's local pre-push validation (`./smackerel.sh check`, `lint`, `format --check`, `test unit`, `test integration`, `up`, `status`, `down`) all returned exit 0. The CI integration job nevertheless reported `failure` with the same step-pass/step-fail pattern observed on every prior `main` HEAD going back to `e809ff9a`.

Cross-reference to BUG-045-001's `report.md` § Evidence 7 (lines 212-222) confirms BUG-045-001 was aware of the log-access constraint and explicitly noted: *"the discovery phase did not independently fetch and grep the CI job logs ... should re-verify the exact job-log error message against the local reproduction during the implement/test phase to confirm root-cause attribution"*. That re-verification was never performed. BUG-045-001 certified done on the basis of local-only evidence and the assumption (not the proof) that fixing the validator would fix the CI failure.

This bug packet (BUG-045-002) is the formal corrective: surfacing that BUG-045-001's `done` verdict was for a real but DIFFERENT bug than the one its spec claimed to fix, and creating the right packet for the actual chronic CI failure.

## Problem Statement

The CI workflow at HEAD `5c8d857e` continues to fail on every `main` push. The failure mode (verbatim from the GitHub jobs REST endpoint):

```text
GET /repos/pkirsanov/smackerel/actions/runs/25974673514/jobs
job id=76352925791  name=integration  conclusion=failure
  step "Set up job"                                  conclusion=success
  step "Initialize containers"                       conclusion=success
  step "Run actions/checkout@..."                    conclusion=success
  step "Run actions/setup-go@..."                    conclusion=success
  step "Start NATS with auth and JetStream"          conclusion=success
  step "Apply database migrations via db.Migrate"    conclusion=success
  step "Generate SST config files for integration"   conclusion=success
  step "Run integration tests"                       conclusion=success   (outcome=failure, masked by continue-on-error: true)
  step "Upload integration test log"                 conclusion=success
  step "Fail job if integration tests failed"        conclusion=failure   ← TRIGGERED BY outcome=failure ABOVE
  step "Post Run actions/setup-go@..."               conclusion=skipped
  step "Stop containers"                             conclusion=success
  step "Complete job"                                conclusion=success
```

The `Upload integration test log` step uploaded an artifact `integration-test-log` (id `7037283464`, 27003 bytes, expires 2026-05-23T22:37:49Z) which contains the verbatim `go test` output — but the GitHub REST `actions/artifacts/<id>/zip` endpoint requires `actions:read` permission (HTTP 401 to anonymous callers), and the per-job logs endpoint requires `admin` rights (HTTP 403 to anonymous callers per `https://docs.github.com/rest/actions/workflow-jobs#download-job-logs-for-a-workflow-run`). The exact failing test name and error text are therefore opaque to this RCA pass.

What IS observable without auth (and IS captured verbatim in `report.md`):

1. The `Run integration tests` step's command-line: `go test -tags=integration ./tests/integration/... -v -count=1 -timeout 10m 2>&1 | tee integration-test.log` (per `.github/workflows/ci.yml` lines ~308-316, verbatim).
2. The CI runner's backing-service topology: postgres (GH service container) + nats (docker run); NO ollama, NO smackerel-core, NO smackerel-ml.
3. The local runner's backing-service topology: full Compose stack including ollama / core / ml, brought up by `tests/integration/test_runtime_health.sh` before Go tests run (per `smackerel.sh:687-723`, verbatim).
4. The list of integration test files that reference ollama / live HTTP endpoints (per `grep_search` evidence in `report.md` § Evidence 5).
5. The chronic-failure history pattern: 20/20 consecutive `failure` runs on `main` between `e809ff9a` and `5c8d857e`.

## Detection

| Aspect | Detail |
|--------|--------|
| Trigger | 2026-05-16 push of HEAD `5c8d857e80a07f59600f51b9e9bce906814a6311` to `origin/main`; CI workflow run `25974673514` |
| Finding | `integration` job failure (id `76352925791`); chronic-failure pattern 20/20 most-recent `main` runs |
| Severity | P1 — HIGH (chronic `main` CI red; downstream pipeline cannot certify any HEAD) |
| Audit method | (a) `curl -s https://api.github.com/repos/pkirsanov/smackerel/actions/runs/25974673514/jobs` returned step-pass/step-fail JSON for job `76352925791` showing `Run integration tests` outcome=failure (masked by continue-on-error) and `Fail job if integration tests failed` triggering. (b) `curl -s https://api.github.com/repos/pkirsanov/smackerel/actions/workflows/ci.yml/runs?branch=main&per_page=20` returned 20/20 consecutive failure runs on main. (c) `read_file .github/workflows/ci.yml` confirmed the CI integration job ships only postgres + nats; no ollama / core / ml. (d) `read_file smackerel.sh` lines 687-723 confirmed local `./smackerel.sh test integration` brings up the full Compose stack via `tests/integration/test_runtime_health.sh`. (e) `read_file specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-001-ml-envelope-cross-service-routing/report.md` line 222 confirmed BUG-045-001's Uncertainty Declaration that root-cause attribution to CI was unverified. (f) `git diff --stat ad512fc6..5c8d857e -- ml/Dockerfile ml/requirements.txt ml/pyproject.toml` returned empty — the BUG-045-001 commit did not change anything CI-environment-relevant outside Go source + ml Python test files; the chronic CI failure pattern is therefore expected to be unchanged. |
| CI evidence URLs | Failing run: `https://github.com/pkirsanov/smackerel/actions/runs/25974673514` ; failing job: `https://github.com/pkirsanov/smackerel/actions/runs/25974673514/job/76352925791` ; chronic-history query: `https://github.com/pkirsanov/smackerel/actions/workflows/ci.yml?query=branch%3Amain` |

## Reproduction

```bash
# (1) Confirm the chronic CI failure pattern is real and unchanged on the current HEAD
curl -s "https://api.github.com/repos/pkirsanov/smackerel/actions/workflows/ci.yml/runs?branch=main&per_page=20" | python3 -m json.tool | python3 -c "
import json, sys
d = json.load(sys.stdin)
for r in d.get('workflow_runs', []):
    print(f\"{r['head_sha'][:8]}  {r['conclusion']:>10}  {r['created_at']}  {r['display_title'][:80]}\")
"
# Expected: 20/20 most recent rows show conclusion=failure on main between
# e809ff9a (2026-05-14) and 5c8d857e (2026-05-16). The pattern is unbroken.

# (2) Confirm the failing job + step on the current HEAD
curl -s "https://api.github.com/repos/pkirsanov/smackerel/actions/runs/25974673514/jobs" | python3 -m json.tool | grep -E '"(name|conclusion)"' | head -120
# Expected: `integration` job conclusion=failure;
# step "Run integration tests" conclusion=success (outcome=failure, masked);
# step "Fail job if integration tests failed" conclusion=failure.

# (3) Confirm the CI integration job ships only postgres + nats (no ollama / core / ml)
grep -nE '^\s*services:|^\s*ollama:|^\s*postgres:|^\s*nats:' .github/workflows/ci.yml
# Expected:
#   line ~219 (services: block) — only `postgres:` is declared
#   step "Start NATS with auth and JetStream" — docker run nats
#   no ollama / smackerel-core / smackerel-ml anywhere in the workflow

# (4) Confirm local `./smackerel.sh test integration` brings up the full Compose stack
sed -n '685,720p' smackerel.sh
# Expected: case "integration" branch invokes test_runtime_health.sh with KEEP_STACK_UP=1
# (which calls `./smackerel.sh --env test up` to bring up ollama + core + ml + postgres + nats),
# then runs the Go tests with --network host.

# (5) Confirm local validation is green at HEAD 5c8d857e
git rev-parse HEAD     # expect 5c8d857e80a07f59600f51b9e9bce906814a6311
./smackerel.sh check            # expected exit 0
./smackerel.sh test integration # expected exit 0 (per pre-push validation evidence)

# (6) Confirm CI is red at the same HEAD
curl -s "https://api.github.com/repos/pkirsanov/smackerel/actions/runs/25974673514" | python3 -m json.tool | grep -E '"(conclusion|head_sha)"' | head -5
# Expected: head_sha = 5c8d857e..., conclusion = failure

# (7) Authenticate gh CLI and download the integration-test-log artifact to confirm the failing test name
gh auth login
gh run download 25974673514 -n integration-test-log -D /tmp/bug-045-002-int-log
grep -E '^--- FAIL|^FAIL\s|panic:|fatal' /tmp/bug-045-002-int-log/integration-test.log
# Expected after auth: the artifact zip contains integration-test.log;
# grep returns the failing test name(s) and error message(s). This step
# is BLOCKED in the discovery phase (anonymous API access denied);
# `bubbles.design` or `bubbles.implement` MUST run it before fix design.
```

## Expected Behavior

After the fix, a reader inspecting the system should observe:

1. **CI `integration` job conclusion is `success` on every `main` push.** `curl -s https://api.github.com/repos/pkirsanov/smackerel/actions/workflows/ci.yml/runs?branch=main&per_page=10` returns 10/10 `conclusion: success` runs after the fix lands.
2. **The CI integration job's backing-service topology covers every dependency the integration test suite requires** OR **the integration test suite is partitioned so the subset that runs in CI uses only the CI-available services** (postgres + nats) and the subset that requires ollama / core / ml is gated behind a build tag (e.g. `//go:build integration && local_stack`) and runs ONLY under `./smackerel.sh test integration`. Whichever path the fix takes, the contract is encoded as a guard test that fails build if a new ollama-dependent integration test is added to the CI-runnable set without the gate tag.
3. **The Uncertainty Declaration recorded in BUG-045-001's `report.md` is resolved.** Either the CI integration job log is captured for the failing-test attribution (resolving the Uncertainty), or the fix design proves the attribution by removing the failure mode (the CI-incompatible tests are no longer in the CI-runnable set).
4. **The chronic-failure pattern's root cause is named.** The fix design's `report.md` cites the specific failing test name(s), the specific HTTP/Docker call(s) those tests make, and the specific service(s) the call(s) target — VERBATIM from the captured integration-test.log.
5. **A regression detector exists.** A guard test in `tests/contract/` or `internal/deploy/` asserts that for every test file in `tests/integration/` that imports `net/http`, contains `os/exec`, or references `localhost:11434` / `localhost:8080` / `localhost:8081`, EITHER the file carries a build tag that excludes it from CI OR the CI workflow declares the matching backing service. The guard fails build if a new violator is added.

## Acceptance Criteria

1. **AC-1 — CI `integration` job green on `main`.** After fix lands, the next push to `main` produces a CI workflow run where the `integration` job conclusion is `success`. Verifiable via `curl -s https://api.github.com/repos/pkirsanov/smackerel/actions/runs/<NEW_RUN_ID>/jobs`. Adversarial: also assert the prior failure pattern is broken — at least 3 consecutive `success` runs on `main` after the fix HEAD.
2. **AC-2 — Verbatim failing-test attribution.** `bubbles.design` or `bubbles.implement` MUST authenticate `gh` CLI, download the `integration-test-log` artifact from run `25974673514` (or any subsequent failing run if `25974673514` has expired by the time fix design starts), grep for `--- FAIL:` / `FAIL\t` / `panic:` / `fatal`, and record the verbatim failing-test name(s) + error message(s) in this packet's `report.md`. This resolves BUG-045-001's Uncertainty Declaration. No fix design may proceed without this evidence.
3. **AC-3 — Service-topology contract.** EITHER (a) the CI workflow declares every backing service the CI-runnable integration test set requires (likely adding ollama as a service container or as a docker-run sidecar, mirroring the existing nats pattern), OR (b) the integration test set is partitioned via build tag so the CI-runnable subset has only postgres + nats dependencies. Decision recorded in `design.md` with the trade-off cited (CI runner resource cost vs. coverage loss).
4. **AC-4 — Build-time guard.** A new guard test (location: `internal/deploy/ci_integration_service_topology_contract_test.go` or peer) asserts the contract chosen in AC-3 and fails build if a new integration test is added that violates it.
5. **AC-5 — Chronic-failure history pattern broken.** Verifiable via `curl` of the workflow-runs endpoint after at least 3 `main` pushes following the fix HEAD: all 3 most recent runs show `conclusion: success`.
6. **AC-6 — BUG-045-001 close-out reconciled.** This packet's close-out `report.md` adds a cross-reference to BUG-045-001's `state.json` recording that the chronic CI failure cited in BUG-045-001 § Severity bullet (1) is resolved BY this packet, not by BUG-045-001's validator routing fix. The cross-reference appears in BUG-045-001's `state.json` under a new `subsequentResolutions` entry pointing to BUG-045-002.

## Out of Scope

- The BUG-045-001 validator routing fix (`internal/config/config.go::validateModelEnvelopes`) — already certified done at HEAD `5c8d857e` and unchanged by this packet.
- The BUG-045-001 default model swaps in `config/smackerel.yaml` (gemma4:26b → gemma3:4b etc.) — already certified done and unchanged.
- The build workflow's Trivy ml scan failure — separate bug packet `specs/047-ci-image-vulnerability-gate/bugs/BUG-047-002-trivy-ml-fixable-cve-regression/` covers that.
- Any change to the local `./smackerel.sh test integration` command surface — the local runner is green and stays unchanged.

## Uncertainty Declaration

**Claim Source:** interpreted (Phase 1 RCA without log access).

The "Summary" hypothesis that the CI failure is caused by integration tests reaching for an absent ollama / smackerel-core / smackerel-ml service is the most likely class of failure given the observable environment-mismatch evidence (Reproduction step 3 vs step 4) and the BUG-045-001 author's explicit deferral of CI-log inspection. It is NOT a verified attribution. Until `bubbles.design` or `bubbles.implement` performs AC-2 (authenticated `gh` CLI download of the integration-test-log artifact and verbatim grep for failing-test name), the actual failing test could be:

- An ollama-dependent test (matches the hypothesis)
- A smackerel-core or smackerel-ml HTTP-dependent test (matches the hypothesis)
- A `docker exec` test (matches the hypothesis)
- A test that depends on a NATS subject or stream that the local Compose stack pre-populates but CI does not (matches the hypothesis class)
- A test that has a hardcoded path expecting the local Compose stack's volume layout (matches the hypothesis class)
- A test affected by a CI-only constraint not in the above list (e.g. GH runner kernel version, GH runner Docker version, GH runner network policy, missing host package, etc.) — would invalidate the hypothesis and require a different fix design

AC-2 is the gate. The fix design CANNOT proceed without it.
