# BUG-002 Scopes

## Scope 01 — Replace ollama `wget` healthcheck + add adversarial binary-presence guard

**Status:** Done

### Change Boundary

Allowed:
- `docker-compose.yml` (line 182 — replace `wget`-based `CMD-SHELL` healthcheck with `["CMD", "ollama", "list"]`)
- `deploy/compose.deploy.yml` (line 199 — same replacement; mirror per BUG-001 design OQ-BUG-001-A)
- `tests/integration/ollama_healthcheck_test.go` (NEW — live + adversarial healthcheck-binary guard, build tag `integration`)
- `specs/043-ollama-test-infrastructure/bugs/BUG-002-ollama-healthcheck-uses-missing-wget/{spec,design,scopes,uservalidation,report,scenario-manifest,state}.{md,json}` (new — bug packet)

Forbidden:
- Any change to `internal/`, `cmd/`, `ml/`, `scripts/`, `web/`, `Dockerfile` (no consumer code reads the healthcheck command — this fix is compose-only)
- Any change to `config/smackerel.yaml`, `config/generated/*.env`, `deploy/contract.yaml` (the healthcheck command is a compose-side concern, not an SST key — it does not flow through `config generate`)
- Any change to `internal/deploy/compose_ollama_contract_test.go` (its scope is image hoist + profile gate + volume indirection; the new healthcheck guard lives under `tests/integration/`)
- Any change to `internal/config/sst_grep_guard_test.go` (no new SST violations introduced)
- Any change to other spec or bug folders
- Any change to the parent `specs/043-ollama-test-infrastructure/{spec,design,scopes,report,uservalidation,state}.{md,json}` (parent spec is `done`; this bug operates within Scope 01 territory). Optional single `executionHistory` audit-trail entry on the parent state.json is permitted.

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: BUG-002 — Ollama healthcheck must call a binary that exists in the pinned image

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

### Implementation Plan

1. Verify which command works inside `ollama/ollama:0.23.2` (`docker exec smackerel-test-ollama-1 ollama list` → exit 0; with bad `OLLAMA_HOST` → exit 1; `wget`/`curl` → exit 127 not-found).
2. Replace the healthcheck `test:` line in `docker-compose.yml` line 182 from the `CMD-SHELL` `wget` form to `["CMD", "ollama", "list"]`. Preserve `interval`/`timeout`/`retries`/`start_period`. Add inline comment citing BUG-002 audit trail.
3. Mirror the change in `deploy/compose.deploy.yml` line 199.
4. Create `tests/integration/ollama_healthcheck_test.go` (build tag `integration`):
   - Helper `assertOllamaHealthcheckUsesInImageBinary(yamlBytes []byte) error` — parses YAML, locates `services.ollama.healthcheck.test`, extracts the first command token (handling both `CMD` and `CMD-SHELL` forms), asserts first token is in allowlist `{ollama, /usr/bin/ollama, /bin/ollama}` AND not in forbidden set `{wget, curl, /usr/bin/wget, /usr/bin/curl, /bin/wget, /bin/curl}`. Errors name the offending binary AND the image `ollama/ollama:0.23.2`.
   - `TestOllamaHealthcheck_LiveFiles` — runs the helper against both compose files; fail-loud on any violation.
   - Three adversarial sub-tests: synthetic compose with `wget` (CMD form); synthetic compose with `curl` (CMD form); synthetic compose with `CMD-SHELL` `wget` wrapper. All three assert rejection.
   - NO `t.Skip()` calls.
5. Run static gates (`./smackerel.sh check`, `./smackerel.sh test unit`).
6. Run targeted regression test (`go test -count=1 -tags=integration -v -run 'TestOllamaHealthcheck' ./tests/integration/`).
7. Live-stack verification: bring the test stack up against the fixed compose, confirm ollama container reports `(healthy)`.
8. Run full integration lane (`./smackerel.sh test integration`).
9. Run artifact-lint on the bug folder.

### Test Plan

