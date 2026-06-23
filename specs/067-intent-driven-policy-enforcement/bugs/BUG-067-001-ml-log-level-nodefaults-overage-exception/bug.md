# Bug: BUG-067-001 ML_LOG_LEVEL NO-DEFAULTS violation + over-age policy exception (masked by magic-constant test caps)

## Summary
`ml/app/main.py` reads `os.environ.get("ML_LOG_LEVEL", "INFO")` — a FORBIDDEN NO-DEFAULTS fallback — kept legal only by an over-age policy exception (`G067-A05-ml-log-level`, `expires_on: 2026-12-01`, 162 days out vs the 90-day SST cap), and the live defect is hidden because every real-baseline policy guard test overrides `ExceptionMaxAgeDays` to magic constants (`365*10` / `180`) instead of the real cap.

## Severity
- [ ] Critical - System unusable, data loss
- [ ] High - Major feature broken, no workaround
- [x] Medium - Live policy-compliance violation, currently latent (masked by a test hole); fail-loud SST contract not enforced for the ML sidecar log level
- [ ] Low - Minor issue, cosmetic

## Status
- [ ] Reported
- [ ] Confirmed (reproduced)
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [x] Closed

## Reproduction Steps
1. `cat policy-exception-baseline.json` → exception `G067-A05-ml-log-level` has `expires_on: 2026-12-01`.
2. Compute the delta from now (`2026-06-22`) → `162` days; SST `policy.policy_exception_max_age_days = 90` → over by 72 days. `ValidateException` returns `G067-A07` for this exception at the real cap.
3. `grep -rn ML_LOG_LEVEL scripts/commands/config.sh config/generated/` → ZERO references: the value is emitted by nothing, so the sidecar ALWAYS falls through to the literal `"INFO"` default.
4. `grep -n 'ML_LOG_LEVEL' ml/app/main.py` → line 22 `level=os.environ.get("ML_LOG_LEVEL", "INFO").upper()` — a forbidden `os.getenv(k, default)` form under `smackerel-no-defaults`.
5. `grep -n 'ExceptionMaxAgeDays:' tests/integration/policy/*_guard_test.go` → the 4 real-baseline tests use `365*10` / `180`, never the real 90-day cap, so the over-age exception never trips a gate (G02 — masking).

## Expected Behavior
- The ML sidecar log level is an SST-owned value (`config/smackerel.yaml services.ml.log_level`) emitted into every generated env (dev/test) and the deploy bundle as `ML_LOG_LEVEL`.
- `ml/app/main.py` reads `ML_LOG_LEVEL` fail-loud (no literal fallback); a missing/empty value exits non-zero.
- No policy exception is required, so `policy-exception-baseline.json` carries no over-age entry.
- The policy guard tests validate the REAL committed baseline at the REAL SST cap, and an adversarial regression proves the guard flags any future over-age exception.

## Actual Behavior
- `ML_LOG_LEVEL` is never emitted by config generation → the sidecar silently uses the literal `"INFO"`.
- The forbidden default is kept legal only by an over-age exception (`G067-A05`, 162 days > 90 cap), which `ValidateException` would flag as `G067-A07`.
- The defect is latent because the real-baseline tests use inflated `ExceptionMaxAgeDays` magic constants, masking it (violates spec 067 Hard Constraint 2 — test integrity).

## Environment
- Service: `smackerel-ml` (Python FastAPI sidecar) + `smackerel-core` config-generation pipeline + spec 067 policy guards.
- Version: working tree at bug-open (2026-06-22).
- Platform: repo-local (pure file-system policy guards + Python unit tests + `./smackerel.sh config generate`); no live stack required.

## Error Output
```
exception "G067-A05-ml-log-level" expires_on=2026-12-01 is more than 90 days from now   (rule_id=G067-A07)
ml/app/main.py:22  level=os.environ.get("ML_LOG_LEVEL", "INFO").upper()                   (rule_id=G067-A05; required form os.environ["ML_LOG_LEVEL"])
config.sh / config/generated/: 0 occurrences of ML_LOG_LEVEL                              (sidecar always uses literal "INFO")
```

## Root Cause (filled after analysis)
See [design.md](design.md). Two entangled root causes:
- **G01 (live, latent):** the `ml.log_level` SST key was never landed, so config generation emits no `ML_LOG_LEVEL`; `ml/app/main.py` papered over the gap with a literal `"INFO"` default permitted by an over-age policy exception. Fix = land the SST key + wire emission + make the read fail-loud + delete the exception (SST-ify; the exception is no longer needed).
- **G02 (test hole):** the real-baseline policy guard tests override `ExceptionMaxAgeDays` to `365*10` / `180` instead of the real 90-day SST cap, so the over-age exception never trips a gate. Fix = anchor the real-baseline tests to the SST cap + add an adversarial regression with teeth.

## Related
- Feature: `specs/067-intent-driven-policy-enforcement/` (owns `policy-exception-baseline.json` + the policy guards).
- Discovered by: spec 067 gaps + harden phases — recorded as routed findings `GAP-067-G01` (live-but-latent over-age exception) and `GAP-067-G02` (magic-constant cap masking) in `specs/067-intent-driven-policy-enforcement/report.md` and `state.json`.
- Cross-spec reference: `specs/076-assistant-completion-rescope/bugs/BUG-076-001-ml-agent-logs-raw-conversational-content/` notes the `ML_LOG_LEVEL` literal `"INFO"` default as a "separate, already-tracked" issue — that separate tracking is THIS bug. BUG-076-001 (foreign-owned by spec 076) is not modified here.

## Deferred Reason (if mode: document)
N/A — driven to terminal via `bugfix-fastlane`.
