# Scopes: BUG-043-003 — ML OCR fallback reads OLLAMA_URL with silent default, bypassing the spec 043 ENABLE_OLLAMA SST gate

## Scope 1: ENABLE_OLLAMA fail-loud gate at OCR fallback site + adversarial RED→GREEN coverage

**Status:** Done

**Files:**
- [ml/app/ocr.py](../../../../ml/app/ocr.py) (lines 244–256: replace silent-default `os.environ.get("OLLAMA_URL", "")` with fail-loud `os.environ["ENABLE_OLLAMA"]` gate; remove positional `ollama_url` arg from `extract_text_ollama` call)
- [ml/tests/test_ocr.py](../../../../ml/tests/test_ocr.py) (new `TestEnableOllamaFailLoudGating` class with 5 test methods covering truthy / falsy / unset / invalid / sufficient-tesseract guard rail)
- [ml/tests/test_keep.py](../../../../ml/tests/test_keep.py) (update 2 pre-existing tests `test_ollama_fallback` and `test_both_ocr_fail_returns_ok` to set `ENABLE_OLLAMA` explicitly in their `patch.dict(os.environ, ...)` calls)

### Use Cases

```gherkin
Feature: ML OCR fallback gates Ollama via the spec 043 ENABLE_OLLAMA SST flag, fail-loud
  Scenario: SCN-043-003-A — handle_ocr_request reads ENABLE_OLLAMA fail-loud (no positional default)
    Given ml/app/ocr.py handle_ocr_request reaches the slow path (Tesseract output < MIN_TESSERACT_CHARS)
    When the function consults the OCR-fallback gate
    Then it reads `os.environ["ENABLE_OLLAMA"]` (raises KeyError when missing)
    And it does NOT call `os.environ.get("KEY", "default")` or `os.getenv("KEY", "default")` for ENABLE_OLLAMA or any other gate variable

  Scenario: SCN-043-003-B — ENABLE_OLLAMA truthy invokes Ollama fallback
    Given ENABLE_OLLAMA is set to one of "true"/"1"/"yes"/"on" (case-insensitive, whitespace-trimmed)
    And Tesseract output is shorter than MIN_TESSERACT_CHARS
    When handle_ocr_request runs
    Then extract_text_ollama is invoked
    And the returned ocr_engine field equals "ollama" if Ollama returned a longer result than Tesseract

  Scenario: SCN-043-003-C — ENABLE_OLLAMA falsy skips Ollama fallback
    Given ENABLE_OLLAMA is set to one of "false"/"0"/"no"/"off"/"" (case-insensitive, whitespace-trimmed)
    And Tesseract output is shorter than MIN_TESSERACT_CHARS
    When handle_ocr_request runs
    Then extract_text_ollama is NOT invoked
    And the returned ocr_engine field equals "tesseract" with the original Tesseract result text

  Scenario: SCN-043-003-D — ENABLE_OLLAMA missing raises KeyError on the slow path
    Given ENABLE_OLLAMA is not present in the environment
    And Tesseract output is shorter than MIN_TESSERACT_CHARS
    When handle_ocr_request runs
    Then it raises KeyError naming "ENABLE_OLLAMA"

  Scenario: SCN-043-003-E — ENABLE_OLLAMA invalid value raises RuntimeError naming Gate G028
    Given ENABLE_OLLAMA is set to a non-canonical token (e.g. "maybe", "enabled", "disabled")
    And Tesseract output is shorter than MIN_TESSERACT_CHARS
    When handle_ocr_request runs
    Then it raises RuntimeError mentioning "ENABLE_OLLAMA must be exactly one of" AND naming the invalid value

  Scenario: SCN-043-003-F — ENABLE_OLLAMA only consulted on the slow path (lazy gating)
    Given Tesseract output is sufficient (>= MIN_TESSERACT_CHARS)
    And ENABLE_OLLAMA is NOT present in the environment (which would raise KeyError if read)
    When handle_ocr_request runs
    Then handle_ocr_request returns successfully with ocr_engine="tesseract"
    And ENABLE_OLLAMA is never read (no KeyError raised)
```

### Implementation Plan

