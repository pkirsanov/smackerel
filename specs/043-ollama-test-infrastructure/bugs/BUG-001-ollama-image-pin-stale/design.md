# BUG-001 Design — Re-pin ollama image to currently-published tag + add adversarial registry guard

## Current Truth

- `config/smackerel.yaml` lines 713 and 715 pin both `infrastructure.ollama.image` and `infrastructure.ollama.test.image` to `ollama/ollama:0.6`.
- That literal flows through `scripts/commands/config.sh` (`required_value infrastructure.ollama.test.image` for the test env, `required_value infrastructure.ollama.image` otherwise) into `config/generated/<env>.env` as `OLLAMA_IMAGE=ollama/ollama:0.6`.
- `docker-compose.yml` line 171 substitutes `${OLLAMA_IMAGE}` for the ollama service.
- `deploy/compose.deploy.yml` line 187 ships a literal `ollama/ollama:0.6` (the deploy-side compose does not currently substitute from `OLLAMA_IMAGE`; the deploy bundle is a separate artifact per build-once deploy-many).
- `deploy/contract.yaml` line 29 lists `ollama/ollama:0.6` under `externalImages` (the build/deploy contract surface).
- `tests/integration/ollama_config_contract_test.go` line 119 uses `      image: ollama/ollama:0.6` as the strip target for the `AdversarialMissingTestImage` adversarial regression.
- The `internal/config/sst_grep_guard_test.go` and `internal/deploy/compose_ollama_contract_test.go` files contain `ollama/ollama:0.6` as in-test fixtures (NOT live SST). They assert that hardcoded literal images get rejected, regardless of which tag the literal happens to be — they do not need to track the live pin.

## Root cause

Upstream Docker Hub has yanked the `0.6` manifest. The pin was a static literal with zero registry-liveness coverage, so the regression slipped past every Smackerel test surface and only surfaced when the integration test stack tried to pull the image.

## Change shape

