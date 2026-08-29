# BUG-102-002 — Specification: projected `ml.env` must satisfy the sidecar's boot contract

## Scope

This packet fixes the compute-only env projection for `smackerel-ml` and removes
the class of drift that hid the defect. It changes no product behavior beyond
letting the sidecar boot.

## Functional requirements

**FR-102-002-1 — The projection covers every boot-required key.**
The `ml.env` artifact staged into a config bundle MUST contain every environment
key whose absence makes `smackerel-ml` terminate at startup. Concretely, for
every key in `ml/app/main.py::_check_required_config`'s unconditional `keys`
list, and for every `os.environ["KEY"]` subscript read under `ml/app/`, `ml.env`
MUST carry a `KEY=` line.

**FR-102-002-2 — The seven intent-compiler keys are projected.**
`ASSISTANT_INTENT_COMPILER_ENABLED`, `_MODEL_ROLE`, `_PROVIDER_NAME`,
`_PROVIDER_URL`, `_PROMPT_CONTRACT_VERSION`, `_SCHEMA_VERSION`, and `_TIMEOUT_MS`
MUST be present in the projected `ml.env` for the `self-hosted` environment.

**FR-102-002-3 — Secret isolation is preserved.**
The fix MUST NOT weaken spec 102 SCOPE-102-01. The projected `ml.env` MUST still
contain no member of `SHELL_SECRET_KEYS`, no `POSTGRES_*` connection part, and no
`DATABASE_URL`. The generator's allowlist∩secret tripwire and its
credential-suffix tripwire MUST remain effective. The isolated-ML-sidecar
boundary is unchanged: the Python tier receives no database, cache, or queue
credential.

**FR-102-002-4 — The boot-safety guard derives, it does not duplicate.**
`TestMLEnv_ContainsEveryComputeKey_Spec102` MUST compute its expected key set
from the sidecar sources rather than restating it as literals. A hand-maintained
expectation is the defect, not the fix.

**FR-102-002-5 — The guard is adversarial.**
There MUST be a test that fails when (a) the sidecar begins requiring a key the
allowlist does not project, and (b) the allowlist stops projecting a key the
sidecar requires. A guard that cannot fail is not a guard.

**FR-102-002-6 — The derivation cannot pass vacuously.**
If the derivation's parse of the sidecar source yields an implausibly small set,
the guard MUST fail loudly rather than assert nothing.

## Behavioural scenarios

```gherkin
Scenario: SCN-102-002-A — the sidecar boots against the projected env
  Given a self-hosted config bundle generated from the current SST
  When smackerel-ml starts with env_file ./ml.env
  Then _check_required_config finds every required key
  And the sidecar reaches "Application startup complete"
  And GET /health returns 200
  And smackerel-core clears ML readiness so /api/health?strict=true returns 200

Scenario: SCN-102-002-B — the projection carries the intent-compiler contract
  Given a generated self-hosted config bundle
  When ml.env is extracted from the bundle
  Then it contains all seven ASSISTANT_INTENT_COMPILER_* boot-required keys

Scenario: SCN-102-002-C — secret isolation survives the fix
  Given a generated self-hosted config bundle
  When ml.env is extracted from the bundle
  Then it contains no SHELL_SECRET_KEYS member
  And it contains no POSTGRES_* connection part
  And it contains no DATABASE_URL

Scenario: SCN-102-002-D — a new boot requirement without allowlist coverage fails
  Given the sidecar's boot-required list is extended with a key no allowlist entry matches
  When the boot-safety guard derives its expected set and checks the projection
  Then the guard reports that key as missing and fails

Scenario: SCN-102-002-E — an allowlist regression fails
  Given services.ml.env_allowlist no longer declares ASSISTANT_INTENT_COMPILER_*
  When a config bundle is generated and checked against the derived expected set
  Then the guard reports the seven boot-required keys as missing and fails

Scenario: SCN-102-002-F — a broken derivation fails loudly
  Given the parser can no longer locate the sidecar's boot-required list
  When the guard runs
  Then it fails with DERIVATION BROKEN rather than asserting an empty set
```

## Out of scope

- Deploy timeouts. The ML cold start was measured at ~15s against a 240s budget;
  the timing work already shipped in the knb overlay and is not revisited here.
- The four non-boot `ASSISTANT_INTENT_COMPILER_*` tuning keys
  (`_CONFIDENCE_FLOOR`, `_MAX_CONTEXT_TURNS`, `_MAX_OUTPUT_BYTES`,
  `_RETRY_BUDGET`) are projected as a consequence of the prefix entry. They are
  non-secret members of the same compiler contract; no separate requirement is
  claimed for them.
- Request-time fail-loud reads behind per-module helpers (for example
  `ml/app/routes/intent_compile.py::_required`). Those return HTTP 500 rather
  than killing the process and are outside this boot-safety contract.