1. **`ml/app/ocr.py` lines 244–256:** Replace the pre-fix block:
   ```python
   if len(text) < MIN_TESSERACT_CHARS:
       ollama_url = os.environ.get("OLLAMA_URL", "")
       if ollama_url:
           ollama_text = extract_text_ollama(image_bytes, ollama_url)
           if len(ollama_text) > len(text):
               text = ollama_text
               engine = "ollama"
   ```
   with the post-fix block:
   ```python
   if len(text) < MIN_TESSERACT_CHARS:
       enable_ollama = os.environ["ENABLE_OLLAMA"].strip().lower()
       if enable_ollama in ("true", "1", "yes", "on"):
           ollama_text = extract_text_ollama(image_bytes)
           if len(ollama_text) > len(text):
               text = ollama_text
               engine = "ollama"
       elif enable_ollama in ("false", "0", "no", "off", ""):
           pass  # Ollama fallback explicitly disabled — Tesseract result stands.
       else:
           raise RuntimeError(
               f"ENABLE_OLLAMA must be exactly one of true/1/yes/on/false/0/no/off, "
               f"got {enable_ollama!r} (HL-RESCAN-006 / Gate G028 — no-defaults SST policy)"
           )
   ```
   Add a comment block above the block naming HL-RESCAN-006 / Gate G028 / spec 049 (no-defaults SST policy) and explaining why the constant exists.
2. **`ml/tests/test_ocr.py`:** Add a new `TestEnableOllamaFailLoudGating` class with 5 test methods:
   - `test_enable_ollama_truthy_invokes_ollama_fallback` — patches Tesseract to return short text, patches Ollama to return long text, sets `ENABLE_OLLAMA=true`, asserts Ollama was called and ocr_engine == "ollama"
   - `test_enable_ollama_falsy_skips_ollama_fallback` — patches Tesseract to return short text, patches Ollama to flag if called, sets `ENABLE_OLLAMA=false`, asserts Ollama was NOT called and ocr_engine == "tesseract"
   - `test_enable_ollama_unset_raises_keyerror` — patches Tesseract to return short text, omits ENABLE_OLLAMA from environment via `clear=True`, asserts `pytest.raises(KeyError, match="ENABLE_OLLAMA")`
   - `test_enable_ollama_invalid_value_raises_runtimeerror` — patches Tesseract to return short text, sets `ENABLE_OLLAMA=maybe`, asserts `pytest.raises(RuntimeError, match="ENABLE_OLLAMA must be exactly one of")`
   - `test_enable_ollama_only_consulted_when_tesseract_insufficient` — patches Tesseract to return long text, omits ENABLE_OLLAMA via `clear=True`, asserts handle_ocr_request returns successfully with ocr_engine == "tesseract" (proves lazy gating)
3. **`ml/tests/test_keep.py`:** Update two pre-existing tests:
   - `test_ollama_fallback` — add `"ENABLE_OLLAMA": "true"` to the existing `patch.dict(os.environ, {"OLLAMA_URL": "http://localhost:11434"})` call
   - `test_both_ocr_fail_returns_ok` — wrap the inner `asyncio.run(handle_ocr_request(...))` in a new `with patch.dict(os.environ, {"ENABLE_OLLAMA": "false"}, clear=False):` block; add a comment explaining the HL-RESCAN-006 reason
4. **RED→GREEN proof:** Capture FAIL output by temporarily reverting `ml/app/ocr.py` lines 244–256 to the pre-fix `os.environ.get("OLLAMA_URL", "")` form via `replace_string_in_file`, keeping the new tests in place. Run `./smackerel.sh test unit --python -k 'TestEnableOllamaFailLoudGating'`. Observe exactly THREE FAILs (truthy / unset / invalid) with the expected error messages. Restore via `replace_string_in_file` and re-run to confirm GREEN.
5. Confine all changes to `ml/app/ocr.py` + `ml/tests/test_ocr.py` + `ml/tests/test_keep.py` plus the bug-packet artifacts in `specs/043-ollama-test-infrastructure/bugs/BUG-043-003-ml-ocr-ollama-fail-loud/`. No production runtime Go code, no compose, no `config/smackerel.yaml`, no other `specs/**`, no CI workflow.

### Test Plan

