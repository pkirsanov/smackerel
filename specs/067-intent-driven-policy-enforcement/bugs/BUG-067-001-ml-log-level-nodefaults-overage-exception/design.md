# Bug Fix Design: BUG-067-001

## Root Cause Analysis

### Investigation Summary
Spec 067's gaps + harden phases directly executed `ValidateException` over the real committed `policy-exception-baseline.json` at the real SST cap (`policy.policy_exception_max_age_days = 90`, now = 2026-06-22) and observed `G067-A07` for exception `G067-A05-ml-log-level` (`expires_on: 2026-12-01`, 162 days out). They recorded this as `GAP-067-G01` and routed it (entangled across `ml/app/main.py:19+21`, the baseline JSON, and cross-spec BUG-076-001). They also recorded `GAP-067-G02`: every real-baseline guard test overrides `ExceptionMaxAgeDays` to `365*10`/`180` magic constants, masking G01. This bug owns the permanent fix.

### Root Cause
Two entangled causes:

- **G01 — missing SST key + paper-over default (live, latent).** The ML sidecar log level was never given an SST key. `config/smackerel.yaml` has the core `runtime.log_level` but no `services.ml.log_level`, and config generation emits no `ML_LOG_LEVEL`. `ml/app/main.py` papered over the gap at line 22 with `os.environ.get("ML_LOG_LEVEL", "INFO").upper()` — a forbidden NO-DEFAULTS fallback — kept legal only by exception `G067-A05-ml-log-level`. Because nothing emits `ML_LOG_LEVEL`, the sidecar ALWAYS uses the literal `"INFO"`. The exception's `expires_on` (2026-12-01) is 162 days from now, exceeding the 90-day cap, so the production `ValidateException` flags it `G067-A07` — latent only because of G02.

- **G02 — magic-constant test caps (the masking test hole).** `PythonNoDefaultsGuard`/`GoNoDefaultsGuard` real-corpus tests use `ExceptionMaxAgeDays: 365*10`; the keyword real-corpus tests use `180`. None validate the committed baseline at the real 90-day cap, so the over-age exception never trips a gate. This violates spec 067 Hard Constraint 2 (test integrity).

### Impact Analysis
- Affected components: `ml/app/main.py` (sidecar logging bootstrap + required-config), config generation (`config/smackerel.yaml`, `scripts/commands/config.sh`), `policy-exception-baseline.json`, the spec 067 policy guard tests.
- Affected data: none (config + governance only; the guards are pure file-system scanners with zero runtime state).
- Affected users/operators: the NO-DEFAULTS / fail-loud SST contract is not enforced for the ML sidecar log level; the policy ratchet is silently weakened by an over-age exception that no gate catches.

## Fix Design

### Solution Approach (SST-ify — eliminate the exception permanently)
The exception exists "until `ml.log_level` SST key lands." So land it and delete the exception:

- **D1 (REQ-1) — SST key.** Add `services.ml.log_level: info` to `config/smackerel.yaml` (ml-owned; the existing `runtime.log_level: info` is the distinct core key). Mirror pattern follows the sibling Python-only ML keys (`services.ml.embedding_workers` etc.): yaml SST → shell mirror in `scripts/commands/config.sh` → Python consumer in `ml/app/main.py`. (These sidecar keys have NO Go config-struct mirror — verified: `ML_EMBEDDING_WORKERS`/`ML_HEALTH_LATENCY_SLA_MS` are absent from `internal/config`. So no Go struct field / Go drift test applies; forcing one would be wrong.)
- **D2 (REQ-2) — emit `ML_LOG_LEVEL`.** In `scripts/commands/config.sh`, extract `ML_LOG_LEVEL="$(required_value services.ml.log_level)"` next to the sibling ML extractions, and emit `ML_LOG_LEVEL=${ML_LOG_LEVEL}` in the single env-render heredoc next to the sibling ML emissions. That one emission point feeds `dev.env`, `test.env`, AND the deploy bundle `app.env` (proven by the sibling required ML keys having exactly one emission point each yet booting the sidecar in every environment). The deploy `sstKeyCatalog` in `deploy/contract.yaml` does NOT enumerate `services.ml.*` keys at all (the whole spec-050 ML block is absent), and the catalog is explicitly "informational + not lockstep-tested; treat the YAML as the SST authority" — so adding a lone `services.ml.log_level` row there would be inconsistent with every sibling ML key; the YAML remains the authority and the bundle wiring is the real contract.
- **D3 (REQ-3) — fail-loud read.** In `ml/app/main.py`: (a) remove the two `# smackerel:policy-exception` markers and the `level=os.environ.get("ML_LOG_LEVEL", "INFO")` default; configure import-time bootstrap logging WITHOUT an env-derived level (no NO-DEFAULTS read at module scope, so importing the module in tests never requires the env); (b) add `ML_LOG_LEVEL` to `_check_required_config()`'s required `keys` list (missing/empty → `sys.exit(1)`); (c) after the missing-keys check, validate the value against the `debug|info|warn|error` allowlist (mirroring the existing `SMACKEREL_ENV` allowlist pattern) and apply it via `logging.getLogger().setLevel(...)`. Reading `required["ML_LOG_LEVEL"].upper()` is fail-loud because the key is already required above.
- **D4 (REQ-4) — delete the exception.** Remove `G067-A05-ml-log-level` from `policy-exception-baseline.json`, leaving `exceptions: []`. With main.py fail-loud, the guard finds no fallback pattern → no exception needed → no `G067-A07`.
- **D5 (REQ-5) — close the test hole + adversarial regression.** Add a package test helper `realPolicyExceptionMaxAgeDays(t)` that reads `policy.policy_exception_max_age_days` directly from `config/smackerel.yaml` (SST-anchored, no re-hardcoded magic number). Replace the four real-baseline caps (`no_defaults_python_guard_test.go` L26, `no_defaults_go_guard_test.go` L26, `keyword_map_guard_test.go` L43, `keyword_routing_guard_test.go` L50) with the helper. Add two adversarial tests:
  - `TestRealBaselineHasNoOverAgeExceptionsAtRealCap` — loads the REAL committed baseline and validates EVERY exception via `ValidateException` at the real cap; FAILS (RED) while any over-age exception (like the removed 162-day `G067-A05`) is present, passes (GREEN) once removed. This is both the reproduction and the RED-if-reintroduced regression.
  - `TestValidateExceptionFlagsOverAgeAtRealCap` — proves teeth (non-tautological): a synthetic 162-day exception MUST be flagged `G067-A07` at the real cap; a synthetic 80-day exception MUST NOT be flagged.

  The temp-fixture tests (`365*100`, the `180` planted-fixture cases at L66/L100/L151/L182/L92) keep their own caps — they exercise fixture trees, not the committed baseline.
- **D6 — Python test wiring.** `ml/tests/test_main.py`: add `ML_LOG_LEVEL` to the `clear_required_env` autouse fixture and set `ML_LOG_LEVEL=info` in the success-path tests + the `_set_required_env_minus` helper so they reach the validations they intend to exercise (the new required key must not short-circuit the existing SMACKEREL_ENV / degraded-fallback / auth assertions).

### Alternative Approaches Considered
1. **Re-issue a shorter expiry on the exception** — rejected: leaves the forbidden default in place, kicks the can, and keeps the sidecar on a silent `"INFO"`. The decided approach eliminates the exception at the root.
2. **Reuse the core `runtime.log_level` / `LOG_LEVEL` for the sidecar** — rejected: conflates two distinct concerns (Go core vs Python sidecar log level); the task requires a separate ml-owned key, and the sidecar already reads ML-prefixed keys.
3. **Hardcode `90` in the four tests** — rejected: that is the same magic-constant anti-pattern with a different number and re-introduces drift risk. Reading the cap from the SST file anchors the tests to the single source of truth.

## Complexity Tracking

| Decision | Simpler fix considered | Why rejected |
|----------|------------------------|--------------|
| Add an allowlist check (`debug\|info\|warn\|error`) for `ML_LOG_LEVEL` in `_check_required_config` | Just `logging.getLogger().setLevel(os.environ["ML_LOG_LEVEL"].upper())` with no validation | A bad value would raise an unhandled `ValueError` mid-lifespan; the file already validates `SMACKEREL_ENV` against an allowlist with `logger.error` + `sys.exit(1)`, so mirroring that is consistent fail-loud, not new complexity. |
| `realPolicyExceptionMaxAgeDays(t)` helper reads the cap from `config/smackerel.yaml` | Hardcode `90` in the 4 tests | Hardcoding re-introduces the magic-constant drift the bug is fixing; reading from SST anchors the tests to the single source of truth. |