| ID | Type | File | Scenario | Notes |
|----|------|------|----------|-------|
| T01-01 | integration (compose contract — NEW) | `tests/integration/ollama_healthcheck_test.go::TestOllamaHealthcheck_LiveFiles` | SCN-BUG-002-001 | NEW. Parses both `docker-compose.yml` and `deploy/compose.deploy.yml`, asserts `services.ollama.healthcheck.test` first token is in the allowlist of in-image binaries. Fail-loud (no `t.Skip()`). |
| T01-02 | integration (adversarial wget — NEW) | `tests/integration/ollama_healthcheck_test.go::TestOllamaHealthcheck_AdversarialMissingBinary` | SCN-BUG-002-002 | NEW. Synthetic compose with `wget` in healthcheck. Asserts helper rejects with error mentioning `wget` and `ollama/ollama:0.23.2`. |
| T01-03 | integration (adversarial curl — NEW) | `tests/integration/ollama_healthcheck_test.go::TestOllamaHealthcheck_AdversarialMissingBinaryCurl` | SCN-BUG-002-002 | NEW. Same with `curl` (proves forbidden-binary set is enforced for both). |
| T01-04 | integration (adversarial CMD-SHELL wrapper — NEW) | `tests/integration/ollama_healthcheck_test.go::TestOllamaHealthcheck_AdversarialCMDShellWrappedWget` | SCN-BUG-002-002 | NEW. Synthetic compose with `["CMD-SHELL", "wget …"]` (the original broken form). Asserts helper still rejects — proves validator handles both `CMD` and `CMD-SHELL` forms. |
| T01-05 | live-stack (E2E) | `./smackerel.sh test integration` | SCN-BUG-002-001 | Full integration runner. Pre-fix: exited 124 at `docker compose up -d --wait`. Post-fix: stack comes up healthy and Go integration tests proceed. |
| T01-06 | unit (no regression) | `./smackerel.sh test unit` (Go + Python) | n/a | No source/compose change should regress unit lane. |
| T01-07 | static (config sync) | `./smackerel.sh check` | n/a | Config + env-file drift guard + scenario-lint must remain green. |

### Definition of Done — 3-Part Validation

- [x] **Root cause confirmed and documented:** `ollama/ollama:0.23.2` does NOT ship `wget` or `curl` (`docker exec` returns exit 127 with `executable file not found in $PATH`). The image DOES ship `/usr/bin/ollama` and `ollama list` is a real liveness signal (exit 0 when daemon up, exit 1 when daemon unreachable). Documented in `spec.md` § Root cause and `design.md` § Current Truth.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ docker exec smackerel-test-ollama-1 wget --no-verbose --tries=1 --spider http://localhost:11434/api/tags 2>&1; echo "EXIT=$?"
      OCI runtime exec failed: exec failed: unable to start container process: exec: "wget": executable file not found in $PATH
      EXIT=127

      $ docker exec smackerel-test-ollama-1 curl -sS http://localhost:11434/api/tags 2>&1; echo "EXIT=$?"
      OCI runtime exec failed: exec failed: unable to start container process: exec: "curl": executable file not found in $PATH
      EXIT=127

      $ docker exec smackerel-test-ollama-1 which ollama
      /usr/bin/ollama

      $ docker exec smackerel-test-ollama-1 ollama list 2>&1; echo "EXIT=$?"
      NAME    ID    SIZE    MODIFIED
      EXIT=0

      $ docker exec -e OLLAMA_HOST=http://127.0.0.1:11999 smackerel-test-ollama-1 ollama list 2>&1; echo "EXIT=$?"
      Error: could not connect to ollama server, run 'ollama serve' to start it
      EXIT=1
      ```

- [x] **Fix implemented:** Healthcheck command replaced in both `docker-compose.yml` and `deploy/compose.deploy.yml`. Switched from `CMD-SHELL` `wget …` form to `CMD` `ollama list` form. `interval`/`timeout`/`retries`/`start_period` preserved.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ grep -nA1 -B1 'ollama, list' docker-compose.yml deploy/compose.deploy.yml
      docker-compose.yml-181-    healthcheck:
      docker-compose.yml:182:      test: ["CMD", "ollama", "list"]
      docker-compose.yml-183-      interval: 10s
      --
      deploy/compose.deploy.yml-198-    healthcheck:
      deploy/compose.deploy.yml:199:      test: ["CMD", "ollama", "list"]
      deploy/compose.deploy.yml-200-      interval: 10s

      $ grep -n 'wget.*OLLAMA_CONTAINER_PORT\|wget.*api/tags' docker-compose.yml deploy/compose.deploy.yml || echo "ZERO wget-based ollama healthchecks remaining"
      ZERO wget-based ollama healthchecks remaining
      ```

