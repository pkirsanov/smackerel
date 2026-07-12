# BUG-002 — Ollama healthcheck calls `wget`, which is not in `ollama/ollama:0.23.2`

> **Parent feature:** [specs/043-ollama-test-infrastructure](../../)
> **Parent scope:** Scope 01 (Config + Compose Foundation) — owns the compose-side ollama service contract
> **Related bug:** [BUG-001](../BUG-001-ollama-image-pin-stale/) — that fix bumped the image from `ollama/ollama:0.6` → `ollama/ollama:0.23.2` (commit `ea2af19a`), which exposed this bug
> **Filed by:** `bubbles.bug` (bugfix-fastlane)
> **Filed at:** 2026-05-10
> **Severity:** P0 — blocks the entire `./smackerel.sh test integration` lane and every downstream spec that uses the integration stack
> **Status:** Fixed

---

## Symptom

`docker-compose.yml` line 182 (and the deploy-side mirror `deploy/compose.deploy.yml` line 199) define the ollama service healthcheck as:

```yaml
healthcheck:
  test: ["CMD-SHELL", "wget --no-verbose --tries=1 --spider http://localhost:${OLLAMA_CONTAINER_PORT}/api/tags || exit 1"]
```

The `ollama/ollama:0.23.2` image (pinned by BUG-001) does NOT contain `wget` or `curl`. The container starts fine and ollama serves on `:11434`, but every healthcheck invocation fails with:

```
OCI runtime exec failed: exec failed: unable to start container process: exec: "wget": executable file not found in $PATH
```

`docker compose up -d --wait` therefore reports the container `unhealthy` and `--wait-timeout` elapses, returning exit code 124. This blocks the integration runner at the stack-up step before any Go test gets a chance to run.

## Reproduction

Pre-fix:

```
$ docker run --rm -d --name ollama-debug ollama/ollama:0.23.2
$ docker exec ollama-debug wget --no-verbose --tries=1 --spider http://localhost:11434/api/tags
OCI runtime exec failed: exec failed: unable to start container process: exec: "wget": executable file not found in $PATH

$ docker exec ollama-debug curl -sS http://localhost:11434/api/tags
OCI runtime exec failed: exec failed: unable to start container process: exec: "curl": executable file not found in $PATH
```

End-to-end reproduction in the integration runner:

```
$ ./smackerel.sh test integration
[+] Running 5/6
 ✔ Container smackerel-test-postgres-1        Healthy                     14.2s
 ⠦ Container smackerel-test-ollama-1          Waiting                     80.7s
 ✔ Container smackerel-test-nats-1            Healthy                     14.2s
 ✔ Container smackerel-test-smackerel-ml-1    Healthy                     18.1s
 ✔ Container smackerel-test-smackerel-core-1  Healthy                     19.1s
container smackerel-test-ollama-1 is unhealthy
EXIT=124
```

Discovered while running the validate phase of spec 044; the integration stack startup failed at the ollama healthcheck step.

## Impact (P0)

| Surface | Impact |
|---------|--------|
| `./smackerel.sh test integration` | BLOCKED — `docker compose up -d --wait --wait-timeout 80` exits 124 because ollama never reports healthy. Every downstream Go integration test fails to even start. |
| `./smackerel.sh test e2e` (live-stack lane) | BLOCKED — same `up` path, same wait-timeout. |
| `./smackerel.sh --env test up` | BLOCKED — same wait. |
| `./smackerel.sh --env self-hosted up --profile ollama` | BLOCKED — same `wget`-based healthcheck in `deploy/compose.deploy.yml`. |
| `./smackerel.sh test unit` | NOT IMPACTED — unit lane does not bring up the test stack. |
| `./smackerel.sh check` | NOT IMPACTED — config-only validation. |

The ollama daemon itself is fine — only the liveness probe is broken, but compose treats `unhealthy` as a deploy failure under `--wait`, so the entire integration lane is hard-blocked.

## Expected behavior

- The healthcheck command MUST execute a binary that exists inside the `ollama/ollama:0.23.2` image (and any future image we pin).
- The command MUST signal real liveness — i.e. it must fail when the ollama HTTP server is unreachable, not just when the container process is alive.
- A regression test MUST guard against re-introducing a healthcheck that depends on a binary not in the image, so a future image switch (or a future stripped-down image) cannot silently re-introduce this failure mode.

## Root cause