- **RED→GREEN proof (scenario-first TDD):** Temporarily revert `ml/app/ocr.py` lines 244–256 to the pre-fix `os.environ.get("OLLAMA_URL", "")` form via `replace_string_in_file`, keeping all new test code in place. Re-run `./smackerel.sh test unit --python -k 'TestEnableOllamaFailLoudGating'`. Observe `test_enable_ollama_truthy_invokes_ollama_fallback` FAIL (because pre-fix code passes positional `ollama_url` arg, breaking the post-fix assertion `ollama_url_seen == [""]`), `test_enable_ollama_unset_raises_keyerror` FAIL (because pre-fix code uses `.get(KEY, "")` which returns `""` instead of raising), `test_enable_ollama_invalid_value_raises_runtimeerror` FAIL (because pre-fix code has no validation block) — three FAIL with explicit error messages naming the missing assertion. The other two tests (`test_enable_ollama_falsy_skips_ollama_fallback` and `test_enable_ollama_only_consulted_when_tesseract_insufficient`) PASS by coincidence on the pre-fix code (their setup matches what pre-fix code happens to do). All 442 OTHER tests in `ml/tests/` PASS unchanged. Restore the production fix via `replace_string_in_file` and re-run → all 445 tests PASS GREEN. Captured in report.md > Test Evidence > Red→Green proof (scenario-first TDD).
- **Targeted Python unit suite (slow-path coverage):** `./smackerel.sh test unit --python -k 'TestEnableOllamaFailLoudGating or test_ollama or test_both_ocr_fail'` runs the new gate tests + pre-existing Ollama-related tests + the updated test_keep.py tests — all PASS.
- **Adversarial isolation:** Each new test fails RED only when the production code regresses. The truthy / unset / invalid tests are non-tautological — their assertions could not be satisfied by the pre-fix code. The falsy / sufficient-tesseract tests are positive guard rails (intentionally pass on both pre-fix and post-fix code) — they prove the new gate did not over-restrict.
- **Cross-test smoke:** `./smackerel.sh test unit --python` covers the full ML sidecar Python unit suite (`ml/tests/test_ocr.py`, `ml/tests/test_keep.py`, all other ml/tests/ files) — 445 PASS, no regression.
- **Static checks:** `./smackerel.sh lint` clean; the Python sidecar uses ruff/black via its own pyproject.toml; no formatter drift.

#### Test Plan Coverage Matrix

