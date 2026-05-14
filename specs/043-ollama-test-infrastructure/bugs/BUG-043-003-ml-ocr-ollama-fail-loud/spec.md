# Bug: BUG-043-003 — ML OCR fallback reads OLLAMA_URL with silent default, bypassing the spec 043 ENABLE_OLLAMA SST gate

## Classification

- **Type:** ML sidecar defect — silent default at runtime env-read site (Gate G028 / no-defaults SST policy violation)
- **Severity:** P2 — MEDIUM (runtime behavior is correct in the home-lab default config because `ENABLE_OLLAMA=false` keeps `OLLAMA_URL` empty in `config/smackerel.yaml`, so the silent default is a no-op there; the defect is the bypass of the spec 043 SST gate, not a live data-loss path)
- **Parent Spec:** 043 — Ollama test infrastructure (the per-env `ENABLE_OLLAMA` SST gate owner)
- **Workflow Mode:** test-to-doc
- **Status:** Fixed
- **Discovered By:** 2026-05-14 home-lab readiness re-scan (finding HL-RESCAN-006)

## Problem Statement

`ml/app/ocr.py` `handle_ocr_request` decided whether to invoke the Ollama OCR fallback via:

```python
if len(text) < MIN_TESSERACT_CHARS:
    ollama_url = os.environ.get("OLLAMA_URL", "")
    if ollama_url:
        ollama_text = extract_text_ollama(image_bytes, ollama_url)
        ...
```

This is forbidden by the repo-wide no-defaults SST policy (`os.getenv("KEY", "default")` and `os.environ.get("KEY", "default")` are explicitly banned in Python per `.github/instructions/smackerel-no-defaults.instructions.md`, gated under spec 049 / Gate G028). The defensive default `""` silently swallowed a missing `OLLAMA_URL` environment variable by skipping the Ollama fallback path — making the gate effectively `if OLLAMA_URL is not empty` rather than `if the spec 043 ENABLE_OLLAMA flag is truthy`.

The two contracts collided:

1. **Spec 043 contract:** Ollama OCR is an OPTIONAL fallback gated by the per-env SST flag `ENABLE_OLLAMA`. When `ENABLE_OLLAMA=true`, the system MUST invoke the fallback (and `OLLAMA_URL` MUST be present and non-empty). When `ENABLE_OLLAMA=false`, the system MUST skip the fallback cleanly, regardless of `OLLAMA_URL`.
2. **Pre-fix runtime behavior:** the gate was `if OLLAMA_URL is not empty`. This made the system rely on a coincidence — that the operator who sets `ENABLE_OLLAMA=true` ALSO sets `OLLAMA_URL` non-empty, AND that the operator who sets `ENABLE_OLLAMA=false` ALSO leaves `OLLAMA_URL` empty.

The coincidence happens to hold for the canonical home-lab `config/smackerel.yaml` (because the SST loader respects the per-env override and only emits a non-empty `OLLAMA_URL` when `ollama_enabled=true`), but it is not enforced anywhere — a future operator override that flipped one flag without the other would silently produce the wrong runtime behavior with no error signal.

The defect is the bypass of the SST gate, not a live data-loss path. The fix replaces the silent default with a fail-loud read of `ENABLE_OLLAMA` — the same SST flag spec 043 documented as the owner of this decision — and lets the existing fail-loud read of `OLLAMA_URL` inside `extract_text_ollama` (line 91) handle the URL itself.

## Detection

| Aspect | Detail |
|---|---|
| Trigger | Home-lab readiness re-scan (system review session 2026-05-14) |
| Finding | HL-RESCAN-006 |
| Severity | P2 (live home-lab config defaults `ENABLE_OLLAMA=false`, so the silent default is a no-op there; defect is the bypass of the spec 043 SST gate, not a live behavior bug) |
| Audit method | Searched `ml/app/` for `os.getenv` / `os.environ.get` with positional default; found one violation at `ml/app/ocr.py` line 235 (`os.environ.get("OLLAMA_URL", "")`). Cross-referenced `.github/instructions/smackerel-no-defaults.instructions.md` Gate G028 wording: `os.getenv("KEY", "default")` is explicitly banned in Python. Cross-referenced `specs/043-ollama-test-infrastructure/spec.md` and `design.md` for the documented `ENABLE_OLLAMA` SST gate; confirmed the runtime gate at `ml/app/ocr.py:235` was reading the wrong env var (`OLLAMA_URL`) with the wrong policy (silent default) instead of reading `ENABLE_OLLAMA` fail-loud. |