- [x] **Pre-fix regression test FAILS:** `tests/integration/ollama_healthcheck_test.go::TestOllamaHealthcheck_AdversarialMissingBinary` exercises the same code path against a synthetic compose snippet whose healthcheck uses the original `wget` form. The helper MUST reject it with an error naming `wget` and `ollama/ollama:0.23.2`. Pre-fix the production `docker-compose.yml` would have produced the same rejection — proving the test would have caught BUG-002 at the time it was introduced.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ go test -count=1 -tags=integration -v -run 'TestOllamaHealthcheck_AdversarialMissingBinary$' ./tests/integration/
      === RUN   TestOllamaHealthcheck_AdversarialMissingBinary
          ollama_healthcheck_test.go:309: adversarial OK: synthetic compose with `wget` healthcheck rejected with: contract violation: services.ollama.healthcheck first token "wget" is in the forbidden-binaries set (binary not present in ollama/ollama:0.23.2 image — `docker exec … wget` returns exit 127 'executable file not found in $PATH'); use `ollama list` instead
      --- PASS: TestOllamaHealthcheck_AdversarialMissingBinary (0.00s)
      PASS
      ok      github.com/smackerel/smackerel/tests/integration        0.029s
      ```

- [x] **Adversarial regression case exists and would fail if the bug returned:** `TestOllamaHealthcheck_AdversarialMissingBinary` + `TestOllamaHealthcheck_AdversarialMissingBinaryCurl` + `TestOllamaHealthcheck_AdversarialCMDShellWrappedWget` together prove the test is non-tautological. Same `assertOllamaHealthcheckUsesInImageBinary` helper, three different inputs (`wget` CMD-form, `curl` CMD-form, `wget` CMD-SHELL-form) — all three are rejected with explicit error messages naming the offending binary and the image. If a future commit regresses either compose file to `wget`/`curl` (or any other not-in-image binary added to the forbidden set), `TestOllamaHealthcheck_LiveFiles` would fail with the same shape of error the adversarial sub-cases assert.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ go test -count=1 -tags=integration -v -run 'TestOllamaHealthcheck_Adversarial' ./tests/integration/
      === RUN   TestOllamaHealthcheck_AdversarialMissingBinary
          ollama_healthcheck_test.go:309: adversarial OK: synthetic compose with `wget` healthcheck rejected with: contract violation: services.ollama.healthcheck first token "wget" is in the forbidden-binaries set (binary not present in ollama/ollama:0.23.2 image — `docker exec … wget` returns exit 127 'executable file not found in $PATH'); use `ollama list` instead
      --- PASS: TestOllamaHealthcheck_AdversarialMissingBinary (0.00s)
      === RUN   TestOllamaHealthcheck_AdversarialMissingBinaryCurl
          ollama_healthcheck_test.go:334: adversarial OK: synthetic compose with `curl` healthcheck rejected with: contract violation: services.ollama.healthcheck first token "curl" is in the forbidden-binaries set (binary not present in ollama/ollama:0.23.2 image — `docker exec … curl` returns exit 127 'executable file not found in $PATH'); use `ollama list` instead
      --- PASS: TestOllamaHealthcheck_AdversarialMissingBinaryCurl (0.00s)
      === RUN   TestOllamaHealthcheck_AdversarialCMDShellWrappedWget
          ollama_healthcheck_test.go:362: adversarial OK: synthetic compose with `CMD-SHELL` `wget …` wrapper rejected with: contract violation: services.ollama.healthcheck first token "wget" (extracted from CMD-SHELL form) is in the forbidden-binaries set (binary not present in ollama/ollama:0.23.2 image — `docker exec … wget` returns exit 127 'executable file not found in $PATH'); use `ollama list` instead
      --- PASS: TestOllamaHealthcheck_AdversarialCMDShellWrappedWget (0.00s)
      PASS
      ok      github.com/smackerel/smackerel/tests/integration        0.054s
      ```