| Scenario / Behavior | Test Type | File | Test ID | Adversarial? | Regression E2E |
|---|---|---|---|---|---|
| SCN-043-003-A: handle_ocr_request reads ENABLE_OLLAMA fail-loud | unit (Python) | ml/tests/test_ocr.py | TestEnableOllamaFailLoudGating::test_enable_ollama_unset_raises_keyerror | YES — fails RED if `os.environ["ENABLE_OLLAMA"]` is replaced with `os.environ.get("...", "")` | Persistent in-tree adversarial Python test that runs on every `./smackerel.sh test unit --python` invocation. The fail-loud gate is a single-call-site invariant; the regression suite IS the Python test suite itself. |
| SCN-043-003-B: ENABLE_OLLAMA truthy invokes Ollama fallback | unit (Python) | ml/tests/test_ocr.py | TestEnableOllamaFailLoudGating::test_enable_ollama_truthy_invokes_ollama_fallback | YES — fails RED if production code passes positional `ollama_url` to `extract_text_ollama` (the pre-fix shape) | Same as above. |
| SCN-043-003-C: ENABLE_OLLAMA falsy skips Ollama fallback | unit (Python) | ml/tests/test_ocr.py | TestEnableOllamaFailLoudGating::test_enable_ollama_falsy_skips_ollama_fallback | NO (positive guard rail — passes on both pre-fix and post-fix; locks the behavior contract going forward) | Same as above. |
| SCN-043-003-D: ENABLE_OLLAMA missing raises KeyError | unit (Python) | ml/tests/test_ocr.py | TestEnableOllamaFailLoudGating::test_enable_ollama_unset_raises_keyerror | YES — fails RED if production code has any defensive default for ENABLE_OLLAMA | Same as above. |
| SCN-043-003-E: ENABLE_OLLAMA invalid raises RuntimeError naming Gate G028 | unit (Python) | ml/tests/test_ocr.py | TestEnableOllamaFailLoudGating::test_enable_ollama_invalid_value_raises_runtimeerror | YES — fails RED if production code has no validation block | Same as above. |
| SCN-043-003-F: ENABLE_OLLAMA only consulted on slow path (lazy gating) | unit (Python) | ml/tests/test_ocr.py | TestEnableOllamaFailLoudGating::test_enable_ollama_only_consulted_when_tesseract_insufficient | NO (positive guard rail — proves the gate is not hoisted) | Same as above. |
| Canary: pre-existing Ollama URL env-read positive case preserved | unit (Python) | ml/tests/test_ocr.py | TestExtractTextOllama::test_ollama_url_from_env | NO (canary) | Pre-existing extract_text_ollama positive case; preserved unchanged. |
| Canary: pre-existing keep-connector OCR fallback positive case preserved | unit (Python) | ml/tests/test_keep.py | TestOCR::test_ollama_fallback (updated patch.dict) | NO (canary, with minimal patch.dict update for new gate) | Pre-existing keep-connector OCR fallback case; preserved with minimal env-patch update. |
| Canary: pre-existing both-engines-fail positive case preserved | unit (Python) | ml/tests/test_keep.py | TestOCR::test_both_ocr_fail_returns_ok (updated patch.dict) | NO (canary, with minimal patch.dict update for new gate) | Pre-existing both-engines-fail case; preserved with minimal env-patch update. |
| Broader: full ml/tests/ Python suite passes | unit (Python) | ml/tests/* | 445 tests | (mixed) | `./smackerel.sh test unit --python` returns `445 passed in 12.60s`; zero regression in any pre-existing test. |

### Definition of Done

- [x] `ml/app/ocr.py` line 244 reads `os.environ["ENABLE_OLLAMA"].strip().lower()` (fail-loud, no positional default). [SCN-043-003-A]
   → Evidence: `grep -n 'os.environ\["ENABLE_OLLAMA"\]' ml/app/ocr.py` returns the fail-loud read site. See report.md > Code Diff Evidence.
- [x] `ml/app/ocr.py` line 235 (the documented pre-fix form) is no longer present in the executable code branch; the only remaining occurrence is inside the explanatory comment block above the new gate. [SCN-043-003-A]
   → Evidence: `grep -n 'os.environ.get\|os.getenv' ml/app/ocr.py` returns zero positional-default reads in executable code. See report.md > Code Diff Evidence.
- [x] The new gate accepts exactly the canonical truthy tokens `true/1/yes/on` (case-insensitive, whitespace-trimmed) and invokes `extract_text_ollama` for them. [SCN-043-003-B]
   → Evidence: `grep -n 'true.*1.*yes.*on' ml/app/ocr.py` returns the truthy-tuple membership check. See report.md > Code Diff Evidence.
- [x] The new gate accepts exactly the canonical falsy tokens `false/0/no/off/""` (case-insensitive, whitespace-trimmed) and skips the Ollama fallback for them. [SCN-043-003-C]
   → Evidence: `grep -n 'false.*0.*no.*off' ml/app/ocr.py` returns the falsy-tuple membership check. See report.md > Code Diff Evidence.
- [x] The new gate raises `RuntimeError` for any other token, with an error message naming `ENABLE_OLLAMA` AND naming Gate G028. [SCN-043-003-E]
   → Evidence: `grep -n 'RuntimeError.*ENABLE_OLLAMA must be exactly one of\|HL-RESCAN-006 / Gate G028' ml/app/ocr.py` returns the validation block. See report.md > Code Diff Evidence.
- [x] The new gate only consults `ENABLE_OLLAMA` on the slow path (`if len(text) < MIN_TESSERACT_CHARS:`); the fast path does not read the variable. [SCN-043-003-F]
   → Evidence: `grep -B 1 'os.environ\["ENABLE_OLLAMA"\]' ml/app/ocr.py` returns the line preceded by the `if len(text) < MIN_TESSERACT_CHARS:` guard. See report.md > Code Diff Evidence.
- [x] The post-fix call site invokes `extract_text_ollama(image_bytes)` with NO positional `ollama_url` arg; `extract_text_ollama` line 91 owns the `OLLAMA_URL` fail-loud read. [SCN-043-003-A, B]
   → Evidence: `grep -n 'extract_text_ollama(image_bytes)' ml/app/ocr.py` returns the call site with single positional arg. See report.md > Code Diff Evidence.
- [x] `ml/tests/test_ocr.py` declares a new `TestEnableOllamaFailLoudGating` class with five test methods. [SCN-043-003-A through F]
   → Evidence: `grep -n 'class TestEnableOllamaFailLoudGating\|def test_enable_ollama' ml/tests/test_ocr.py` returns the class declaration and 5 test method declarations. See report.md > Code Diff Evidence.
- [x] `ml/tests/test_keep.py` `test_ollama_fallback` and `test_both_ocr_fail_returns_ok` set `ENABLE_OLLAMA` explicitly in their `patch.dict(os.environ, ...)` calls. [SCN-043-003-A]
   → Evidence: `grep -n 'ENABLE_OLLAMA.*true\|ENABLE_OLLAMA.*false' ml/tests/test_keep.py` returns both call sites. See report.md > Code Diff Evidence.
- [x] RED proof captured: temporarily reverting `ml/app/ocr.py` lines 244–256 to the pre-fix `os.environ.get("OLLAMA_URL", "")` form causes EXACTLY THREE of the five new tests to FAIL with the expected error messages. [SCN-043-003-A, B, D, E]
   → Evidence: see report.md > Test Evidence > Red→Green proof (scenario-first TDD).
- [x] GREEN proof captured: restoring the production fix returns the suite to all-PASS (445 passed). [SCN-043-003-A through F]
   → Evidence: see report.md > Test Evidence > Red→Green proof (scenario-first TDD) — restore step.
- [x] Targeted suite: `./smackerel.sh test unit --python -k 'TestEnableOllamaFailLoudGating or test_ollama'` PASS. [SCN-043-003-A through F]
   → Evidence: see report.md > Validation Evidence > Targeted suite.
- [x] Cross-test smoke: full `./smackerel.sh test unit --python` PASS (445 tests, all of ml/tests/). [Broader regression]
   → Evidence: see report.md > Validation Evidence > Cross-test smoke.
- [x] Static checks: `./smackerel.sh lint` clean (Python lint stage exits 0). [Broader regression]
   → Evidence: see report.md > Validation Evidence > Static checks.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior. [SCN-043-003-A through F]
   → Evidence: persistent in-tree `TestEnableOllamaFailLoudGating` (5 test methods covering A/B/C/D/E/F) — runs on every `./smackerel.sh test unit --python` invocation. The fail-loud gate is a single-call-site invariant; the regression suite IS the Python test suite itself. See report.md > Audit Evidence > Regression Evidence.
- [x] Broader E2E regression suite passes — full `ml/tests/` Python suite (445 tests) plus the existing Go-side unit suite for `internal/connector/keep/...` (which calls into the OCR endpoint via HTTP, untouched by this fix) all PASS, including the spec 043 BUG-001 + BUG-002 regression tests. [Broader regression]
   → Evidence: `./smackerel.sh test unit --python` returns `445 passed`; spec 043 BUG-001 (ollama image pin) and BUG-002 (healthcheck) coverage is unaffected by this fix (they live in compose / Dockerfile surfaces, not in ml/app). See report.md > Audit Evidence > Cross-package smoke.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns. [Pre-existing TestExtractTextOllama, TestOCR canaries + spec 043 BUG-001 + BUG-002 contracts]
   → Evidence: `TestExtractTextOllama::test_ollama_url_from_env`, `TestExtractTextOllama::test_ollama_returns_empty_on_exception`, `TestOCR::test_ollama_fallback` (updated patch.dict), `TestOCR::test_both_ocr_fail_returns_ok` (updated patch.dict), all pre-existing TestOCRCacheLRU / TestOCRRequestCachedFastPath / TestSecurityOCR tests — all PASS unchanged. The new ENABLE_OLLAMA gate is purely additive at one call site — it adds a new branch + new tests WITHOUT touching the existing extract_text_ollama function or the cache layer. Running these canaries before the broader suite reruns proves the new gate did not over-reach into adjacent surfaces. See report.md > Audit Evidence > Canary suite.
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. [Shared Infrastructure Impact Sweep]
   → Evidence: rollback is a single git revert of the BUG-043-003 commit. The new gate is purely additive at one call site; the live self-hosted `ENABLE_OLLAMA=false` env is unchanged, so revert is safe — no live-config mismatch could result. Restore is the same git revert. Verified by the RED proof step which temporarily disables the gate (revert to silent-default form), confirms expected FAIL output, then restores. See report.md > Code Diff Evidence + Test Evidence > Red→Green proof (scenario-first TDD).
- [x] Change Boundary respected. The fix touches only `ml/app/ocr.py` + `ml/tests/test_ocr.py` + `ml/tests/test_keep.py` plus the bug-packet artifacts. No production runtime Go code, no compose, no `config/smackerel.yaml`, no other `specs/**`, no CI workflow.
   → Evidence: `git status --short` shows only allowed-family files. See report.md > Code Diff Evidence.
- [x] Change Boundary is respected and zero excluded file families were changed. [Allowed file families + Excluded surfaces enumerated below]
   → Evidence: `git status --short` shows only allowed-family files. Zero changes to excluded surfaces. See report.md > Code Diff Evidence.

### Shared Infrastructure Impact Sweep

`ml/app/ocr.py` `handle_ocr_request` is the **single OCR entry point in the ML sidecar** — every OCR request from any Go-side connector (currently the Google Keep connector; future: any connector that uploads images for text extraction) flows through this function. Changes to its env-read behavior affect every OCR request in production. The BUG-043-003 fix has the following blast radius:

- **Direct downstream consumers:** the Google Keep connector (`internal/connector/keep/`) is the sole production caller of the `/ocr` HTTP endpoint that wraps `handle_ocr_request`. The HTTP request and response shapes are unchanged, so the connector code is unaffected. Future connectors that adopt OCR will inherit the same fail-loud gate.
- **Operator-side fan-out:** the self-hosted deployment env file (`config/generated/self-hosted.env`) MUST contain `ENABLE_OLLAMA=false` (the spec 043 default per `design.md` line 91 and line 166). The SST loader `scripts/commands/config.sh` already emits this value correctly per spec 043 BUG-001 + BUG-002 close-outs. No operator action required for the fix.
- **Adapter-side fan-out:** none. The deploy adapter overlay reads the SST-generated env file at apply time; it does not introspect the OCR gate variable directly.
- **Test infrastructure (canary surface):** all pre-existing `ml/tests/test_ocr.py` and `ml/tests/test_keep.py` tests PASS unchanged after the minimal `patch.dict(os.environ, ...)` updates to the two pre-existing tests that exercised the Ollama fallback path. The 445-test Python unit suite is the canary; running it before the broader suite reruns proves the new gate did not over-reach.
- **Generated-artifact contract:** none — `config/generated/<env>.env` files already contain `ENABLE_OLLAMA` per spec 043; no SST loader change required.
- **Bootstrap contract for downstream specs:** spec 043 (Ollama test infrastructure) is the SST-flag owner. The fix realigns the runtime-side enforcement with the spec 043 documented contract — strengthening the gate without changing the SST source-of-truth.
- **Rollback path:** see the corresponding DoD item — single `git revert`; no live-config mismatch possible because the self-hosted `ENABLE_OLLAMA=false` value is already in the generated env file.
- **Ordering / timing / storage / session / context / role / blast radius:** no impact. The fix runs as a per-request env-read on the slow path only; no daemon state, no shared cache change, no cross-process ordering concern. Lazy gating (DD-2) preserves the production fast-path performance.
- **Stress coverage assessment (Gate G026):** explicit stress/load coverage is NOT REQUIRED for this fix. The phrase "slow path" in this scope refers to a Python-internal code branch (the post-Tesseract OCR fallback branch), NOT a service-level objective. The fix adds a single in-process env-read + small string comparison to the existing slow-path branch; it changes neither algorithmic complexity nor I/O profile. The pre-existing `TestOCRCacheLRU` and `TestOCRRequestCachedFastPath` tests already cover the throughput-relevant cache and fast-path behavior unchanged. No additional `./smackerel.sh test stress` invocation is warranted; this DoD line documents the assessment for the Gate G026 lint.

### Change Boundary

**Allowed file families (this fix may modify):**

- `ml/app/ocr.py` — the single fail-loud gate site being fixed (the only production code change point)
- `ml/tests/test_ocr.py` — new TestEnableOllamaFailLoudGating class
- `ml/tests/test_keep.py` — minimal `patch.dict(os.environ, ...)` updates to two pre-existing tests
- `specs/043-ollama-test-infrastructure/bugs/BUG-043-003-ml-ocr-ollama-fail-loud/**` — this bug packet's seven artifacts

**Excluded surfaces (this fix MUST NOT touch):**

- `ml/app/keep.py`, `ml/app/main.py`, `ml/app/embeddings.py`, `ml/app/llm.py`, `ml/app/auth.py` — other ML sidecar modules; outside HL-RESCAN-006's scope (HL-RESCAN-013 covers `ml/app/auth.py` separately)
- `internal/connector/keep/...` Go code — the connector calls the OCR HTTP endpoint via the Go HTTP client; the HTTP contract is unchanged, so no Go change is required or allowed
- `config/smackerel.yaml` — the SST source-of-truth values are owned by spec 043; the per-env `environments.<env>.ollama_enabled` keys already exist and are correct
- `scripts/commands/config.sh` — the SST loader already emits `ENABLE_OLLAMA=true|false` per env per spec 043; no loader change required
- `config/generated/<env>.env` — generated artifacts; never edit by hand
- `deploy/compose.deploy.yml`, `docker-compose.yml`, `ml/Dockerfile`, `Dockerfile` — runtime container configuration; the gate is at the Python read site, not at compose / Dockerfile
- `specs/043-ollama-test-infrastructure/spec.md`, `design.md`, `scopes.md`, `state.json`, `report.md`, `uservalidation.md` — foreign-owned parent-spec content; outside `bubbles.devops` mode edit scope
- `specs/049-no-defaults-sst-policy/...` — the Gate G028 policy doc itself; outside HL-RESCAN-006's scope (the fix REFERENCES the policy in error messages but does not change the policy)
- Any other `specs/**` directory — single-bug-scope discipline
- `.github/workflows/*` — unrelated; the Python tests are invoked by the existing `unit-tests` job
- `scripts/...` (other than the SST loader, which is excluded above) — unrelated

### Regression E2E Coverage

| Scenario | Test ID | File | Type | Adversarial? |
|---|---|---|---|---|
| SCN-043-003-A: handle_ocr_request reads ENABLE_OLLAMA fail-loud | TestEnableOllamaFailLoudGating::test_enable_ollama_unset_raises_keyerror | ml/tests/test_ocr.py | unit (Python) | YES — fails RED if `os.environ["ENABLE_OLLAMA"]` is replaced with positional default |
| SCN-043-003-B: ENABLE_OLLAMA truthy invokes Ollama fallback | TestEnableOllamaFailLoudGating::test_enable_ollama_truthy_invokes_ollama_fallback | same as above | unit (Python) | YES — fails RED if pre-fix call site shape (positional `ollama_url`) is restored |
| SCN-043-003-C: ENABLE_OLLAMA falsy skips Ollama fallback | TestEnableOllamaFailLoudGating::test_enable_ollama_falsy_skips_ollama_fallback | same as above | unit (Python) | NO (positive guard rail) |
| SCN-043-003-D: ENABLE_OLLAMA missing raises KeyError | TestEnableOllamaFailLoudGating::test_enable_ollama_unset_raises_keyerror | same as above | unit (Python) | YES — fails RED if production code has any defensive default for ENABLE_OLLAMA |
| SCN-043-003-E: ENABLE_OLLAMA invalid raises RuntimeError | TestEnableOllamaFailLoudGating::test_enable_ollama_invalid_value_raises_runtimeerror | same as above | unit (Python) | YES — fails RED if production code has no validation block |
| SCN-043-003-F: ENABLE_OLLAMA only consulted on slow path | TestEnableOllamaFailLoudGating::test_enable_ollama_only_consulted_when_tesseract_insufficient | same as above | unit (Python) | NO (positive guard rail) |
| Canary: pre-existing TestExtractTextOllama positive cases | TestExtractTextOllama (4 sub-tests) | ml/tests/test_ocr.py | unit (Python) | NO (canaries) |
| Canary: pre-existing TestOCR keep-connector cases | TestOCR (multiple sub-tests, 2 with updated patch.dict) | ml/tests/test_keep.py | unit (Python) | NO (canaries) |
| Canary: pre-existing TestSecurityOCR (SSRF protection) | TestSecurityOCR (5+ sub-tests) | ml/tests/test_keep.py | unit (Python) | NO (canaries) |
| Canary: pre-existing TestOCRCacheLRU + TestOCRRequestCachedFastPath | TestOCRCacheLRU + TestOCRRequestCachedFastPath (10+ sub-tests) | ml/tests/test_ocr.py | unit (Python) | NO (canaries) |
