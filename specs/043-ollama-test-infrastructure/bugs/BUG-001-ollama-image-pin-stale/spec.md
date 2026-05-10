# BUG-001 — Ollama image pin `ollama/ollama:0.6` yanked from Docker Hub

> **Parent feature:** [specs/043-ollama-test-infrastructure](../../)
> **Parent scope:** Scope 01 (Config + Compose Foundation) — owns `infrastructure.ollama.image` and `infrastructure.ollama.test.image`
> **Filed by:** `bubbles.bug` (bugfix-fastlane)
> **Filed at:** 2026-05-10
> **Severity:** P0 — blocks ALL integration tests; blocks operator-driven cold-pull lane for spec 043 itself
> **Status:** Fixed

---

## Symptom

Spec 043 Scope 01 pinned the Ollama image to `ollama/ollama:0.6` in two SST keys:

- `config/smackerel.yaml` → `infrastructure.ollama.image: ollama/ollama:0.6`
- `config/smackerel.yaml` → `infrastructure.ollama.test.image: ollama/ollama:0.6`

That tag has been yanked from Docker Hub. Any environment that auto-starts Ollama (test env auto-starts via `environments.test.ollama_enabled: true` per spec 043 Scope 03) cannot start because the image pull fails:

```
ollama Error manifest for ollama/ollama:0.6 not found: manifest unknown: manifest unknown
```

The Docker Hub Tags API confirms the regression:

```
$ curl -sS -o /dev/null -w "%{http_code}\n" https://hub.docker.com/v2/repositories/ollama/ollama/tags/0.6
404

$ curl -sS -o /dev/null -w "%{http_code}\n" https://hub.docker.com/v2/repositories/ollama/ollama/tags/0.23.2
200
```

The latest stable release per [`https://github.com/ollama/ollama/releases/latest`](https://github.com/ollama/ollama/releases/latest) is `v0.23.2`.

## Reproduction

1. From a clean checkout, run `./smackerel.sh --env test config generate` (this writes `config/generated/test.env` with `OLLAMA_IMAGE=ollama/ollama:0.6` and `ENABLE_OLLAMA=true`).
2. Run `./smackerel.sh test integration`. The runner attempts to bring up the test compose stack.
3. The `ollama` service in `docker-compose.yml` resolves `image: ${OLLAMA_IMAGE}` to `ollama/ollama:0.6` and `docker compose up -d --wait` fails:

   ```
   ollama Error manifest for ollama/ollama:0.6 not found: manifest unknown: manifest unknown
   ```
4. Same failure mode for the operator-driven cold-pull lane: `SMACKEREL_TEST_OLLAMA=1 ./smackerel.sh test e2e` cannot pull the image either.

Discovered while running spec 044 Scope 1 integration tests; the auth-test stack startup failed at the ollama pull step.

## Impact (P0)

| Surface | Impact |
|---------|--------|
| `./smackerel.sh test integration` | BLOCKED — test stack fails to start because `ENABLE_OLLAMA=true` (spec 043 Scope 01) and the pinned image is unpullable. Affects every downstream integration test, including spec 044 auth integration tests. |
| `SMACKEREL_TEST_OLLAMA=1 ./smackerel.sh test e2e` | BLOCKED — operator-driven cold-pull lane for spec 043 itself cannot run. The `scripts/commands/ollama-test-pull.sh` HTTP `/api/pull` call fails because the daemon never starts. |
| `./smackerel.sh --env home-lab up --profile ollama` | BLOCKED — home-lab opt-in lane cannot start ollama. |
| `./smackerel.sh test unit` (Go + Python) | NOT IMPACTED — unit tests do not touch the live stack. |
| `./smackerel.sh check` | NOT IMPACTED — config-only validation. |

## Expected behavior

- Both SST keys MUST point to a Docker Hub tag whose manifest is currently published.
- A regression test MUST run on every integration pass that asserts the pinned tag is currently available, so a future yanked tag fails LOUD before the test stack tries to start (rather than silently inside `docker compose up`).

## Root cause

The image pin was a static literal that did not participate in any liveness check. There was no test exercising the registry-side existence of the tag — only the SST→env-file round-trip was guarded. Upstream Docker Hub yanked `0.6` (likely as part of normalizing semver-style tags now that Ollama has reached `0.23.x`), and nothing in the Smackerel test suite caught the regression until an integration run failed at the docker-pull step.

## Fix outcome

- Re-pin both SST keys to `ollama/ollama:0.23.2` (verified live HTTP 200 against the Docker Hub Tags API at the time of fix).
- Mirror the bump in `deploy/contract.yaml` (build/deploy contract pins) and `deploy/compose.deploy.yml` (deploy-side compose).
- Update the strip target in `tests/integration/ollama_config_contract_test.go` so the existing `AdversarialMissingTestImage` strip-and-rerun fixture continues to find its target line in the live YAML.
- Add a NEW adversarial regression test `tests/integration/ollama_image_availability_test.go` that:
  - Reads `OLLAMA_IMAGE` from `config/generated/test.env` (zero hardcoded values — honors SCN-OLLAMA-006).
  - HEADs `https://hub.docker.com/v2/repositories/ollama/ollama/tags/<tag>` and asserts HTTP 200.
  - Adversarial sub-case: confirms the same code path returns HTTP 404 for the known-yanked tag `0.6` (proves the test would actually fail if the pin regressed back to a yanked tag).
  - Fail-loud when `OLLAMA_IMAGE` is unset (no `t.Skip()` bailout).

## Acceptance scenarios

```gherkin
Scenario: SCN-BUG-001-001 Pinned ollama tag is currently published on Docker Hub
  Given config/generated/test.env carries OLLAMA_IMAGE pointing at ollama/ollama:<tag>
  When the regression test HEADs https://hub.docker.com/v2/repositories/ollama/ollama/tags/<tag>
  Then the response is HTTP 200
  And the test stack can pull and start the ollama service

Scenario: SCN-BUG-001-002 Adversarial — guard rejects yanked tags
  Given the regression test inspects the known-yanked tag "0.6"
  When the same code path HEADs the Docker Hub tags API for that tag
  Then the response is HTTP 404
  And the regression test reports a tag-availability failure
  And the same code path applied to the live OLLAMA_IMAGE pin would fail loudly if it ever pointed at a yanked tag
```

## Adversarial regression

`tests/integration/ollama_image_availability_test.go::TestOllamaImagePinIsPublished_LiveTag` reads the live `OLLAMA_IMAGE` from `config/generated/test.env` and HEADs the Docker Hub tags API. Pre-fix this test would have failed against `ollama/ollama:0.6` (HTTP 404) but passes against the new `0.23.2` pin (HTTP 200).

`TestOllamaImagePinIsPublished_AdversarialYankedTag` proves the test is not tautological by exercising the same code path against the synthetic input `ollama/ollama:0.6` and asserting it returns HTTP 404 — i.e. the test would have caught this exact bug at the time it was introduced.

## Out of scope

- Pinning by content digest (deferred to spec 043 OQ-D2; tracked separately).
- Choosing a new production Ollama version policy (this fix tracks the latest stable release that exists today; ongoing version selection is out of scope).
- Re-running the full ollama happy-path test (`-tags=e2e_ollama`) which is the operator-driven cold-pull lane gated by `SMACKEREL_TEST_OLLAMA=1`. The bug fix unblocks it; live cold-pull verification is documented as an operator-side acceptance step.