1. **Re-pin SST keys** in `config/smackerel.yaml` from `ollama/ollama:0.6` → `ollama/ollama:0.23.2` (latest stable per [`github.com/ollama/ollama/releases/latest`](https://github.com/ollama/ollama/releases/latest); verified HTTP 200 against the Docker Hub Tags API at fix time). Add an inline comment documenting the audit trail and the new adversarial regression test.
2. **Mirror the bump** in `deploy/contract.yaml` (`externalImages.ollama.image`) and `deploy/compose.deploy.yml` (`services.ollama.image`) so the deploy-side surface stays consistent with the SST source.
3. **Update the adversarial strip target** in `tests/integration/ollama_config_contract_test.go::AdversarialMissingTestImage` from `      image: ollama/ollama:0.6` → `      image: ollama/ollama:0.23.2` so the strip-and-rerun adversarial assertion continues to find its target line in the live YAML.
4. **Regenerate** `config/generated/<env>.env` for `dev`, `test`, `self-hosted` via `for env in dev test self-hosted; do ./smackerel.sh --env "$env" config generate; done`.
5. **New adversarial regression test** `tests/integration/ollama_image_availability_test.go` (build tag `integration`):
   - `TestOllamaImagePinIsPublished_LiveTag`: reads `OLLAMA_IMAGE` from `config/generated/test.env`, splits at the colon, HEADs `https://hub.docker.com/v2/repositories/ollama/ollama/tags/<tag>`, asserts HTTP 200. Fail-loud (`t.Fatalf`) when `OLLAMA_IMAGE` is unset, when `OLLAMA_IMAGE` does not start with `ollama/ollama:`, when the HTTP response is non-200, or when the network is unreachable. **No `t.Skip()` of any kind.**
   - `TestOllamaImagePinIsPublished_AdversarialYankedTag`: applies the same code path to the synthetic input `ollama/ollama:0.6` and asserts the response is HTTP 404 (proves the registry-existence check is real and would have caught the bug at the time it was introduced).
   - The two tests share a single `dockerHubTagExists(tag string) (int, error)` helper so the adversarial proves the live test is not tautological.

## Why the SST grep guard is unaffected

`internal/config/sst_grep_guard_test.go` enforces that the literal `ollama/ollama:` does not appear in production source paths (`internal/`, `cmd/`, `ml/app/`, `scripts/`, `Dockerfile`, `docker-compose.yml`, `docker-compose.prod.yml`). It allowlists `*_test.go`, `*_test.py`, `tests/`, and ignores `config/`, `deploy/`, `docs/`, `specs/`. The bump only touches `config/smackerel.yaml` (allowlisted) and `deploy/*` (not in scan roots). The new test file lives under `tests/integration/` (allowlisted via the `tests/` directory carve-out and the `_test.go` suffix). Zero new SST violations.

## Why we do NOT pin by content digest in this fix

OQ-D2 from spec 043 design.md explicitly defers digest pinning to a future hardening spec. Re-pinning to a tag-only reference is the minimum-blast-radius fix for this P0 outage. The adversarial registry-existence test still detects yanked tags within seconds of an integration run, which closes the immediate operational gap. A follow-up spec can layer digest pinning on top once the version-selection policy stabilizes.

## Why we do NOT make `deploy/compose.deploy.yml` use `${OLLAMA_IMAGE}` substitution in this fix

The deploy-side compose ships as part of the deploy adapter bundle per the Build-Once Deploy-Many contract. Wiring it to `OLLAMA_IMAGE` would require coordinating env-var propagation across the deploy adapter, which is out of scope for a P0 unblock fix. The literal in `deploy/compose.deploy.yml` is updated in lockstep with `config/smackerel.yaml` for now; SST-aligning the deploy compose can be tracked as a follow-up (see Open Questions).

## Backward compatibility

- `OLLAMA_IMAGE` env-var contract: unchanged. Only the right-hand-side value moves from `ollama/ollama:0.6` → `ollama/ollama:0.23.2`.
- All consumer code paths (Go core, Python sidecar, scripts, compose) read the env var, not the literal — they pick up the new pin transparently on next `config generate`.
- The deterministic test model `qwen2.5:0.5b-instruct` is unchanged; both `0.6` and `0.23.x` of the Ollama daemon support the same pull/run protocol used by `scripts/commands/ollama-test-pull.sh`.
- The new regression test is `//go:build integration`, so it does not change the default `./smackerel.sh test unit` surface.

## Rejected alternatives

- **Pin to `latest`.** Rejected — explicitly forbidden by the deploy contract (`externalImages` are pinned for reproducibility) and by spec 043 design (the test environment must be deterministic per FR-OLLAMA-006).
- **Skip the test when network is unreachable.** Rejected — would silently mask the exact failure mode this bug exists to catch. The test must fail-loud when it cannot prove the pin is alive.
- **Pin to a major-only tag like `0`.** Rejected — Ollama appears to have moved away from short-form tags (the yank itself is evidence). Pin to a full semver release that we have proven to exist.
- **Mirror the image into a Smackerel-controlled registry.** Rejected — adds infra complexity and ongoing mirroring cost for a P0 unblock; reconsider when digest pinning is added.

## Open questions (deferred — NOT blocking this fix)

- **OQ-BUG-001-A:** Should `deploy/compose.deploy.yml` substitute `${OLLAMA_IMAGE}` from a deploy-side env file instead of carrying a literal? Tracked as a follow-up alongside OQ-D2 (digest pinning).
- **OQ-BUG-001-B:** Should the adversarial registry-existence test cache the HTTP HEAD result for some short TTL to avoid hammering the Docker Hub Tags API on every integration run? At single-tag-per-test-run cardinality this is not a real cost, but if we add per-image checks across more pinned externals (postgres, nats), a small TTL cache becomes worthwhile.
