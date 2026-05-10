# BUG-002 Design — Replace `wget` healthcheck with in-image `ollama` CLI + adversarial binary-presence guard

## Current Truth

- `docker-compose.yml` line 182 defines `services.ollama.healthcheck.test: ["CMD-SHELL", "wget --no-verbose --tries=1 --spider http://localhost:${OLLAMA_CONTAINER_PORT}/api/tags || exit 1"]`.
- `deploy/compose.deploy.yml` line 199 carries the same `wget`-based literal (mirror of the SST surface; see BUG-001 design OQ-BUG-001-A).
- `ollama/ollama:0.23.2` (the image pinned by BUG-001) does NOT ship `wget` or `curl`. `docker exec smackerel-test-ollama-1 wget …` exits 127 with `executable file not found in $PATH`.
- The image DOES ship `/usr/bin/ollama` (the ollama CLI) on PATH, and `ollama list` exits 0 when the daemon is reachable on `OLLAMA_HOST` (defaults to `http://127.0.0.1:11434` inside the container) and exit 1 when unreachable. This is a real liveness signal, not just a process-alive probe.
- `interval: 10s`, `timeout: 5s`, `retries: 5`, `start_period: 30s` are correctly tuned per spec 043 Scope 01 and have no relationship to the broken `test:` line — the bug is purely the command, not the timing.
- The `internal/deploy/compose_ollama_contract_test.go` contract test asserts image hoist + profile gate + volume indirection, but does NOT inspect the healthcheck command — that contract surface had no coverage of healthcheck-binary-existence prior to this fix.

## Root cause

The `wget`-based healthcheck originated when the image was `ollama/ollama:0.6` (which happened to ship `wget`). When BUG-001 bumped the image to `ollama/ollama:0.23.2` for an unrelated reason (yanked manifest), nothing in the test suite parsed the healthcheck command against the binaries actually present in the new image. The misalignment between compose-side healthcheck command and image-side binary set went undetected until `docker compose up -d --wait` failed with exit 124.

## Change shape

1. **Replace the healthcheck command** in `docker-compose.yml` line 182:
   - Before: `test: ["CMD-SHELL", "wget --no-verbose --tries=1 --spider http://localhost:${OLLAMA_CONTAINER_PORT}/api/tags || exit 1"]`
   - After: `test: ["CMD", "ollama", "list"]`
   - Switch from `CMD-SHELL` to `CMD` form: drops the `/bin/sh -c` wrapper, runs the binary directly. Cleaner, matches the in-image semantics (no shell expansion needed). The `${OLLAMA_CONTAINER_PORT}` substitution is no longer needed because `ollama list` reads `OLLAMA_HOST` (which defaults correctly inside the container).

2. **Mirror the change** in `deploy/compose.deploy.yml` line 199 with the same replacement.

3. **Preserve** `interval: 10s`, `timeout: 5s`, `retries: 5`, `start_period: 30s` in both files. They are spec-043-tuned and orthogonal to the bug.

4. **New adversarial regression test** `tests/integration/ollama_healthcheck_test.go` (build tag `integration`):

   - `assertOllamaHealthcheckUsesInImageBinary(yamlBytes []byte) error` — parses the compose YAML, locates `services.ollama.healthcheck.test`, extracts the FIRST command token (handling both `CMD` form `[binary, arg1, …]` and `CMD-SHELL` form `[CMD-SHELL, "binary args"]`), and asserts:
     - The first token is NOT in a forbidden-binaries set: `{wget, curl, /usr/bin/wget, /usr/bin/curl, /bin/wget, /bin/curl}`. The error names the offending binary AND the image (`ollama/ollama:0.23.2`).
     - The first token IS in an allowlist of in-image binaries: `{ollama, /usr/bin/ollama, /bin/ollama}` (or equivalently `[CMD-SHELL, "ollama …"]` where the shell command starts with `ollama`).
   - `TestOllamaHealthcheck_LiveFiles` — runs the helper against `docker-compose.yml` AND `deploy/compose.deploy.yml`. Fail-loud (`t.Fatalf`) when either file is missing, fails to parse, or violates the contract.
   - `TestOllamaHealthcheck_AdversarialMissingBinary` — runs the helper against a synthetic compose snippet whose healthcheck calls `wget --spider …`. Asserts the helper returns an error that mentions `wget` AND `ollama/ollama:0.23.2`.
   - `TestOllamaHealthcheck_AdversarialMissingBinaryCurl` — same, with `curl` (proves the forbidden-binaries set is enforced for both, not just one).
   - `TestOllamaHealthcheck_AdversarialCMDShellWrappedWget` — runs the helper against a synthetic snippet that uses the original `CMD-SHELL` wrapper form (`["CMD-SHELL", "wget …"]`). Asserts the helper still rejects it (proves the validator handles both `CMD` and `CMD-SHELL` forms).
   - **No `t.Skip()`** of any kind. Missing file, parse failure, contract violation are all fail-loud conditions.

