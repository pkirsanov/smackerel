# BUG-102-002 — Design: fix the projection, and make the guard derive

## Root cause chain

Each link was read directly from the source, not inferred.

| # | Fact | Evidence |
|---|---|---|
| 1 | `ml/app/main.py::_check_required_config` lists seven `ASSISTANT_INTENT_COMPILER_*` keys as **unconditionally** required; a missing or empty value calls `logger.error(...)` then `sys.exit(1)`. | `ml/app/main.py` lines 60-66 (BUG-069-005 comment) and the `if missing:` branch |
| 2 | `deploy/compose.deploy.yml` gives `smackerel-ml` exactly `env_file: [./ml.env]`; its inline `environment:` block carries only `PROMPT_CONTRACTS_DIR`, `HF_HOME`, `SENTENCE_TRANSFORMERS_HOME`. So `ml.env` is the sidecar's ONLY source for these keys. | the `smackerel-ml` service block |
| 3 | `ml.env` is a filtered projection: `{ KEY=VALUE ∈ app.env : KEY ∈ services.ml.env_allowlist } MINUS (SHELL_SECRET_KEYS ∪ POSTGRES_* ∪ DATABASE_URL)`. | `scripts/commands/config.sh::project_service_env` |
| 4 | The allowlist covered `ML_*`, `OLLAMA_*`, `NATS_*`, `AGENT_*` plus 14 exact keys — **no `ASSISTANT_*` coverage of any kind**. | `config/smackerel.yaml::services.ml.env_allowlist` (pre-fix) |
| 5 | `app.env` DOES carry all 11 `ASSISTANT_INTENT_COMPILER_*` keys populated. | `scripts/commands/config.sh` lines 2958-2968 |

So the keys existed, were correct, and were simply filtered away one step before
the sidecar could read them. Nothing upstream of the projection was wrong: the
image, signature, SBOM, and Trivy gate were all clean.

## Why the guard was silent

`TestMLEnv_ContainsEveryComputeKey_Spec102` asserted a **hand-written** `required`
slice of 15 Go string literals. When BUG-069-005 added the seven keys to the
sidecar, three surfaces had to change and only one did:

| Surface | Kind | Updated by BUG-069-005? |
|---|---|---|
| `ml/app/main.py` required list | source of truth | yes |
| `services.ml.env_allowlist` | derived duplicate | **no** |
| the guard's `required` slice | derived duplicate | **no** |

Two duplicates of one truth, kept in sync by hand. That is the actual defect
class; the missing keys are only its first observable symptom.

## Fix 1 — allowlist (the immediate unblock)

Add one PREFIX entry to `services.ml.env_allowlist`:

```yaml
    - "ASSISTANT_INTENT_COMPILER_*"
```

Design decisions:

- **Prefix, not the seven exact keys.** `project_service_env` supports both an
  exact key and a trailing-glob prefix (`_env_allowlist_match`). The sidecar's
  required set grows in batches — BUG-069-005 added all seven at once — so an
  exact list is the same drift trap that produced this bug. A prefix cannot drift
  when an eighth compiler key appears.
- **Narrow prefix, not `ASSISTANT_*`.** The broad prefix would sweep ~90
  unrelated assistant keys into the least-trusted tier, including
  `ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_API_KEY` and several `*_SECRET_REF` /
  `*_ACCESS_TOKEN_REF` values. The sidecar needs the compiler contract and
  nothing else.
- **Tripwires stay effective.** All eleven `ASSISTANT_INTENT_COMPILER_*` keys are
  non-secret and none carries a credential suffix (`_KEY`, `_TOKEN`, `_SECRET`,
  `_PASSWORD`, `_PASSPHRASE`, `_CREDENTIAL`, `_CREDENTIALS`), so neither the
  allowlist∩`SHELL_SECRET_KEYS` tripwire nor the credential-shape tripwire is
  weakened. A future `ASSISTANT_INTENT_COMPILER_API_KEY` would be refused by the
  credential-shape tripwire rather than silently riding the prefix.
- **Isolated-ML-sidecar boundary unchanged.** No database, cache, or queue
  credential enters the ML tier; the hard subtraction of
  `SHELL_SECRET_KEYS ∪ POSTGRES_* ∪ DATABASE_URL` is unconditional and untouched.

## Fix 2 — make the guard derive (the durable fix)

