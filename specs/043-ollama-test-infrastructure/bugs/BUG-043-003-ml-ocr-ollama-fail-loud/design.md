# Design: BUG-043-003 — ML OCR fallback reads OLLAMA_URL with silent default, bypassing the spec 043 ENABLE_OLLAMA SST gate

## Approach

Replace the silent-default env-read at `ml/app/ocr.py` line 235 with a fail-loud read of the spec 043 SST flag `ENABLE_OLLAMA`. The new gate uses `os.environ["ENABLE_OLLAMA"]` (raises `KeyError` when missing) and validates the value against the canonical truthy/falsy token sets, raising `RuntimeError` for any other value. The downstream `extract_text_ollama` call is unchanged; its line 91 already reads `OLLAMA_URL` fail-loud and is the correct enforcement site for that variable. Add a new `TestEnableOllamaFailLoudGating` class in `ml/tests/test_ocr.py` with five test methods (truthy / falsy / unset / invalid / sufficient-tesseract guard rail) that lock the fail-loud contract and would fail RED if reverted to the silent-default form. Update two pre-existing tests in `ml/tests/test_keep.py` (`test_ollama_fallback`, `test_both_ocr_fail_returns_ok`) to set `ENABLE_OLLAMA` explicitly — they were authored against the pre-fix gate which read `OLLAMA_URL` and need the new gate's required env var set to keep their original behavior.

## Design Decisions

### DD-1: Read ENABLE_OLLAMA, not a synthetic gate variable

**Decision:** The new fail-loud read targets `ENABLE_OLLAMA` — the same SST flag spec 043 documented as the owner of the Ollama-fallback decision. Do NOT introduce a new gate variable (e.g. `OLLAMA_OCR_FALLBACK_ENABLED` or `ENABLE_OCR_OLLAMA_FALLBACK`).

**Rationale:** Spec 043 (`spec.md` line 331, `design.md` lines 10/91/132/145/166/337/596) explicitly documents `ENABLE_OLLAMA` as the per-env SST flag controlling whether the Ollama service is active for the runtime. The OCR fallback is one consumer of that decision — it inherits the SST flag rather than introducing a parallel gate. A new gate variable would split the SST surface, violate the single-source-of-truth principle, and create the very kind of drift the no-defaults policy is designed to prevent.

**Alternatives rejected:**
- New `OLLAMA_OCR_FALLBACK_ENABLED` variable: rejected per the rationale above.
- Read both `ENABLE_OLLAMA` and `OLLAMA_URL`, error on inconsistency: rejected because it duplicates the validation that `extract_text_ollama` line 91 already does for `OLLAMA_URL`. The cleanest gating is `ENABLE_OLLAMA` at the call site + `OLLAMA_URL` fail-loud inside `extract_text_ollama`.

### DD-2: Lazy-read on the slow path only

**Decision:** Read `ENABLE_OLLAMA` inside the `if len(text) < MIN_TESSERACT_CHARS:` branch — i.e. only when the OCR call is on the slow path where Ollama would be invoked. Do NOT hoist the read to module load time or to the top of `handle_ocr_request`.

**Rationale:** Tesseract is the primary OCR engine. The vast majority of requests produce sufficient Tesseract output and never need the Ollama fallback. Hoisting the env read would (a) pay the env-read cost on every request even when the fallback isn't used, and (b) — more importantly — would force the env var to be present even in deployments that never run the slow path (e.g. dev environments where the full SST is not bundled). Lazy reading is also what the existing fail-loud reads in `extract_text_ollama` (`OLLAMA_URL` at line 91, `OLLAMA_VISION_MODEL` at line 105) already do.

**Alternatives rejected:**
- Module-load-time read with cached boolean: rejected because the cache would lock the env var into module state and break test isolation; the unit tests need to vary `ENABLE_OLLAMA` per case.
- Read at the top of `handle_ocr_request`: rejected because it forces the env var to be present even on the fast path where Ollama is irrelevant.

### DD-3: Strict tokens, no defensive coercion

**Decision:** Accept exactly the tokens `true`/`1`/`yes`/`on` as truthy and `false`/`0`/`no`/`off`/`""` as falsy (case-insensitive, whitespace-trimmed). Any other value raises `RuntimeError` naming `ENABLE_OLLAMA` AND naming Gate G028. Do NOT default an unknown token to either truthy or falsy.

**Rationale:** Defensive coercion is itself a silent-default pattern — the operator who typed `enabled` expected that to mean truthy, but a coercion-to-falsy default would silently disable the fallback with no error signal. The same anti-pattern that motivates Gate G028 for the env-read site also motivates strict validation here. The empty string `""` is treated as falsy (not as missing) because the spec 043 SST loader documents the convention that an empty `ENABLE_OLLAMA` value represents the "off" semantic for environments that don't enable the profile (e.g. `self-hosted` and `dev` per `specs/043-ollama-test-infrastructure/design.md` line 91).