## Why `ollama list` is the right command

| Property | `ollama list` | `wget --spider /api/tags` (original) |
|----------|---------------|--------------------------------------|
| In-image | YES (`/usr/bin/ollama`) | NO (`wget` not in PATH on `0.23.2`) |
| Exit 0 only when daemon reachable | YES (verified live: exit 1 with bad `OLLAMA_HOST`) | YES (when `wget` is present) |
| Exit fast | YES (single TCP roundtrip to `127.0.0.1:11434/api/tags`, ~ms) | YES |
| No external network dependency | YES (loopback only) | YES |
| Honors compose env-var indirection | YES (defaults to `http://127.0.0.1:11434` which matches `OLLAMA_CONTAINER_PORT=11434`) | YES |
| Survives image base rebuild | LIKELY (the `ollama` binary is the entire purpose of the image; would only break if the image renamed the binary) | NO — broke when the image switched to a slimmer base |

`ollama list` lists models on disk. The list is empty in a fresh container — that is fine; we are checking liveness, not model presence. The deterministic-test-model pull script (`scripts/commands/ollama-test-pull.sh`) is the model-readiness signal and runs separately, AFTER the healthcheck reports green.

## Why we use `CMD` form, not `CMD-SHELL` form

- The original `CMD-SHELL` form needed shell expansion of `${OLLAMA_CONTAINER_PORT}`. The new command does not need any expansion.
- `CMD` form runs the binary directly via the OCI exec API. No `/bin/sh -c` wrapper. One fewer process. Matches what the ollama image actually has at PATH.
- Other healthchecks in the same files use `CMD` form for the same reason (e.g. `["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:${NATS_MONITOR_PORT}/healthz"]`). Consistent style.

## Backward compatibility

- The compose-side healthcheck contract (`interval`, `timeout`, `retries`, `start_period`) is unchanged.
- The integration runner's wait timeout (80s default — `start_period 30s + 5 retries × 10s interval = 80s` ceiling) is unchanged.
- Consumer code (Go core, Python sidecar, scripts) does not invoke the healthcheck directly — they hit `/api/tags` or use the ollama HTTP client. They are unaffected.
- `tests/integration/ollama_image_availability_test.go` (the BUG-001 adversarial registry-existence guard) is unaffected — different concern.
- `tests/integration/ollama_config_contract_test.go` (the SST round-trip test) is unaffected — it does not inspect the healthcheck.
- `internal/deploy/compose_ollama_contract_test.go` (the compose contract for image/profile/volume) is unaffected — it does not inspect the healthcheck.
- The new test file lives under `tests/integration/`, build tag `integration`, so it does not change the default `./smackerel.sh test unit` surface.

## Why the SST grep guard is unaffected

`internal/config/sst_grep_guard_test.go` enforces zero-hardcoded-values for the literals `11434`, `qwen2.5`, and `ollama/ollama:`. The fix introduces no new literals from that set in production source paths. The new test file under `tests/integration/` is allowlisted (the `tests/` directory carve-out + the `_test.go` suffix). Zero new SST violations.

## Rejected alternatives

- **Install `wget` in a custom-built ollama image.** Rejected — adds image-build complexity, defeats the upstream-image pin (BUG-001), requires maintaining a custom Dockerfile for ollama, and `ollama list` is already in the image with the right semantics.
- **Healthcheck via `pgrep ollama` or `cat /proc/1/cmdline`.** Rejected — these probes pass even when the HTTP server has crashed but the process is still alive. Not a real liveness signal.
- **Healthcheck via TCP probe (`bash -c "</dev/tcp/127.0.0.1/11434"`).** Rejected — the image is unlikely to ship bash; even if it did, this is a transport-layer probe that does not prove the API is responsive. `ollama list` is application-layer.
- **Drop the healthcheck entirely.** Rejected — `docker compose up -d --wait` would no longer wait for ollama, but downstream tests that expect a ready ollama would race against pull-script invocation. Spec 043 design specifies real liveness gating.
- **Switch the deploy-side compose file to substitute `${OLLAMA_IMAGE}`.** Out of scope per BUG-001 design OQ-BUG-001-A; tracked as a follow-up. The literal in `deploy/compose.deploy.yml` is updated in lockstep with `docker-compose.yml`.

## Open questions (deferred — NOT blocking this fix)

- **OQ-BUG-002-A:** Should the healthcheck-binary guard be generalized to all services (postgres / nats / smackerel-core / smackerel-ml)? Pinging known images for `which <binary>` would fully close the loop for every service. Out of scope for this P0 unblock; can be tracked alongside OQ-BUG-001-A and OQ-D2.
- **OQ-BUG-002-B:** Should the integration runner emit a more actionable error when `docker compose up -d --wait` exits 124, naming the unhealthy container and surfacing the last few healthcheck log lines? Today it just exits 124 and the operator has to grep `docker inspect` for the cause. Improves DX but does not affect correctness.