The `wget`-based healthcheck originated when the image was `ollama/ollama:0.6`, which happened to ship `wget` in PATH. BUG-001 bumped the image to `0.23.2` for an unrelated reason (yanked manifest); the new image (built on a slimmer base — likely `gcr.io/distroless/cc-debian12` or similar) ships only the `ollama` binary in PATH. Nothing in the Smackerel test suite parses the healthcheck command against the binaries actually present in the image, so the regression slipped past every static gate and only surfaced when `docker compose up -d --wait` failed at integration time.

The bug is mechanically the same shape as BUG-001: a static literal in the compose file became misaligned with the image we now pull, and the pre-existing test surface had no way to catch the misalignment until the container actually ran. BUG-001 closed the *image existence* loop. BUG-002 closes the *healthcheck binary existence* loop.

## Fix outcome

- Replace the `wget`-based `CMD-SHELL` healthcheck in **both** `docker-compose.yml` and `deploy/compose.deploy.yml` with the in-image `ollama` CLI: `["CMD", "ollama", "list"]`. `ollama list` is a real liveness signal — it returns exit 0 only when the daemon is reachable on `OLLAMA_HOST` (defaults to `http://127.0.0.1:11434` inside the container, which is what we want), and exit 1 when the daemon is unreachable. Verified live against `ollama/ollama:0.23.2`:

  ```
  $ docker exec smackerel-test-ollama-1 which ollama
  /usr/bin/ollama
  $ docker exec smackerel-test-ollama-1 ollama list
  NAME    ID    SIZE    MODIFIED
  EXIT=0
  $ docker exec -e OLLAMA_HOST=http://127.0.0.1:11999 smackerel-test-ollama-1 ollama list
  Error: could not connect to ollama server, run 'ollama serve' to start it
  EXIT=1
  ```

- Preserve `interval`, `timeout`, `retries`, `start_period` semantics from spec 043.
- Add an adversarial regression test `tests/integration/ollama_healthcheck_test.go` (build tag `integration`) that:
  - Parses both compose files and asserts the ollama service's healthcheck command starts with an in-image binary (`ollama`).
  - Adversarial sub-cases: assert the validator rejects synthetic compose snippets that put `wget` or `curl` as the first token, with an error message that names the missing binary AND the image it is missing from.
  - Fail-loud (no `t.Skip()`) on parse errors.

## Acceptance scenarios

```gherkin
Scenario: SCN-BUG-002-001 ollama container reports healthy via in-image command
  Given docker-compose.yml services.ollama.healthcheck uses an in-image binary (ollama)
  When `./smackerel.sh test integration` brings the test stack up
  Then `docker ps --filter name=smackerel-test-ollama` shows status `Up (healthy)` within `start_period + interval × retries` seconds
  And `docker compose up -d --wait` exits 0 instead of timing out at 80s

Scenario: SCN-BUG-002-002 Adversarial — guard rejects healthcheck commands that reference binaries not in the ollama image
  Given the regression test inspects a synthetic compose snippet whose ollama healthcheck calls `wget`
  When the validator parses the snippet
  Then the validator rejects it with an error that names `wget` and the `ollama/ollama:0.23.2` image
  And the same code path applied to the live compose files reports OK
```

## Adversarial regression

`tests/integration/ollama_healthcheck_test.go::TestOllamaHealthcheck_AdversarialMissingBinary` exercises the same `assertOllamaHealthcheckUsesInImageBinary` helper as the live test against a synthetic compose snippet whose healthcheck calls `wget`. The validator MUST reject it with an error that names `wget` AND the `ollama/ollama:0.23.2` image. Pre-fix, the helper applied to the live `docker-compose.yml` would have produced the same rejection — it would have caught BUG-002 at the time it was introduced.

The adversarial proves the live test (`TestOllamaHealthcheck_LiveFiles`) is not tautological: different inputs route through identical code and produce different outputs depending on whether the first token of the healthcheck command is in an allowlist of in-image binaries.

## Out of scope

- Switching to a non-CLI healthcheck (e.g. installing `wget`/`curl` in a custom-built ollama image): the ollama CLI is already in the image, requires no rebuild, and provides a real liveness signal.
- Generalizing the healthcheck-binary guard to all services in compose (postgres uses `pg_isready`, nats uses `wget` against `:8222/healthz` and the nats image ships `wget`, smackerel-core/ml use `wget` and the core/ml images install `wget` in their Dockerfiles): scoped to the ollama service that actually broke. A broader audit can be tracked separately if other images ever drop `wget`.
- Pinning the image by content digest (deferred per spec 043 OQ-D2 and BUG-001 design.md OQ-BUG-001-A).
- Switching `deploy/compose.deploy.yml` to substitute `${OLLAMA_IMAGE}` (deferred per BUG-001 design.md OQ-BUG-001-A).