## Acceptance Criteria

- AC-1: `ml/app/ocr.py` `handle_ocr_request` MUST read `ENABLE_OLLAMA` from the environment fail-loud — no positional default to `os.environ.get` or `os.getenv`. The `os.environ["ENABLE_OLLAMA"]` form (raises `KeyError` when missing) is the required pattern.
- AC-2: When `ENABLE_OLLAMA` is truthy (`true`/`1`/`yes`/`on`, case-insensitive, whitespace-trimmed), `handle_ocr_request` MUST invoke `extract_text_ollama` (which itself reads `OLLAMA_URL` fail-loud at line 91 — pre-existing).
- AC-3: When `ENABLE_OLLAMA` is falsy (`false`/`0`/`no`/`off`/`""`, case-insensitive, whitespace-trimmed), `handle_ocr_request` MUST skip the Ollama fallback entirely. The Tesseract result (even if empty) stands.
- AC-4: When `ENABLE_OLLAMA` is missing entirely from the environment, `handle_ocr_request` MUST raise `KeyError` (fail-loud per Gate G028) on the slow path where the gate is consulted.
- AC-5: When `ENABLE_OLLAMA` is set to a non-boolean string (e.g. `maybe`, `enabled`, `disabled`), `handle_ocr_request` MUST raise `RuntimeError` naming `ENABLE_OLLAMA` AND naming Gate G028 — no defensive coercion to either truthy or falsy.
- AC-6: `ENABLE_OLLAMA` MUST only be consulted on the slow path — i.e. when Tesseract output is shorter than `MIN_TESSERACT_CHARS`. The gate MUST NOT be read on every request (lazy gating preserves the production performance characteristic).
- AC-7: A new `TestEnableOllamaFailLoudGating` test class exists in `ml/tests/test_ocr.py` with five test methods: truthy → invokes fallback; falsy → skips fallback; unset → raises `KeyError`; invalid → raises `RuntimeError`; sufficient-tesseract → does NOT consult `ENABLE_OLLAMA`. Three of the five (truthy / unset / invalid) FAIL RED on the pre-fix code; two are positive guard rails.
- AC-8: RED proof captured: temporarily reverting `ml/app/ocr.py` lines 244–256 to the pre-fix `os.environ.get("OLLAMA_URL", "")` form causes EXACTLY THREE of the five new tests to FAIL with the expected error messages, while every OTHER test in `ml/tests/test_ocr.py` continues to PASS. Restoring the fix returns the suite to all-PASS GREEN.

## Out of Scope

- Editing the existing `extract_text_ollama` function (line 91 already reads `OLLAMA_URL` fail-loud; that path was already compliant with Gate G028 — this fix only adds the `ENABLE_OLLAMA` gate at the call site).
- Editing `config/smackerel.yaml` `environments.<env>.ollama_enabled` (the SST source-of-truth values are owned by spec 043; this fix is bounded to the runtime read site).
- Editing the SST loader script `scripts/commands/config.sh` (the loader already correctly emits `ENABLE_OLLAMA=true|false` per env; the live home-lab env file already contains the correct value).
- Editing `specs/043-ollama-test-infrastructure/spec.md`, `design.md`, `scopes.md`, `state.json`, `report.md`, or `uservalidation.md` (foreign-owned parent-spec content; outside `bubbles.devops` mode edit scope).
- Editing `internal/connector/keep/...` Go code that calls into the ML OCR endpoint (the contract change is invisible from the Go side — same HTTP request shape, same response shape).
- Adding ENABLE_OLLAMA enforcement to OTHER ML endpoints that may invoke Ollama in the future (only the OCR fallback path is in HL-RESCAN-006's scope).
- Deprecating or removing the `OLLAMA_URL` environment variable (it is still required by `extract_text_ollama` line 91 and `extract_text_ollama` line 105 for the model name read; both remain fail-loud).