- [x] **Post-fix regression test PASSES:** `TestOllamaHealthcheck_LiveFiles` parses both production compose files and confirms each ollama healthcheck first-token is in the allowlist. Live-stack confirmation: `docker ps --filter name=smackerel-test-ollama` shows `Up <duration> (healthy)`.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ go test -count=1 -tags=integration -v -run 'TestOllamaHealthcheck_LiveFiles' ./tests/integration/
      === RUN   TestOllamaHealthcheck_LiveFiles
          ollama_healthcheck_test.go:279: contract OK: docker-compose.yml ollama healthcheck first-token "ollama" is in the in-image-binary allowlist
          ollama_healthcheck_test.go:279: contract OK: deploy/compose.deploy.yml ollama healthcheck first-token "ollama" is in the in-image-binary allowlist
      --- PASS: TestOllamaHealthcheck_LiveFiles (0.00s)
      PASS
      ok      github.com/smackerel/smackerel/tests/integration        0.054s

      $ docker ps --filter name=smackerel-test-ollama --format '{{.Names}}\t{{.Status}}'
      smackerel-test-ollama-1	Up 23 seconds (healthy)
      ```

- [x] **Regression tests contain no silent-pass bailout patterns:** scan confirms zero `t.Skip*` calls in the new test file; fail-loud paths use `t.Fatalf`/`t.Errorf`.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ grep -nE 't\.(Skip|SkipNow|Skipf)\(' tests/integration/ollama_healthcheck_test.go || echo "ZERO t.Skip-family calls"
      ZERO t.Skip-family calls
      ```

- [x] **All existing tests pass (no regressions):** `./smackerel.sh check`, `./smackerel.sh test unit` (Go + Python), and the full `./smackerel.sh test integration` lane are green after the fix.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ ./smackerel.sh check
      Config is in sync with SST
      env_file drift guard: OK
      scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
      scenarios registered: 5, rejected: 0
      scenario-lint: OK
      EXIT=0

      $ ./smackerel.sh test unit 2>&1 | grep -cE '^ok '
      72
      $ ./smackerel.sh test unit 2>&1 | grep -E '^FAIL' || echo "ZERO FAIL lines"
      ZERO FAIL lines
      $ ./smackerel.sh test unit 2>&1 | tail -2
      417 passed in 14.78s
      EXIT=0

      $ ./smackerel.sh test integration
      [+] Running 9/9
       ✔ Network smackerel-test_default             Created                     0.6s
       ✔ Volume "smackerel-test-postgres-data"      Created                     0.0s
       ✔ Volume "smackerel-test-nats-data"          Created                     0.0s
       ✔ Volume "smackerel-test-ollama-data"        Created                     0.0s
       ✔ Container smackerel-test-ollama-1          Healthy                    12.6s
       ✔ Container smackerel-test-nats-1            Healthy                    12.6s
       ✔ Container smackerel-test-postgres-1        Healthy                    12.6s
       ✔ Container smackerel-test-smackerel-ml-1    Healthy                    16.9s
       ✔ Container smackerel-test-smackerel-core-1  Healthy                    17.4s
      … Go integration tests run: 159 PASS / 0 FAIL across all packages …
      ok      github.com/smackerel/smackerel/tests/integration/drive  7.834s
      EXIT=0
      ```

- [x] **Bug marked as Fixed in spec.md** with audit-trail commit SHA in `state.json`.
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ grep -n '^> \*\*Status:\*\*' specs/043-ollama-test-infrastructure/bugs/BUG-002-ollama-healthcheck-uses-missing-wget/spec.md
      11:> **Status:** Fixed
      ```
