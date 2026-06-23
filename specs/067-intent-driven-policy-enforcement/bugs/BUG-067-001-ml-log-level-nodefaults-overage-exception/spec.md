# Spec: BUG-067-001 — ML_LOG_LEVEL SST fail-loud + over-age exception elimination

## Context
Spec 067 (Intent-Driven Policy Enforcement) owns `policy-exception-baseline.json` and the NO-DEFAULTS policy guards under `tests/integration/policy/`. This bug formalizes and permanently fixes two entangled findings spec 067's own gaps + harden phases surfaced and routed out at certification (`GAP-067-G01`, `GAP-067-G02`).

The fix is SST-ification: land the missing `ml.log_level` SST key, wire it into config generation, make the ML sidecar read it fail-loud, and delete the policy exception entirely (rather than re-issuing a shorter expiry). Then close the test hole that masked the live defect.

## Product Principle Alignment
- **Engineering constitution NO-DEFAULTS / fail-loud SST (`smackerel-no-defaults`):** the ML sidecar log level becomes an SST value with a fail-loud read; no literal fallback. This is the principle the bug restores.
- This bug touches framework/operator governance surfaces only (config SST plumbing, Python config validation, policy-guard tests). No product Principle 1–10 behavior changes.

## Expected Behavior (Requirements)

### REQ-1 — `ml.log_level` is an SST-owned key
`config/smackerel.yaml` declares `services.ml.log_level` (an ml-owned key, distinct from the core `runtime.log_level`). Config generation resolves it via the existing `required_value` path (fail-loud if missing/empty).

### REQ-2 — `ML_LOG_LEVEL` is emitted into every environment
`./smackerel.sh config generate` emits `ML_LOG_LEVEL=<value>` into `config/generated/dev.env` AND `config/generated/test.env`, and the same single emission point carries it into the deploy bundle `app.env` (the sidecar boots in every environment).

### REQ-3 — The ML sidecar reads `ML_LOG_LEVEL` fail-loud
`ml/app/main.py` adds `ML_LOG_LEVEL` to `_check_required_config()`'s required keys and reads it WITHOUT a default (`os.environ["ML_LOG_LEVEL"].upper()`). A missing/empty/invalid value exits non-zero. No `# smackerel:policy-exception` markers remain in `ml/app/main.py`, and no `os.environ.get("ML_LOG_LEVEL", <literal>)` fallback remains.

### REQ-4 — The policy exception is eliminated
`policy-exception-baseline.json` no longer contains `G067-A05-ml-log-level`. With the SST key landed and the read fail-loud, no exception is needed, so the over-age `G067-A07` condition is cleared at the root.

### REQ-5 — Policy guard tests validate the REAL baseline at the REAL cap
The four real-baseline policy guard tests no longer override `ExceptionMaxAgeDays` to magic constants (`365*10` / `180`); they use the real SST cap (`policy.policy_exception_max_age_days = 90`, read from config — not re-hardcoded). An adversarial regression proves the guard has teeth: a fixture exception ~162 days out MUST be flagged `G067-A07` at the real cap, and the real committed (now exception-free) baseline MUST validate clean.

## BDD Scenarios

### Scenario SCN-BUG-067-001-001 — ML sidecar fails loud when ML_LOG_LEVEL is missing
```gherkin
Given the ML sidecar required-config validator
When ML_LOG_LEVEL is unset (or empty)
Then _check_required_config() exits non-zero (sys.exit(1))
And the error names ML_LOG_LEVEL as missing
```

### Scenario SCN-BUG-067-001-002 — ML sidecar applies a valid SST log level without any default fallback
```gherkin
Given every required ML sidecar env var is set including ML_LOG_LEVEL=info
When _check_required_config() runs
Then it returns the resolved config carrying ML_LOG_LEVEL
And ml/app/main.py contains no os.environ.get("ML_LOG_LEVEL", <literal>) fallback
And ml/app/main.py contains no # smackerel:policy-exception marker
```

### Scenario SCN-BUG-067-001-003 — config generation emits ML_LOG_LEVEL for dev and test
```gherkin
Given config/smackerel.yaml declares services.ml.log_level
When ./smackerel.sh config generate runs for dev and for test
Then config/generated/dev.env contains a non-empty ML_LOG_LEVEL=
And config/generated/test.env contains a non-empty ML_LOG_LEVEL=
```

### Scenario SCN-BUG-067-001-004 — the committed baseline is clean at the real SST cap
```gherkin
Given the committed policy-exception-baseline.json
And the real SST cap policy.policy_exception_max_age_days = 90 read from config
When every baseline exception is validated via ValidateException at the real cap
Then no exception is flagged (G067-A05-ml-log-level has been removed)
And PythonNoDefaultsGuard over the real ml/app/ tree produces zero findings at the real cap
```

### Scenario SCN-BUG-067-001-005 — the guard flags a future over-age exception (adversarial, RED-if-reintroduced)
```gherkin
Given a synthetic policy exception whose expires_on is ~162 days out (the shape of the removed entry)
When it is validated via ValidateException at the real 90-day cap
Then it is flagged with rule_id G067-A07 (over the cap)
And a synthetic exception 80 days out is NOT flagged (no false positive)
```

## Hard Constraints
1. NO-DEFAULTS / fail-loud everywhere — no `${VAR:-default}`, no `os.getenv(k, default)`, no `unwrap_or`.
2. Test integrity (spec 067 Hard Constraint 2) — real-baseline tests validate at the real SST cap; the adversarial case must FAIL if the bug is reintroduced (non-tautological, no bailout).
3. Terminal discipline — runtime ops via `./smackerel.sh`; governance via committed bubbles scripts; no hand-editing `config/generated/*`.
4. No live stack — the verification surface (policy guard integration tests = pure file-system; ml Python unit tests; config-generation static checks) runs without Postgres/NATS/Ollama/core/ml.

## Out of Scope
- `GAP-067-G03` (spec 067 Scope 3 Test Plan names a non-existent e2e test) — a separate spec-067 artifact finding, not part of this bug.
- `specs/076-.../BUG-076-001` (ML agent logs raw conversational content) — foreign-owned, orthogonal (log content vs log level); not modified.
