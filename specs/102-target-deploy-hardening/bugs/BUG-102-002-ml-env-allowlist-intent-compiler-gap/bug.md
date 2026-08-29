# BUG-102-002 — ml.env projection omits the sidecar's boot-required `ASSISTANT_INTENT_COMPILER_*` keys

- **Parent spec:** [`specs/102-target-deploy-hardening`](../../spec.md) (owns
  `services.ml.env_allowlist`, the compute-only `ml.env` projection).
- **Severity:** S1 — DEPLOY-BLOCKING. Every `self-hosted` config bundle produces
  an `ml.env` that makes `smackerel-ml` exit 1 about one second after start, so
  `smackerel-core` never clears ML readiness, `/api/health?strict=true` never
  reaches 200, and the knb adapter's `ollama-pre-retirement-proof` apply gate
  refuses. No self-hosted deploy can complete.
- **Discovered:** 2026-08-29, promoting the `local-operator` build of `cf005ed4`
  to the `<deploy-target>` target.
- **Owner:** bubbles.devops

## Symptom

The ML sidecar container starts, logs one error, and exits 1:

```
ERROR smackerel-ml Missing required configuration: ASSISTANT_INTENT_COMPILER_ENABLED,
ASSISTANT_INTENT_COMPILER_MODEL_ROLE, ASSISTANT_INTENT_COMPILER_PROVIDER_NAME,
ASSISTANT_INTENT_COMPILER_PROVIDER_URL, ASSISTANT_INTENT_COMPILER_PROMPT_CONTRACT_VERSION,
ASSISTANT_INTENT_COMPILER_SCHEMA_VERSION, ASSISTANT_INTENT_COMPILER_TIMEOUT_MS
File "/app/app/main.py", line 145, in _check_required_config → sys.exit(1)
```

`smackerel-core` then blocks on ML readiness, strict health never returns 200,
and the adapter's apply gate refuses. The previous release stays live because the
adapter rolls back, so the outward symptom is "the deploy never lands", not "the
product is down".

## Reproduction

1. Generate a self-hosted config bundle:
   `./smackerel.sh config generate --env self-hosted --bundle --output-dir <dir> --source-sha <sha>`
2. Extract the bundle and grep the projected sidecar env file:
   `grep -c '^ASSISTANT_INTENT_COMPILER_' ml.env` → **0** (pre-fix).
   The sibling `app.env` carries all 11 of them.
3. Promote that bundle. `smackerel-ml` exits 1 within ~1s; `docker logs` shows
   the missing-configuration error above.

The defect is fully reproducible offline from step 2 alone — no host required.

## Root cause (one line)

`services.ml.env_allowlist` in `config/smackerel.yaml` has no `ASSISTANT_*`
coverage, so the `ml.env` projection filters out the seven
`ASSISTANT_INTENT_COMPILER_*` keys that `ml/app/main.py::_check_required_config`
requires unconditionally. See [design.md](design.md) for the full chain.

## Why no guard caught it

`internal/deploy/bundle_secret_contract_test.go::TestMLEnv_ContainsEveryComputeKey_Spec102`
exists precisely to prevent "the sidecar exits 1 on boot". It did not fire
because its expected set was a **hand-maintained list of Go string literals** that
drifted from `ml/app/main.py`. Two hand-maintained lists (the allowlist and the
guard's expectation) both had to be updated when BUG-069-005 added the seven
keys; neither was.

## Latency

This is **latent, not new**. The seven keys became unconditionally required in
`_check_required_config` when BUG-069-005 landed. Every self-hosted bundle built
since then carries the same defect — it blocked the `02697bfd` build from
deploying as well, not just `cf005ed4`. The build, signing, SBOM/Trivy, and
provenance chain were all healthy the whole time; only the projected sidecar env
was wrong.

## Related

- **BUG-069-005** — introduced the seven required keys in the sidecar.
- **Spec 102 SCOPE-102-01** — owns the projection and the allowlist.
- `specs/102-target-deploy-hardening/uservalidation.md` carries an unchecked
  item, "`smackerel-ml` still starts cleanly against `ml.env` — no missing-env
  fail-loud error (it has every compute key it actually reads)", which is exactly
  this regression. This bug closes it.
