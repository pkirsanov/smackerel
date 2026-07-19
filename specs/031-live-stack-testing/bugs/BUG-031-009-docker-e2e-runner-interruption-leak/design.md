# Bug Fix Design: BUG-031-009

## Root Cause Analysis

### Investigation Summary

The BUG-031-004 reaper marks a host command with `SMACKEREL_E2E_CHILD_RUN_ID`, discovers host descendants, escalates TERM to KILL, then tears down the Compose project. The Go E2E block starts `docker run --rm ... golang:1.25.10-bookworm`. Docker daemon, not the CLI process, owns the resulting container process, and the run ID is neither a Docker label nor passed into the container. Killing the CLI cannot guarantee container termination.

### Root Cause

The harness models child ownership only as a host process tree. Docker containers are a second ownership domain and need an explicit run-ID projection plus a scoped removal step. Without it, teardown ordering is inverted from the test's perspective: dependencies disappear before the test workload.

### Impact Analysis

- Affected components: `smackerel.sh` E2E child lifecycle and interruption regression.
- Affected data: disposable test state only.
- Affected users: developers and CI interpreting broad E2E failures.
- Observed cascade: Drive health timeout followed by missing core/network/DNS failures in later packages.

## Fix Design

### Single-Implementation Justification

The existing `e2e_run_child` / `e2e_stop_child` pair remains the one lifecycle implementation for shell and Docker children. The Docker label is an ownership projection consumed by that same cleanup path, not a parallel adapter.

### Solution Approach

1. Define one label key for E2E child ownership.
2. Add `--label <key>=${e2e_child_run_id}` to every Dockerized child launched through `e2e_run_child` in the E2E branch, including baseline Go and opt-in Ollama Go runners.
3. Add a scoped container reaper that queries the exact label value, issues `docker rm --force` for matches, and verifies no exact-label container remains.
4. Invoke the container reaper in `e2e_stop_child` before host process-group termination and before stack teardown.
5. Extend the existing BUG-031-004 shell regression with a nested focused Go runner interruption case and an adversarial detector proof.

### Alternatives Rejected

1. Rely on killing the Docker CLI - rejected because daemon-owned container lifetime is independent.
2. Use image/network filters only - rejected because those can match unrelated work and violate scoped cleanup.
3. Add `--name` only - rejected because labels support exact filtering without global name collision and work for every runner type.
4. Delay teardown - rejected because it hides the ordering defect and wastes the timeout budget.

## Change Boundary

Allowed:

- `smackerel.sh`
- `tests/e2e/test_timeout_process_cleanup.sh`
- focused harness contract tests if needed
- this BUG-031-009 packet
- routing metadata in the originating BUG-038-003 packet

Excluded:

- product runtime search/Drive code
- arbitrary sleeps or longer health waits
- synthesis/assistant packets
- deployment adapters, manifests, secrets, evo-x2, `knb`, and release-train bundles

## Complexity Tracking

| Decision | Simpler fix considered | Why rejected |
|---|---|---|
| Exact run-ID Docker label and reaper | Kill Docker CLI only | CLI termination does not own daemon-side container lifetime |
