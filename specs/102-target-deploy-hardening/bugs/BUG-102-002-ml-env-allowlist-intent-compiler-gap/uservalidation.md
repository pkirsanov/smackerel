# User Validation — BUG-102-002

> **Convention (anti-fabrication).** This checklist follows the parent spec's
> rule in [`../../uservalidation.md`](../../uservalidation.md): an item is
> CHECKED `[x]` only where it describes something already proven with real
> evidence recorded in [report.md](report.md). Items that can only be observed
> against a running deployment stay UNCHECKED `[ ]` until the operator confirms
> them after a live apply. Nothing here is pre-checked to look complete.
>
> Uncheck any item that stops being true — the User Validation Gate treats an
> unchecked box as a reported regression.

## Checklist

### Verified in-repo (evidence in [report.md](report.md))

- [x] A freshly generated `self-hosted` config bundle produces an `ml.env` that
  contains all seven boot-required `ASSISTANT_INTENT_COMPILER_*` keys
  (`_ENABLED`, `_MODEL_ROLE`, `_PROVIDER_NAME`, `_PROVIDER_URL`,
  `_PROMPT_CONTRACT_VERSION`, `_SCHEMA_VERSION`, `_TIMEOUT_MS`) — pre-fix this
  count was zero.
  → [Evidence 2](report.md#evidence-2--bundle-projection-carries-the-seven-keys)
- [x] That same `ml.env` still carries no `DATABASE_URL`, no `POSTGRES_*`
  connection part, and no cache/queue credential — the fix widened the
  projection without weakening the isolated-ML-sidecar boundary.
  → [Evidence 2](report.md#evidence-2--bundle-projection-carries-the-seven-keys),
  [Evidence 5](report.md#evidence-5--lint)
- [x] The boot-safety guard no longer restates a hand-maintained key list: it
  derives its expectation from the sidecar's own sources and reports the derived
  count (30 keys) on every run.
  → [Evidence 3](report.md#evidence-3--derived-guard--adversarial-bite)
- [x] Deleting the new allowlist entry makes the guard FAIL and name exactly the
  seven boot-required keys that disappear — i.e. this outage is now reproducible
  under test rather than only in production.
  → [Evidence 3](report.md#evidence-3--derived-guard--adversarial-bite)
- [x] A sidecar that starts requiring a key no allowlist entry covers also makes
  the guard FAIL, so a future eighth key cannot drift in silently.
  → [Evidence 3](report.md#evidence-3--derived-guard--adversarial-bite)
- [x] `./smackerel.sh check`, `./smackerel.sh lint`, and `./smackerel.sh test unit`
  all pass with the fix in place; the config bundle digest is reproducible across
  two consecutive generates of the same source SHA.
  → [Evidence 1](report.md#evidence-1--bundle-regeneration),
  [Evidence 4](report.md#evidence-4--check),
  [Evidence 5](report.md#evidence-5--lint),
  [Evidence 6](report.md#evidence-6--full-unit-suite)

### Awaiting a live apply (operator-confirmed — deliberately unchecked)

These describe a running deployment. No build, promotion, or host interaction was
performed, so none of them is claimed. See
[Evidence 7](report.md#evidence-7--live-deploy).

- [ ] After promoting a build made from the fixed SST, `smackerel-ml` starts and
  stays up — no `Missing required configuration:` error, no `exit 1` about a
  second after start.
- [ ] `smackerel-core` clears ML readiness and `/api/health?strict=true` returns
  200 on the deploy target.
- [ ] All containers report healthy, and the running core AND ml image digests
  match the digests pinned in the new deployment manifest.
- [ ] The `<deploy-host>` adapter's `ollama-pre-retirement-proof` apply gate
  passes instead of refusing, so the deploy actually lands rather than rolling
  back to the previous release.
- [ ] The parent spec's SCN-102-C1 item — "`smackerel-ml` still starts cleanly
  against `ml.env` — no missing-env fail-loud error" — can then be checked in
  [`../../uservalidation.md`](../../uservalidation.md). It is left unchecked
  there by this packet.