`TestMLEnv_ContainsEveryComputeKey_Spec102` now computes its expected set from
the sidecar sources. Two derivation rules, both keyed on **fail-loud shapes**:

1. **Boot list.** Parse the `keys = [...]` literal inside
   `ml/app/main.py::_check_required_config`. Comments are stripped first so a `]`
   in a comment cannot truncate the slice, and only `SHOUT_CASE` quoted literals
   are collected.
2. **Subscript reads.** Collect every `os.environ["KEY"]` under `ml/app/`
   (recursively). The subscript form raises `KeyError` when absent, so it is
   genuinely fail-loud.

`os.environ.get(...)` is deliberately **excluded**. That exclusion is
load-bearing, not incidental: `ml/app/keep_bridge.py` reads the managed secret
`KEEP_GOOGLE_APP_PASSWORD` via `.get`, and spec 102 projects it OUT on purpose. A
naive "all env reads" scan would demand that the secret be projected into the
least-trusted tier — turning a boot-safety guard into a security regression.

Also **not** derived: request-time fail-loud reads behind per-module helpers such
as `ml/app/routes/intent_compile.py::_required`. Those return HTTP 500 rather
than terminating the process, so they fall outside this guard's boot-safety
contract; every key they read is already in rule 1's boot list.

The derivation yields 30 keys today, against 15 hand-written before.

### Guarding the guard

A parser that silently matches nothing would make the whole assertion vacuous —
strictly worse than the hand-maintained list, because it would look rigorous. Two
floors prevent that:

- the boot-list parse must yield **at least 12** keys, and
- the subscript scan must find **at least 8** reads,

else the test fails with `DERIVATION BROKEN`. These are sanity bounds on the
PARSE, not a restatement of the expected keys, so they do not reintroduce the
duplication. A failed anchor lookup also fails loudly rather than returning an
empty set.

## Fix 3 — adversarial bite

`TestMLEnv_DerivedComputeKeySet_HasBite_BUG102002` proves the guard can fail, in
the two directions the outage can recur:

- **`sidecar requires an unprojected key`** — injects
  `SIDECAR_BOOT_DRIFT_SENTINEL` into a tampered in-memory copy of `main.py`'s
  boot list (the live file is never mutated) and asserts the coverage check
  reports it missing. This is BUG-069-005 replayed.
- **`allowlist drops the intent-compiler prefix`** — removes the new allowlist
  entry from a tampered copy of `config/smackerel.yaml`, regenerates a real
  bundle in a sandbox repo root, and asserts the derived set is no longer
  satisfied. It additionally asserts that *only* `ASSISTANT_INTENT_COMPILER_*`
  keys drop out, which proves the projection behaves as a per-entry filter rather
  than collapsing.

Both sub-tests exercise the real parser and the real generator. Neither can pass
while the protection is absent.

## Files changed

| File | Change |
|---|---|
| `config/smackerel.yaml` | add `- "ASSISTANT_INTENT_COMPILER_*"` to `services.ml.env_allowlist`; correct the surrounding SST comment (it claimed the membership was validated, and named a key list that had drifted) |
| `internal/deploy/bundle_secret_contract_test.go` | replace the hand-maintained `required` slice with `deriveMLBootRequiredKeys`; add `parseCheckRequiredConfigKeys`, `stripPyFullLineComments`, `mlMissingRequiredKeys`, `generateMLEnvBodyWithYaml`; add `TestMLEnv_DerivedComputeKeySet_HasBite_BUG102002` |

No change to `scripts/commands/config.sh`: the projection logic was already
correct and already supported prefixes. No change to the knb overlay — the fix
belongs in the product's SST, and injecting the keys from the deploy adapter
would have hidden the defect rather than fixed it.

## Rejected alternatives

| Alternative | Why rejected |
|---|---|
| Append the seven keys to the guard's literal list | Recreates the exact drift that caused the outage. Explicitly ruled out. |
| Add an inline `environment:` block for the seven keys in `deploy/compose.deploy.yml` | Bypasses the SST projection, creates a fourth place to keep in sync, and would not fix any future key. |
| Inject the keys from the knb adapter's `app.env` overlay | Violates the product/overlay boundary and leaves every other consumer of the bundle broken. |
| Broaden the allowlist to `ASSISTANT_*` | Sweeps API keys and `*_SECRET_REF` values into the least-trusted tier. |
| Raise the deploy timeout | Not the cause. ML cold start is ~15s against a 240s budget; the sidecar was exiting after ~1s. |
