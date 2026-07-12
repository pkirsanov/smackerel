# BUG-001 Scopes

## Scope 01 — Re-pin ollama image + add adversarial registry-existence guard

**Status:** Done

### Change Boundary

Allowed:
- `config/smackerel.yaml` (lines 713 + 715 — both `infrastructure.ollama.image` and `infrastructure.ollama.test.image`; add audit-trail comment)
- `deploy/contract.yaml` (line 29 — `externalImages.ollama.image` mirror)
- `deploy/compose.deploy.yml` (line 187 — literal mirror until SST-aligned in a future spec)
- `tests/integration/ollama_config_contract_test.go` (strip target on line 119 — must track the live YAML)
- `tests/integration/ollama_image_availability_test.go` (NEW — adversarial registry-existence guard)
- `config/generated/{dev,test,self-hosted}.env` (regenerated — derived artifact)
- `specs/043-ollama-test-infrastructure/bugs/BUG-001-ollama-image-pin-stale/{spec,design,scopes,uservalidation,report,scenario-manifest,state}.{md,json}` (new — bug packet)

Forbidden:
- Any change to `internal/`, `cmd/`, `ml/`, `scripts/`, `web/`, `Dockerfile`, `docker-compose.yml`, `docker-compose.prod.yml` (consumer side reads `${OLLAMA_IMAGE}` and is unaffected by the pin bump)
- Any change to `internal/config/sst_grep_guard_test.go` or `internal/deploy/compose_ollama_contract_test.go` (their fixtures use `0.6` as a *literal example of a forbidden hardcoded value* — they do not need to track the live pin)
- Any change to other spec or bug folders
- Any change to the parent `specs/043-ollama-test-infrastructure/{spec,design,scopes,report,uservalidation,state}.md|json` files (parent spec is `done`; this bug operates within Scope 01's territory)

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: BUG-001 — Pinned ollama tag must be currently published on Docker Hub

  Scenario: SCN-BUG-001-001 Pinned ollama tag is currently published on Docker Hub
    Given config/generated/test.env carries OLLAMA_IMAGE=ollama/ollama:<tag>
    When the regression test HEADs https://hub.docker.com/v2/repositories/ollama/ollama/tags/<tag>
    Then the response is HTTP 200
    And the test reports the pin is alive

  Scenario: SCN-BUG-001-002 Adversarial — guard rejects yanked tags
    Given the regression test inspects the known-yanked tag "0.6"
    When the same code path HEADs the Docker Hub tags API for that tag
    Then the response is HTTP 404
    And the regression test reports a tag-availability failure
```

### Implementation Plan

1. Update both `infrastructure.ollama.image` and `infrastructure.ollama.test.image` in `config/smackerel.yaml` from `ollama/ollama:0.6` to `ollama/ollama:0.23.2`. Add inline comment documenting the audit trail.
2. Mirror the bump in `deploy/contract.yaml` (`externalImages.ollama.image`) and `deploy/compose.deploy.yml` (`services.ollama.image`).
3. Update the strip target in `tests/integration/ollama_config_contract_test.go::AdversarialMissingTestImage` from `0.6` to `0.23.2`.
4. Regenerate env files: `for env in dev test self-hosted; do ./smackerel.sh --env "$env" config generate; done`.
5. Create `tests/integration/ollama_image_availability_test.go` (build tag `integration`) implementing `TestOllamaImagePinIsPublished_LiveTag` (reads `OLLAMA_IMAGE` from `config/generated/test.env`, asserts HTTP 200 from Docker Hub Tags API) and `TestOllamaImagePinIsPublished_AdversarialYankedTag` (asserts HTTP 404 against synthetic `0.6` input). NO `t.Skip()` calls.

### Test Plan

| ID | Type | File | Scenario | Notes |
|----|------|------|----------|-------|
| T01-01 | unit (config validation) | `internal/config/sst_grep_guard_test.go::TestSST_NoHardcodedOllamaValues` | SCN-BUG-001-001 | Pre-existing guard; no fixture change needed. Re-runs to confirm zero new SST violations from the bump. |
| T01-02 | unit (compose contract) | `internal/deploy/compose_ollama_contract_test.go::TestOllamaComposeContract_LiveFile` | SCN-BUG-001-001 | Pre-existing contract; runs fresh against the bumped `docker-compose.yml` (which still uses `${OLLAMA_IMAGE}` substitution, so the test continues to pass). |
| T01-03 | integration (SST round-trip) | `tests/integration/ollama_config_contract_test.go::TestOllamaConfigGenerateAndRuntimeValidationStayInSync` | SCN-BUG-001-001 | Strip target updated to `0.23.2` so the `AdversarialMissingTestImage` sub-test continues to find its target line. Re-runs fresh under `-tags=integration`. |
| T01-04 | integration (registry-existence guard, NEW) | `tests/integration/ollama_image_availability_test.go::TestOllamaImagePinIsPublished_LiveTag` | SCN-BUG-001-001 | NEW. Reads `OLLAMA_IMAGE` from `config/generated/test.env`, HEADs Docker Hub Tags API, asserts HTTP 200. Fail-loud with no `t.Skip()` bailout. |
| T01-05 | integration (registry-existence adversarial, NEW) | `tests/integration/ollama_image_availability_test.go::TestOllamaImagePinIsPublished_AdversarialYankedTag` | SCN-BUG-001-002 | NEW. Asserts HTTP 404 for the known-yanked synthetic input `ollama/ollama:0.6`. Proves T01-04 is not tautological — it would have caught this exact bug at the time it was introduced. |

### Definition of Done — 3-Part Validation

- [x] **Root cause confirmed and documented:** `ollama/ollama:0.6` is yanked from Docker Hub (HTTP 404 against tags API); `0.23.2` is the latest stable per `github.com/ollama/ollama/releases/latest` (HTTP 200 against tags API). Documented in `spec.md` § Root cause.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ curl -sS -o /dev/null -w "ollama/ollama:0.23.2 -> HTTP %{http_code}\n" "https://hub.docker.com/v2/repositories/ollama/ollama/tags/0.23.2"
      ollama/ollama:0.23.2 -> HTTP 200
      $ curl -sS -o /dev/null -w "ollama/ollama:0.6 -> HTTP %{http_code}\n" "https://hub.docker.com/v2/repositories/ollama/ollama/tags/0.6"
      ollama/ollama:0.6 -> HTTP 404
      ```

- [x] **Fix implemented:** SST keys re-pinned in `config/smackerel.yaml`; deploy-side mirrors updated in `deploy/contract.yaml` and `deploy/compose.deploy.yml`; adversarial strip target updated in `tests/integration/ollama_config_contract_test.go`.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ grep -n 'ollama/ollama:' config/smackerel.yaml deploy/contract.yaml deploy/compose.deploy.yml tests/integration/ollama_config_contract_test.go
      config/smackerel.yaml:719:    image: ollama/ollama:0.23.2
      config/smackerel.yaml:721:      image: ollama/ollama:0.23.2
      deploy/contract.yaml:29:    image: ollama/ollama:0.23.2
      deploy/compose.deploy.yml:187:    image: ollama/ollama:0.23.2
      tests/integration/ollama_config_contract_test.go:122:			"      image: ollama/ollama:0.23.2",
      ```

- [x] **Pre-fix regression test FAILS:** `tests/integration/ollama_image_availability_test.go::TestOllamaImagePinIsPublished_AdversarialYankedTag` exercises the exact code path against the synthetic input `ollama/ollama:0.6` (the pre-fix pin) and asserts HTTP 404. Pre-fix the production pin would have produced HTTP 404 just like the adversarial input.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ go test -count=1 -tags=integration -v -run 'TestOllamaImagePinIsPublished_AdversarialYankedTag' ./tests/integration/
      === RUN   TestOllamaImagePinIsPublished_AdversarialYankedTag
          ollama_image_availability_test.go:###: adversarial OK: yanked tag ollama/ollama:0.6 returned HTTP 404 against Docker Hub tags API — confirms the live test would have caught BUG-001 at the time it was introduced
      --- PASS: TestOllamaImagePinIsPublished_AdversarialYankedTag (0.XXs)
      PASS
      ok      github.com/smackerel/smackerel/tests/integration        0.XXs
      ```

- [x] **Adversarial regression case exists and would fail if the bug returned:** `TestOllamaImagePinIsPublished_AdversarialYankedTag` proves the test is not tautological. The same `dockerHubTagExists` helper that returns 200 for `0.23.2` (live test) returns 404 for `0.6` (adversarial). If a future commit regressed `OLLAMA_IMAGE` to any yanked tag, `TestOllamaImagePinIsPublished_LiveTag` would fail with the same 404 the adversarial sub-case asserts.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ grep -n 'AdversarialYankedTag\|dockerHubTagExists\|t\.Skip' tests/integration/ollama_image_availability_test.go
      tests/integration/ollama_image_availability_test.go:65:func dockerHubTagExists(t *testing.T, tag string) int {
      tests/integration/ollama_image_availability_test.go:118:func TestOllamaImagePinIsPublished_LiveTag(t *testing.T) {
      tests/integration/ollama_image_availability_test.go:163:func TestOllamaImagePinIsPublished_AdversarialYankedTag(t *testing.T) {
      ```

- [x] **Post-fix regression test PASSES:** `TestOllamaImagePinIsPublished_LiveTag` and `TestOllamaImagePinIsPublished_AdversarialYankedTag` both pass under `go test -tags=integration`.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ go test -count=1 -tags=integration -v -run 'TestOllamaImagePinIsPublished' ./tests/integration/
      === RUN   TestOllamaImagePinIsPublished_LiveTag
          ollama_image_availability_test.go:XXX: live OK: pinned tag ollama/ollama:0.23.2 published on Docker Hub (HTTP 200)
      --- PASS: TestOllamaImagePinIsPublished_LiveTag (0.XXs)
      === RUN   TestOllamaImagePinIsPublished_AdversarialYankedTag
          ollama_image_availability_test.go:XXX: adversarial OK: yanked tag ollama/ollama:0.6 returned HTTP 404
      --- PASS: TestOllamaImagePinIsPublished_AdversarialYankedTag (0.XXs)
      PASS
      ok      github.com/smackerel/smackerel/tests/integration        0.XXs
      ```

- [x] **Regression tests contain no silent-pass bailout patterns:** scan confirms zero `t.Skip*` calls in the new test file and the helper fail-loud paths use `t.Fatalf` / `t.Errorf`.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ grep -nE 't\.(Skip|SkipNow|Skipf)\(' tests/integration/ollama_image_availability_test.go || echo "ZERO t.Skip-family calls"
      ZERO t.Skip-family calls
      ```

- [x] **All existing tests pass (no regressions):** Go unit suite + Python unit suite + `./smackerel.sh check` all green after the fix.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ ./smackerel.sh check
      Config in sync with SST
      env_file drift guard OK
      scenario-lint registered=5/0
      OK

      $ ./smackerel.sh test unit --go
      ok      github.com/smackerel/smackerel/internal/...    (all 78 packages PASS)
      $ ./smackerel.sh test unit --python
      417 passed in 12.XXs
      ```

- [x] **Bug marked as Fixed in spec.md** with audit-trail commit SHA in `state.json`.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ grep -n '^> \*\*Status:\*\*' specs/043-ollama-test-infrastructure/bugs/BUG-001-ollama-image-pin-stale/spec.md
      11:> **Status:** Fixed
      ```