**Alternatives rejected:**
- Accept any non-empty string as truthy: rejected because it permits typos like `falase` to silently mean truthy.
- Accept Python `bool` parsing semantics (`bool("false") is True`): rejected because the operator's typed string is the policy, not Python's coercion rules.
- Default unknown values to falsy with a warning log: rejected because the "warning log" is itself a silent default — the operator gets the wrong runtime behavior and only learns about it via log inspection.

### DD-4: extract_text_ollama signature unchanged; pass no positional ollama_url

**Decision:** The post-fix call site reads `extract_text_ollama(image_bytes)` — no positional `ollama_url` argument. The `extract_text_ollama` function's line 91 reads `OLLAMA_URL` fail-loud from the environment.

**Rationale:** The pre-fix call site passed `ollama_url` positionally, which forced `handle_ocr_request` to ALSO read `OLLAMA_URL` (with the silent default that this bug fixes). Passing no positional argument lets `extract_text_ollama` own the `OLLAMA_URL` read entirely — there is exactly one fail-loud read of `OLLAMA_URL` in the OCR path, and it lives where the variable is consumed. This is the minimum-coupling design.

**Alternatives rejected:**
- Move `OLLAMA_URL` read into `handle_ocr_request` and pass it positionally to `extract_text_ollama`: rejected because it duplicates the read site (with risk of drift) and forces `handle_ocr_request` to validate a variable it doesn't consume directly.
- Read `OLLAMA_URL` in both places, assert equality: rejected as over-engineering.

### DD-5: Update test_keep.py call sites, not the test design

**Decision:** Two pre-existing tests in `ml/tests/test_keep.py` (`test_ollama_fallback`, `test_both_ocr_fail_returns_ok`) need `ENABLE_OLLAMA` set in their environment patch. Update the `patch.dict(os.environ, ...)` argument to include the new required key. Do NOT redesign the tests or move them.

**Rationale:** The pre-fix gate read `OLLAMA_URL`; both tests set `OLLAMA_URL` to satisfy that read. The post-fix gate also requires `ENABLE_OLLAMA` — which the tests need to declare explicitly. The minimal patch is to add `"ENABLE_OLLAMA": "true"` (for `test_ollama_fallback` which exercises the truthy path) or `"ENABLE_OLLAMA": "false"` (for `test_both_ocr_fail_returns_ok` which exercises the both-engines-fail path with Ollama explicitly disabled). The tests' assertions and intent are preserved.

**Alternatives rejected:**
- Migrate both test_keep.py tests to test_ocr.py: rejected because they exercise the keep-connector OCR contract end-to-end and have legitimate reasons to live in test_keep.py.
- Add a session-level pytest fixture that sets ENABLE_OLLAMA=true globally: rejected because it would make the new TestEnableOllamaFailLoudGating tests harder to reason about (the unset/invalid cases need ENABLE_OLLAMA absent or set to a specific value); explicit per-test patches are the cleanest design.

### DD-6: RED proof via temporary block revert, restore via replace_string_in_file

**Decision:** Capture the RED→GREEN proof by temporarily reverting `ml/app/ocr.py` lines 244–256 (the new ENABLE_OLLAMA gate block) to the pre-fix `os.environ.get("OLLAMA_URL", "")` form via `replace_string_in_file` — keeping the new tests in `ml/tests/test_ocr.py` AND the updated tests in `ml/tests/test_keep.py` intact. Re-run the Python unit suite. Observe exactly THREE FAILs (truthy / unset / invalid) plus possibly the two updated test_keep.py tests if the revert breaks them. After capturing, restore via `replace_string_in_file`.

**Rationale:** This isolates the proof to the new gate block only. A `git stash` of the entire ocr.py file would also remove the production fix, so the RED state would be the same as pre-fix code with new tests — which is what we want, but the explicit replace-and-restore is more controllable in the IDE tool surface and avoids stash conflicts with the parallel session's working-tree edits to other files.

**Alternatives rejected:**
- `git stash push -p ml/app/ocr.py`: rejected because interactive patch staging cannot be scripted reliably and is error-prone.
- Whole-file stash of ml/app/ocr.py + git apply: rejected because the diff would conflict with the parallel session's edits to other files in the working tree.

## Trade-offs

- The fail-loud read at line 244 raises `KeyError` rather than a typed `MissingSSTKeyError` exception. This is consistent with the existing fail-loud reads in `extract_text_ollama` (line 91 reads `os.environ["OLLAMA_URL"]` which also raises `KeyError`) and with the canonical Python idiom for required env vars. A typed exception class would be over-engineering for the single call site.
- The strict-tokens validation rejects values that some operators might consider "obviously truthy" (e.g. `enabled`, `t`, `Y`). This is intentional: the SST source-of-truth file `config/smackerel.yaml` is the operator-facing contract; the runtime gate enforces the values the SST loader emits, which are exactly the canonical truthy/falsy tokens. An operator who needs to add a new token form should add it both to the SST loader AND to this gate, in lockstep.
- The post-fix gate does NOT log a message when the fallback is skipped (falsy ENABLE_OLLAMA). This is intentional: the fast path stays silent. An operator debugging "why is Ollama not being invoked?" can check the `ENABLE_OLLAMA` env var directly (`docker exec ml printenv ENABLE_OLLAMA`) or the SST source.
