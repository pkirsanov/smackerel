# Bug Fix Design: BUG-031-005 Stress Stack Health Readiness

## Root Cause Analysis

### Investigation Summary
The authoritative red evidence comes from spec 039's feature-level regression phase on 2026-05-03T22:05:34Z. Shell stress health and search checks passed, then the Go stress phase failed across unrelated package owners while waiting for `http://127.0.0.1:40001` or while pinging the database. Source inspection confirms the shared harness boundary is inconsistent: `smackerel.sh test stress` runs shell stress scripts using the `test` environment, then switches the Go stress Docker phase to `dev` generated env values.

### Root Cause
The stress command does not maintain one coherent stack readiness contract across shell and Go stress phases. The Go stress phase currently consumes `dev` `CORE_EXTERNAL_URL` and `DATABASE_URL` after shell stress has prepared and cleaned up a `test` stack. It also omits `NATS_URL`, even though `tests/stress/agent/concurrency_test.go` requires it after the DB connection succeeds. This creates a shared infrastructure failure that looks like multiple package stress failures.

### Impact Analysis
- Affected components: `./smackerel.sh test stress`, `scripts/runtime/go-stress.sh`, shell stress scripts, Go stress packages, disposable test-stack lifecycle.
- Affected specs: spec 031 owns live-stack test lifecycle; spec 039 is currently blocked; spec 025, spec 037, spec 038, and spec 040 stress packages are collateral victims.
- Affected data: stress runs must use disposable test storage or explicit test cleanup; persistent dev storage must not be required for automated stress validation.
- Affected users/operators: delivery agents cannot distinguish shared stack readiness regression from feature-owned stress workload regressions.

## Fix Design

### Solution Approach
Route implementation to the shared stack/lifecycle owner. The expected repair is to make `./smackerel.sh test stress` provision, verify, and pass one SST-derived stress environment to every stress phase. The likely target is the disposable `test` environment already used by the shell stress scripts, unless the owning DevOps agent formally changes the stress contract and updates docs, specs, and tests together.

The repair should add or reuse a Go stress readiness canary that runs before package workloads. The canary must verify:
- `CORE_EXTERNAL_URL` points to the intended stress stack and `/api/health` is reachable.
- `DATABASE_URL` points to the intended stress database and accepts a ping from the Go stress container.
- `NATS_URL` points to the intended stress NATS endpoint and accepts a connection from the Go stress container.
- `SMACKEREL_AUTH_TOKEN` is present and accepted by an authenticated core request.

The canary must fail as infrastructure readiness when these checks fail, then stop before feature-owned workload tests run. After the canary passes, the workload tests must retain their existing assertions and failure modes.

### Alternative Approaches Considered
1. Treat the recommendation stress timeout as spec 039-owned. Rejected because knowledge, photos, drive, and agent stress packages fail on the same readiness boundary before recommendation-specific workload behavior is reached.
2. Treat this as a spec 022 operational resilience bug. Rejected for initial ownership because the evidence is primarily test-stack lifecycle and env handoff, while spec 031 owns disposable live-stack validation and test isolation.
3. Skip Go stress packages when the dev stack is unavailable. Rejected because it would preserve the hidden mismatch and weaken required stress validation.
4. Keep Go stress on dev and document that operators must run `./smackerel.sh up` first. Rejected for automated validation because repo-standard stress should be self-contained or fail with a clear readiness contract, not depend on an implicit persistent dev stack.

## Change Boundary

Allowed implementation surfaces for the next owner:
- `smackerel.sh` stress command routing and env handoff.
- `scripts/runtime/go-stress.sh` argument, readiness, and package-selection behavior.
- `tests/stress/**` shared helpers or a new stress readiness canary test.
- `tests/stress/test_health_stress.sh` and `tests/stress/test_search_stress.sh` only if stack ownership must be coordinated with Go stress.
- `config/smackerel.yaml` and config-generation scripts only if the owner proves the missing value cannot be emitted from the current SST contract.
- Documentation for stress command semantics in `docs/Testing.md`, `docs/Development.md`, or parent spec 031 artifacts if command behavior changes.

Excluded surfaces:
- Recommendation ranking, provider, policy, attribution, digest, Telegram, or web behavior.
- Knowledge, photos, drive, or agent workload logic unless a residual package-specific failure remains after readiness is fixed.
- Generated config under `config/generated/`.
- Persistent dev volume cleanup beyond project-scoped lifecycle commands.

## Regression Test Design

Regression coverage must be adversarial and must fail if the bug is reintroduced:
- Wrong-stack adversarial case: shell stress prepares the test stack but the Go phase is pointed at the dev core URL or another unused port; expected result is a single readiness failure that names the mismatched core target before package workload tests run.
- DB adversarial case: Go stress receives an unreachable `DATABASE_URL`; expected result is a readiness failure that names DB reachability, not a package-specific timeout.
- NATS adversarial case: Go stress receives missing or unreachable `NATS_URL`; expected result is a readiness failure that names NATS reachability before agent concurrency starts.
- Workload preservation case: with readiness healthy, force or select a real workload assertion path and verify its failure remains a package workload failure, not a readiness skip or command pass.

Required validation commands remain repo-standard:
- `./smackerel.sh test stress`
- `./smackerel.sh test integration`
- `./smackerel.sh test e2e`
- `./smackerel.sh test unit`
- `./smackerel.sh check`
- `./smackerel.sh lint`
- `./smackerel.sh format --check`

Artifact gates for this bug packet:
- `bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness`
- `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness`

## Risks & Open Questions
- The next owner must decide whether stress should always manage the `test` stack or whether a separate stress environment should be added to SST. Either path must remain SST-derived and repo-CLI-owned.
- If readiness is repaired and residual workload failures remain, those residual failures should be split to spec 025, spec 037, spec 038, spec 039, or spec 040 with fresh evidence.
- The canary must not overfit to port `40001`; it should assert environment consistency and reachability through SST-derived values.
