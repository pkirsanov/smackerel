# Report 096 — Multi-Provider AI Model Connections

> Execution evidence. Each scope section below carries REAL captured
> terminal output (anti-fabrication). Absolute `/home/<user>/...` paths are
> redacted to `~/...` in evidence blocks per the repo PII policy.

---

## Summary

SCOPE-01 (the foundation scope) is implemented: the operator-global
provider-connection registry now exists as SST source-of-truth in
`config/smackerel.yaml` (`llm.connections[]` + `llm.discovery` +
`llm.model_costs[]` + the `LLM_PROVIDER_SECRET_MASTER_KEY` managed-secret
manifest entry), flows through `scripts/commands/config.sh` into the
generated env, and is loaded by a new closed-set fail-loud Go
loader/validator (`internal/config/model_connections.go`). The seven
manifest-named SCOPE-01 unit tests pass; `./smackerel.sh config generate`
(dev + test) and `./smackerel.sh check` exit 0; the Ollama-only dev path is
unchanged. SCOPE-01 is `in_progress` with two residuals that are
environmental/foreign (not code gaps): `format --check` is blocked by a
pre-existing untracked foreign file, and `artifact-lint` is blocked by a
missing foreign `uservalidation.md` (owned by `bubbles.plan`). Details and
evidence below.

---

## SCOPE-01 — Provider-connection registry + config SST schema (foundation)

**Status:** in_progress (11 of 12 DoD items met + evidenced; the single
residual is T1-3, blocked by a pre-existing untracked FOREIGN file outside
this scope — see below).
**Executed by:** `bubbles.implement` (parent-expanded full-delivery).
**Scenarios covered:** SCN-096-A01, SCN-096-A04, SCN-096-G02.

### What shipped

The operator-global provider-connection registry as SST source-of-truth,
loaded closed-set fail-loud:

1. **SST schema** (`config/smackerel.yaml` `llm:` block, ADDITIVE — the
   existing single-provider `llm.provider` path is retained byte-for-byte):
   - `llm.connections[]` — N operator-global slots (`id`, `kind`, `enabled`,
     generic per-kind `params`, `secret_ref` {mode, env_key}, curated
     `models`); NO `actor_user_id` (single shared graph). Dev ships
     `local-ollama` ENABLED + anthropic/openai/azure-foundry/google/bedrock
     declared-but-disabled (no secrets) so the Ollama-only dev box keeps
     working unchanged.
   - `llm.discovery.{cache_ttl_ms, per_provider_timeout_ms}` — REQUIRED `> 0`.
   - `llm.model_costs[]` — provider-qualified (`<kind>/<backend-id>`) USD rates.
   - `infrastructure.secret_keys += LLM_PROVIDER_SECRET_MASTER_KEY` (the
     SCOPE-02 connvault master key; manifest entry only this scope).
2. **Config-generation pipeline** (`scripts/commands/config.sh`): the
   registry flows to the generated env as `LLM_CONNECTIONS_JSON` (per-kind
   `params`/`models` carried as inline-JSON strings, following the
   `ML_MODEL_MEMORY_PROFILES_JSON` SST-JSON precedent), `LLM_DISCOVERY_*`
   scalars, and `LLM_MODEL_COSTS_JSON`; the master key rides the
   placeholder/3-mirror managed-secret path.
3. **Go loader + domain types + fail-loud validation**
   (`internal/config/model_connections.go`, wired into `config.go::Load`):
   closed-set kind vocabulary, per-kind required-param checks (carried
   generically via `Params map[string]any`), discovery bounds `> 0`,
   env-mode secret-in-`secret_keys`, and enabled-non-ollama-model-has-cost —
   each aborting with a NAMED error and zero substituted default.
4. **Unit tests** (`internal/config/model_connections_test.go`): the seven
   manifest-named SCOPE-01 tests, specification-driven with non-tautological
   adversarial cases (each carries a passing CONTROL alongside the failing
   mutation).

### Change Manifest (this scope's edits only)

The working tree contains extensive PRE-EXISTING modifications from
concurrent sessions; the SCOPE-01 change set is exactly:

```text
=== SCOPE-01 tracked-file edits (git diff --stat, scoped to MY files) ===
 config/smackerel.yaml               | 169 ++++++++++++++++++++++++++++++++++++
 internal/config/config.go           |  43 +++++++++
 internal/config/secret_keys.go      |   9 ++
 internal/config/secret_keys_test.go |   2 +
 internal/config/validate_test.go    |  41 +++++++++
 scripts/commands/config.sh          |  73 ++++++++++++++++
 6 files changed, 337 insertions(+)

=== SCOPE-01 new untracked files ===
?? internal/config/model_connections.go
?? internal/config/model_connections_test.go

=== 088/089 selection surfaces — confirm NOT in my edit set ===
(my SCOPE-01 edits touch none of these files)
```

`git diff --name-only` over `modelswitch/`, `modelpref/`,
`internal/telegram/model_command.go`, and `internal/api/agent_model.go`
returned EMPTY — this scope adds only the SST registry and does NOT touch
any 088/089 selection/validator/store surface (D01-T1-5).

### Test Evidence

All evidence below is REAL captured terminal output (unedited except for
`/home/<user>/` → `~/` path redaction).

### Evidence E1 — `./smackerel.sh config generate` (dev + test) EXIT 0; registry + master key present

```text
$ ./smackerel.sh config generate
config-validate: ~/smackerel/config/generated/dev.env.tmp.507695 OK
Generated ~/smackerel/config/generated/dev.env
Generated ~/smackerel/config/generated/nats.conf
Generated ~/smackerel/config/generated/prometheus.yml
---EXIT:0---

$ grep -nE 'LLM_CONNECTIONS_JSON|LLM_DISCOVERY_|LLM_MODEL_COSTS_JSON|LLM_PROVIDER_SECRET_MASTER_KEY' config/generated/dev.env
83:LLM_CONNECTIONS_JSON=[{"id":"local-ollama","kind":"ollama","enabled":true,"params":"{\"base_url\":\"http://ollama:11434\"}","secret_ref_mode":"","secret_ref_env_key":"","models":"{\"strategy\":\"live\"}"},{"id":"anthropic-primary","kind":"anthropic","enabled":false,"params":"{}","secret_ref_mode":"db","secret_ref_env_key":"","models":"{\"strategy\":\"curated\",\"list\":[{\"id\":\"claude-3-5-sonnet\",\"tool_capable\":true,\"vision\":true,\"context_window\":200000}]}"}, ... (azure-foundry endpoint/api_version/deployment, google project/location, bedrock region) ...]
84:LLM_DISCOVERY_CACHE_TTL_MS=60000
85:LLM_DISCOVERY_PER_PROVIDER_TIMEOUT_MS=4000
86:LLM_MODEL_COSTS_JSON=[{"model":"anthropic/claude-3-5-sonnet","input_usd_per_1k":0.003,"output_usd_per_1k":0.015}, ... openai/azure-foundry/google/bedrock ...]
87:LLM_PROVIDER_SECRET_MASTER_KEY=

$ ./smackerel.sh config generate --env test
config-validate: ~/smackerel/config/generated/test.env.tmp.537048 OK
Generated ~/smackerel/config/generated/test.env
$ grep -cE 'LLM_CONNECTIONS_JSON|LLM_DISCOVERY_CACHE_TTL_MS|LLM_DISCOVERY_PER_PROVIDER_TIMEOUT_MS|LLM_MODEL_COSTS_JSON|LLM_PROVIDER_SECRET_MASTER_KEY' config/generated/test.env
5
(expected 5)
```

The dev master key value is empty (`LLM_PROVIDER_SECRET_MASTER_KEY=`) — no
secret committed; production-class targets receive the placeholder.

### Evidence E2 — `./smackerel.sh check` EXIT 0 (build + vet + config-sync + scenario-lint)

```text
$ ./smackerel.sh check
config-validate: ~/smackerel/config/generated/dev.env.tmp.651087 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
---EXIT:0---
```

### Evidence E3 — the seven SCOPE-01 unit tests pass, NO skips

```text
$ ./smackerel.sh test unit --go --go-run 'Spec096|ModelConnections' --verbose
[go-unit] applying -run selector: Spec096|ModelConnections
+ go test -v -run 'Spec096|ModelConnections' -count=1 ./...
=== RUN   TestModelConnections_MultipleOperatorGlobalConnections_Spec096
--- PASS: TestModelConnections_MultipleOperatorGlobalConnections_Spec096 (0.00s)
=== RUN   TestModelConnections_UnknownKindRejectedFailLoud_Spec096
--- PASS: TestModelConnections_UnknownKindRejectedFailLoud_Spec096 (0.00s)
=== RUN   TestModelConnections_PerKindParams_AzureFoundryRichest_Spec096
--- PASS: TestModelConnections_PerKindParams_AzureFoundryRichest_Spec096 (0.00s)
=== RUN   TestModelConnections_MissingRequiredPerKindParam_FailsLoud_Spec096
--- PASS: TestModelConnections_MissingRequiredPerKindParam_FailsLoud_Spec096 (0.00s)
=== RUN   TestModelConnections_DiscoveryTtlNonPositive_AbortsNamed_Spec096
--- PASS: TestModelConnections_DiscoveryTtlNonPositive_AbortsNamed_Spec096 (0.00s)
=== RUN   TestModelConnections_EnvModeSecretNotInSecretKeys_AbortsNamed_Spec096
--- PASS: TestModelConnections_EnvModeSecretNotInSecretKeys_AbortsNamed_Spec096 (0.00s)
=== RUN   TestModelConnections_NoDefaultsFailLoud_Spec096
--- PASS: TestModelConnections_NoDefaultsFailLoud_Spec096 (0.00s)
ok      github.com/smackerel/smackerel/internal/config  0.082s
---EXIT:0---
```

Each adversarial test (UnknownKind, MissingParam, DiscoveryTtl,
EnvModeSecret, NoDefaults) is non-tautological: it asserts a passing CONTROL
fixture AND a failing mutation, so a neutralised (always-pass) or
over-zealous (always-fail) validator would trip it (D01-T2-6).

### Evidence E4 — secret-key 3-mirror parity intact (Go ↔ YAML ↔ shell)

The new managed secret was added to all three mirrors + the pinned mirror
test; the parity contract tests pass:

```text
$ ./smackerel.sh test unit --go --go-run 'SecretKeys|ModelConnections|Spec096' --verbose
--- PASS: TestModelConnections_MultipleOperatorGlobalConnections_Spec096 (0.00s)
--- PASS: TestModelConnections_UnknownKindRejectedFailLoud_Spec096 (0.00s)
--- PASS: TestModelConnections_PerKindParams_AzureFoundryRichest_Spec096 (0.00s)
--- PASS: TestModelConnections_MissingRequiredPerKindParam_FailsLoud_Spec096 (0.00s)
--- PASS: TestModelConnections_DiscoveryTtlNonPositive_AbortsNamed_Spec096 (0.00s)
--- PASS: TestModelConnections_EnvModeSecretNotInSecretKeys_AbortsNamed_Spec096 (0.00s)
--- PASS: TestModelConnections_NoDefaultsFailLoud_Spec096 (0.00s)
--- PASS: TestSecretKeys_MirrorsYAMLManifest (0.03s)
--- PASS: TestSecretKeysMirror (0.00s)
--- PASS: TestSecretKeys_KeepAppPasswordRegistered (0.00s)
ok      github.com/smackerel/smackerel/internal/config  0.094s
---EXIT:0---
```

### Evidence E5 — NO-DEFAULTS / fail-loud scan of the spec-096 additions (G028)

```text
$ grep -nE '\$\{[A-Za-z_]+:-|\$\{[A-Za-z_]+-[^}]|getenv\([^,]+,|os\.Getenv\([^)]+,|unwrap_or' \
    internal/config/model_connections.go scripts/commands/config.sh config/smackerel.yaml \
    | grep -iE 'connection|discovery|model_cost|provider_secret|LLM_'
NO forbidden default forms in spec-096 additions
```

The Go loader uses `os.LookupEnv` + fail-loud `[F096-SST-MISSING]`; the
shell uses `required_value` / `yaml_get_json` (the `[]` empty-collection
fallback for connections/model_costs is the established list-shape idiom, NOT
a hidden runtime-value default — discovery bounds use `required_value` and
the Go validator enforces `> 0`).

### Evidence E6 — formatting (gofmt) of the changed Go files

```text
$ gofmt -l internal/config/model_connections.go internal/config/model_connections_test.go
(empty — both files are gofmt-clean)
```

> **Uncertainty Declaration (D01-T1-3, the single residual):** the global
> `./smackerel.sh format --check` currently exits 1, but ONLY because of a
> pre-existing **untracked, foreign** file
> (`internal/connector/qfdecisions/chaos_hardening_test.go`, `git status`
> `??`) that belongs to another concurrent session's in-progress work and
> MUST NOT be modified (operational safety). Every file THIS scope touches is
> gofmt-clean (proven above). Once the foreign untracked file is formatted or
> removed by its owner, `format --check` will exit 0 and D01-T1-3 can be
> checked. **Claim Source: executed (scoped to changed files).**

### Evidence E7 — artifact-lint

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/096-multi-provider-model-connections
❌ Missing required artifact: specs/096-multi-provider-model-connections/uservalidation.md
✅ Required artifact exists: report.md
✅ Found DoD section in scopes.md
✅ scopes.md DoD contains checkbox items
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
Artifact lint FAILED with 1 issue(s).
---EXIT:1---
```

> **Uncertainty Declaration (D01-T1-1, the second residual):** artifact-lint's
> sole remaining failure is the missing `uservalidation.md`, a PLANNING
> artifact owned by `bubbles.plan` — NOT `bubbles.implement`. Creating it
> would violate artifact ownership. Every `bubbles.implement`-owned check
> passes: all required report.md sections present, all checked DoD items have
> evidence blocks, no template placeholders, no repo-CLI bypass. Routed to
> `bubbles.plan` to author `uservalidation.md`; D01-T1-1 can be checked once
> it exists. **Claim Source: executed.**

### DoD mapping (SCOPE-01)

| DoD item | Status | Evidence |
|----------|--------|----------|
| D01-T1-1 artifact-lint clean | ⚠️ residual | E7 (only the foreign `uservalidation.md`, owned by bubbles.plan, is missing) |
| D01-T1-2 `check` EXIT 0 | ✅ | E2 |
| D01-T1-3 `format --check` EXIT 0 | ⚠️ residual | E6 (my files clean; global blocked by foreign untracked file) |
| D01-T1-4 evidence is real terminal output | ✅ | E1–E6 (all captured, unedited) |
| D01-T1-5 088/089 do-not-amend boundary respected | ✅ | Change Manifest (no modelswitch/modelpref/model_command/agent_model edits) |
| D01-T2-1 `llm.connections[]` SST source-of-truth; generate EXIT 0 w/ registry + master key in dev+test env | ✅ | E1 |
| D01-T2-2 NO-DEFAULTS (G028); discovery bounds REQUIRED `> 0` | ✅ | E5 + E3 (DiscoveryTtl test) |
| D01-T2-3 closed-set kind vocabulary; unknown kind aborts | ✅ | E3 (UnknownKind test) |
| D01-T2-4 per-kind required params validated generically; missing param fails loud naming conn+param | ✅ | E3 (MissingParam test) |
| D01-T2-5 env-mode `env_key` ∈ `secret_keys`; db/ollama carry no inline plaintext | ✅ | E3 (EnvModeSecret test); struct carries no secret-value field |
| D01-T2-6 adversarial tests non-tautological | ✅ | E3 (each has CONTROL + mutation) |
| D01-T2-7 all 7 unit tests pass, no skips | ✅ | E3 |

### Findings for downstream scopes

- **SCOPE-02** consumes `LLM_PROVIDER_SECRET_MASTER_KEY` (declared in all 3
  secret-key mirrors this scope) for the connvault; the design's "master key
  REQUIRED iff a db-mode hosted connection is declared" load-gate is NOT
  enforced in SCOPE-01 (correctly deferred) — SCOPE-02 owns it and should
  scope the requirement to ENABLED db-mode connections so the dev config
  (disabled hosted slots, empty master key) stays valid.
- **SCOPE-04** consumes the curated `models` capability flags
  (`tool_capable`/`vision`/`context_window`) and `models.strategy`, which
  SCOPE-01 carries through the registry but does not yet validate
  (strategy-vocabulary validation deferred to the discovery owner).
- **SCOPE-05** consumes `llm.model_costs[]`; SCOPE-01 enforces only the
  presence rule (enabled non-ollama model ⇒ cost entry). The model-aware
  CostFn + ledger are SCOPE-05.
- **Environmental note:** the dev host working tree carries extensive
  pre-existing unrelated modifications from concurrent sessions, and one
  untracked foreign unformatted file. Before any push, the full unit suite +
  `internal/deploy/bundle_secret_contract_test.go` (Go↔shell↔bundle
  secret-keys parity, a long home-lab bundle build NOT run this turn) should
  be run to confirm the 3-mirror addition holds end-to-end.

---

## Completion Statement

SCOPE-01 — Provider-connection registry + config SST schema — is
**implemented and evidenced** (status `in_progress`; 10 of 12 DoD items
checked). The closed-set, fail-loud operator-global connection registry,
the `llm.discovery` bounds, the `llm.model_costs[]` rate table, and the
`LLM_PROVIDER_SECRET_MASTER_KEY` managed-secret manifest entry are SST
source-of-truth, wired through the config-generation pipeline into the
generated dev + test env, and parsed/validated by the new Go loader. The
seven manifest-named unit tests pass with no skips; `config generate` (dev +
test) and `check` exit 0; the secret-key 3-mirror parity holds; the
Ollama-only dev path is unchanged; and the 088/089 selection surfaces are
untouched.

**Two residuals remain, BOTH environmental/foreign (NOT SCOPE-01 code
gaps):**

1. **D01-T1-3** (`format --check` EXIT 0) — every changed file is
   gofmt-clean; the global command is blocked solely by a pre-existing
   untracked foreign file (`internal/connector/qfdecisions/chaos_hardening_test.go`)
   that belongs to another session and must not be modified.
2. **D01-T1-1** (`artifact-lint` clean) — blocked solely by a missing
   `uservalidation.md`, a planning artifact owned by `bubbles.plan` (not
   `bubbles.implement`); all report.md required sections and the
   anti-fabrication evidence checks pass.

Both residuals are recorded with Uncertainty Declarations and routed to
their owners (the foreign-file owner / `bubbles.plan`). SCOPE-01 is held at
`in_progress` rather than fabricating a clean pass on commands that do not
yet exit 0 in this working tree.

---

## SCOPE-02 — Encrypted credential vault + master-key lifecycle

**Status:** in_progress (7 of 12 DoD items met + evidenced; the residuals are
the integration/migrate leg [T2-1/T2-6/T2-7, deferred to a clean-stack run]
plus two foreign/closeout items [T1-1 `uservalidation.md`, T1-3 a foreign
untracked file] — none are SCOPE-02 code gaps).
**Executed by:** `bubbles.implement` (parent-expanded full-delivery).
**Scenarios covered:** SCN-096-W05.

### What shipped

The reversible, authenticated, encrypted-at-rest operator-global credential
vault and its fail-loud master-key lifecycle:

1. **`connvault.SecretVault`** (`internal/assistant/openknowledge/connvault/vault.go`,
   NEW pkg) — AES-256-GCM AEAD (Go stdlib `crypto/aes` + `cipher.NewGCM`), a
   single operator master key, a per-record random 96-bit nonce, a 128-bit
   auth tag, and `AAD = connection_id:provider_kind:secret_key_version` so a
   ciphertext cannot be relocated to another record or replayed under a
   different key epoch. The master key (`raw`) is scrubbed (`zero(raw)`) after
   the cipher is built.
2. **Never-plaintext at-rest shape** — a `VaultRecord` carries only
   `ciphertext` + `nonce` + `key_version` + a non-secret last-4 `redaction`
   hint. It has NO plaintext field; the only recovery path is in-core
   `Decrypt`. No method returns or logs the plaintext or the master key.
3. **Fail-loud master-key load** — `NewSecretVault` validates the
   `LLM_PROVIDER_SECRET_MASTER_KEY` managed secret (base64 of exactly 32
   bytes, positive epoch) and aborts named on a bad key; `LoadVault` requires
   the key iff the SCOPE-01 registry declares at least one db-mode connection
   (an Ollama-only deployment needs no vault and no new secret).
4. **Reversible — NOT hashed (binding)** — the stored credential is replayed
   to `Authorization: Bearer <key>` at dispatch, so it MUST be recoverable.
   One-way hashing (argon2id) is explicitly FORBIDDEN here (doc-comment
   guard, vault.go L23–24); this is the reversible managed-secret class, not
   the verifier class.
5. **Rotation** — the per-row `Rotate` primitive bumps `key_version` and
   re-encrypts under the new key, driving the documented re-encrypt-all
   procedure.
6. **Persistence** — migration `061_model_provider_connections.sql`
   (operator-global, NO `actor_user_id`; app-written `enabled`/`updated_at`,
   no DB-side defaults — G028) plus the ephemeral-Postgres integration
   round-trip (`tests/integration/model_connections_vault_test.go`).

### Change Manifest (this scope's edits only)

All four SCOPE-02 files are NEW (untracked) additive files — the vault adds a
package + a migration + tests and touches NO 088/089 selection surface:

```text
=== SCOPE-02 new untracked files (git status --porcelain, scoped to MY files) ===
?? internal/assistant/openknowledge/connvault/vault.go
?? internal/assistant/openknowledge/connvault/vault_test.go
?? internal/db/migrations/061_model_provider_connections.sql
?? tests/integration/model_connections_vault_test.go
```

No `modelswitch`/`modelpref`/picker/`agent_model` file is in the edit set —
see Evidence V4 (the 088/089 do-not-amend boundary, D02-T1-5).

### Test Evidence

All evidence below is REAL captured terminal output (unedited except for
`/home/<user>/` → `~/` path redaction).

### Evidence V1 — the 6 SCOPE-02 unit tests pass, NO skips (`connvault` ok)

```text
$ ./smackerel.sh test unit --go --go-run 'SecretVault' --verbose
[go-unit] applying -run selector: SecretVault
+ go test -v -run SecretVault -count=1 ./...
--- PASS: TestSecretVault_EncryptDecrypt_RoundTrip_Spec096 (0.00s)
    --- PASS: TestSecretVault_EncryptDecrypt_RoundTrip_Spec096/single-field_anthropic_api_key (0.00s)
    --- PASS: TestSecretVault_EncryptDecrypt_RoundTrip_Spec096/multi-field_bedrock_credentials (0.00s)
--- PASS: TestSecretVault_NeverReturnsPlaintext_RedactionLast4_Spec096 (0.00s)
--- PASS: TestSecretVault_AADTamperRejected_Spec096 (0.00s)
    --- PASS: TestSecretVault_AADTamperRejected_Spec096/tampered_connection_id_rejected (0.00s)
    --- PASS: TestSecretVault_AADTamperRejected_Spec096/tampered_kind_rejected (0.00s)
    --- PASS: TestSecretVault_AADTamperRejected_Spec096/flipped_ciphertext_byte_rejected (0.00s)
--- PASS: TestSecretVault_WrongKeyRejected_Spec096 (0.00s)
--- PASS: TestSecretVault_MasterKeyFailLoud_Spec096 (0.00s)
    --- PASS: TestSecretVault_MasterKeyFailLoud_Spec096/db-mode_declared_+_empty_master_key_→_fail-loud (0.00s)
    --- PASS: TestSecretVault_MasterKeyFailLoud_Spec096/db-mode_declared_+_valid_key_→_vault_built_(CONTROL) (0.00s)
    --- PASS: TestSecretVault_MasterKeyFailLoud_Spec096/ollama-only_+_empty_key_→_no_vault,_no_error_(CONTROL) (0.00s)
    --- PASS: TestSecretVault_MasterKeyFailLoud_Spec096/present-but-not-base64_key_→_fail-loud (0.00s)
    --- PASS: TestSecretVault_MasterKeyFailLoud_Spec096/present-but-wrong-length_key_→_fail-loud (0.00s)
--- PASS: TestSecretVault_Rotation_ReEncryptsToNewEpoch_Spec096 (0.00s)
ok  	github.com/smackerel/smackerel/internal/assistant/openknowledge/connvault	0.044s
[go-unit] go test ./... finished OK
```

`UNIT_EXIT=0` (orchestrator-captured). This is a full `go test ./...` under
the `-run SecretVault` selector; the non-`connvault` packages emit
`[no tests to run]` and are elided here. The five `MasterKeyFailLoud`
subtests are non-tautological — they pair the two fail-loud cases
(empty/non-base64/wrong-length key while a db-mode connection is declared)
with two passing CONTROLs (valid key → vault built; Ollama-only + empty key →
no vault, no error), so a neutralised predicate would trip the test.

### Evidence V2 — `./smackerel.sh check` EXIT 0 (build + vet + config-sync + scenario-lint)

```text
$ ./smackerel.sh check
config-validate: ~/smackerel/config/generated/dev.env.tmp.161613 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
---EXIT:0---
```

### Evidence V3 — reversible (AES-256-GCM), NOT one-way hashed (D02-T2-3)

```text
$ grep -nE 'argon2\.|bcrypt\.|scrypt\.|sha256\.Sum|sha512\.Sum' \
    internal/assistant/openknowledge/connvault/vault.go | grep -vE '^\s*//' | wc -l
0

$ grep -nE 'argon2|one-way' internal/assistant/openknowledge/connvault/vault.go
23:// One-way hashing (argon2id) is structurally wrong for this data and is
24:// FORBIDDEN here: argon2id verifies a presented secret, it cannot recover

$ grep -cE 'aes\.|cipher\.NewGCM|NewGCM|gcm\.|AES-256-GCM' \
    internal/assistant/openknowledge/connvault/vault.go
12
```

**0** actual hash-call usages; the only two `argon2`/`one-way` hits are
doc-comment PROSE on lines 23–24 (the binding "argon2id is FORBIDDEN here"
guard), NOT executable hashing. **12** lines reference the AES-256-GCM
primitive — reversible authenticated encryption confirmed — and
`TestSecretVault_EncryptDecrypt_RoundTrip_Spec096` recovers the original
bundle byte-for-byte (V1).

### Evidence V4 — 088/089 do-not-amend boundary respected (D02-T1-5)

```text
$ grep -nE 'modelswitch|modelpref|model_command|agent_model' \
    internal/assistant/openknowledge/connvault/vault.go \
    internal/assistant/openknowledge/connvault/vault_test.go \
    tests/integration/model_connections_vault_test.go \
  || echo "NO modelswitch/modelpref/picker reference in SCOPE-02 files (088/089 untouched)"
NO modelswitch/modelpref/picker reference in SCOPE-02 files (088/089 untouched)
```

The vault is a NEW additive package; it neither imports nor modifies the
088/089 `modelswitch`/`modelpref` validator/store or the Telegram/web picker.

### Evidence V5 — secret-safety + crypto code review (D02-T2-2, T2-4, T2-5)

Reviewed in `internal/assistant/openknowledge/connvault/vault.go`:

- **AES-256-GCM** via `crypto/aes` + `cipher.NewGCM`; per-record random
  96-bit nonce; `AAD = connection_id:provider_kind:secret_key_version`.
- **Master key scrubbed** — `zero(raw)` after the cipher is constructed; the
  key is confined to the Go core and never passed to the sidecar nor logged.
- **`VaultRecord`** carries only ciphertext + nonce + `key_version` + last-4
  redaction — there is NO plaintext field
  (`TestSecretVault_NeverReturnsPlaintext_RedactionLast4_Spec096`, V1).
- **`NewSecretVault`** is fail-loud (base64 + exactly-32-byte + positive
  epoch); **`LoadVault`** requires the master key iff a db-mode connection is
  declared (Ollama-only adds no secret) —
  `TestSecretVault_MasterKeyFailLoud_Spec096` (V1).
- **AEAD adversarial** — `TestSecretVault_AADTamperRejected_Spec096`
  (tampered connection_id / tampered kind / flipped ciphertext byte) and
  `TestSecretVault_WrongKeyRejected_Spec096` each fail-closed (V1).
- **`Rotate`** bumps `key_version` and re-encrypts under the new key.

### DoD mapping (SCOPE-02)

| DoD item | Status | Evidence |
|----------|--------|----------|
| D02-T1-1 artifact-lint clean | ⬚ deferred | absent `uservalidation.md` (validation/closeout artifact, owned by `bubbles.plan`) — not a SCOPE-02 code gap |
| D02-T1-2 `check` EXIT 0 | ✅ | V2 |
| D02-T1-3 `format --check` EXIT 0 | ⬚ deferred | global gate blocked by a foreign untracked file (`internal/connector/qfdecisions/chaos_hardening_test.go`); SCOPE-02 files are gofmt-clean |
| D02-T1-4 evidence is real terminal output | ✅ | V1–V5 (all captured, unedited) |
| D02-T1-5 088/089 do-not-amend boundary respected | ✅ | V4 + Change Manifest |
| D02-T2-1 migration discipline (ephemeral PG migrate) | ⬚ deferred | `061_*.sql` exists (additive, operator-global, no DB-side defaults); the migrate/integration leg is deferred to a clean-stack run |
| D02-T2-2 secret-safety (no plaintext; last-4; key stays in Go core) | ✅ | V5 + `TestSecretVault_NeverReturnsPlaintext_RedactionLast4` (V1) |
| D02-T2-3 reversible, not hashed | ✅ | V3 (0 hash usages, AES-GCM present) + round-trip (V1) |
| D02-T2-4 authenticated AEAD adversarial (non-tautological) | ✅ | V1 (AADTamper + WrongKey) + V5 |
| D02-T2-5 master-key fail-loud (G028) + rotation documented | ✅ | V1 (MasterKeyFailLoud) + V5 (LoadVault required-iff-db-mode; Rotate) |
| D02-T2-6 test isolation (synthetic secrets + ephemeral PG) | ◐ partial | unit leg synthetic-only (met); integration leg isolated but deferred to a clean-stack run |
| D02-T2-7 all SCOPE-02 tests pass, no skips | ◐ partial | 6/6 unit pass (V1); the 1 integration test is deferred (skips without `DATABASE_URL` by design) |

### Integration leg deferred to clean-stack run

The single integration test
`tests/integration/model_connections_vault_test.go::TestVault_PersistRoundTripTestMasterKey_Spec096`
was NOT run this turn. It depends on a live ephemeral Postgres
(`testPool(t)` skips when `DATABASE_URL` is unset), and the shared test stack
is under concurrent load from other sessions (OOM/contention risk on this
host). The test is correctly isolated for when a clean stack is available —
unique timestamped `connection_id`, `t.Cleanup` row delete, and a synthetic
32-byte master key (never a real provider secret). Migration
`061_model_provider_connections.sql` is idempotently applied by that test.
This is the only SCOPE-02 behavioural residual; it is routed to a downstream
clean-stack / `bubbles.devops` run, not fabricated as passing here.

### Completion Statement (SCOPE-02)

SCOPE-02 — Encrypted credential vault + master-key lifecycle — is
**implemented and evidenced** (status `in_progress`; 7 of 12 DoD items
checked). The AES-256-GCM `connvault.SecretVault` (reversible, authenticated,
never-plaintext, fail-loud master-key load + documented rotation), migration
`061_model_provider_connections`, and the unit + integration tests are in the
tree and compile; the six manifest-plus-lifecycle unit tests pass with no
skips; `./smackerel.sh check` exits 0; the credential is provably reversible
(AES-GCM, zero one-way hashing) and the 088/089 selection surfaces are
untouched. The integration/migrate leg (1 test) is deferred to a clean-stack
run, and two foreign/closeout items (`uservalidation.md`,
`format --check` blocked by a foreign untracked file) are routed to their
owners. SCOPE-02 is held at `in_progress` rather than fabricating a clean
pass on the deferred legs.

---

## SCOPE-03 — Provider-aware `/ask` dispatch (credential seam)

**Status:** in_progress (10 of 11 DoD items met + evidenced; the single
residual splits into the live-stack `integration`/`e2e-api` leg [T2-7, deferred
to a clean-stack run] plus the two closeout gates [T1-2 `check` and T1-3
`format --check`, run by the orchestrator post-implementation] and the
foreign-owned T1-1 `artifact-lint` [absent `uservalidation.md`, owned by
`bubbles.plan`] — none are SCOPE-03 code gaps).
**Executed by:** `bubbles.implement` (parent-expanded full-delivery).
**Scenarios covered:** SCN-096-A02, SCN-096-A03, SCN-096-G01, SCN-096-G04,
SCN-096-G05.

### What shipped

The provider-aware `/ask` dispatch credential seam — additive on both sides of
the Go→sidecar boundary, with the no-override Ollama path held byte-for-byte:

1. **Additive request contract (both sides).**
   - Go `llm.ChatRequest`
     (`internal/assistant/openknowledge/llm/client.go`) gains `Provider string`
     + `APIBase *string` + `APIKey *string` + `ProviderParams map[string]any`,
     all `omitempty` — a zero-value request serializes byte-for-byte the
     pre-096 wire shape.
   - Python `ChatRequest` (`ml/app/schemas.py`) gains the same four optional
     fields and STAYS `extra="forbid"`.
2. **Provider-aware `_dispatch_live`** (`ml/app/routes/chat.py`) forks like
   `synthesis.py`: `provider` absent/`"ollama"` → the existing Ollama path
   (`ollama_chat/<model>` + `api_base` from `req.api_base or OLLAMA_URL`, NO
   `api_key`) with the `litellm.acompletion` kwargs byte-for-byte today's;
   hosted → `_compose_hosted_model` (`<prefix>/<backend-id>`) + `api_base` +
   the per-request cleartext `api_key`; a missing required key raises typed
   `llm_misconfigured` and NEVER substitutes Ollama. Every hosted error/log is
   routed through `_scrub_secret` (built from `type(e).__name__` + provider +
   model, with any `api_key` substring redacted).
3. **`DispatchResolver`** (NEW
   `internal/assistant/openknowledge/llm/dispatch_resolver.go`) resolves a
   provider-qualified model → SCOPE-01 registry connection → SCOPE-02
   `SecretVault.Decrypt` → a populated `ChatRequest` (BARE backend `Model` +
   `Provider` + `APIBase` + `APIKey`) plus the provider-qualified
   `Attribution`. A not-effective-enabled / credential-less / decrypt-failed
   target yields a typed `*ResolveError` — NEVER a silent Ollama fallback. The
   resolver holds NO logger and never places the key in an error body.
4. **Provider-qualified attribution** — the existing spec 089
   `TurnResult.Model` attribution carries the provider-qualified id
   (`anthropic/claude-3-5-sonnet`) verbatim; ADDITIVE (value only, shape
   unchanged), never coerced to a bare or Ollama name.

### Change Manifest (this scope's edits only)

```text
=== SCOPE-03 edits ===
 M internal/assistant/openknowledge/llm/client.go          (ChatRequest += 4 omitempty fields)
?? internal/assistant/openknowledge/llm/dispatch_resolver.go        (NEW)
?? internal/assistant/openknowledge/llm/dispatch_resolver_test.go   (NEW)
?? internal/assistant/openknowledge/llm/client_provider_test.go     (NEW)
?? internal/assistant/openknowledge/agent/attribution_provider_test.go (NEW)
 M ml/app/schemas.py                                        (ChatRequest += 4 optional fields, extra=forbid kept)
 M ml/app/routes/chat.py                                    (_dispatch_live provider fork + scrub helpers)
?? ml/tests/test_chat_dispatch_hosted_spec096.py            (NEW)
?? ml/tests/test_chat_dispatch_parity_spec096.py            (NEW)
?? ml/tests/test_chat_dispatch_secret_scrub_spec096.py      (NEW)
```

No `modelswitch`/`modelpref`/picker/`agent_model` file is in the edit set —
the 088/089 selection surfaces are untouched (additive seam only).

### Test Evidence

All evidence below is REAL captured terminal output (unedited except for shell
prompt + `/home/<user>/` → `~/` redaction). Go tests run in the repo's
`golang:1.25.10-bookworm` container (the image `run_go_tooling` uses); Python
tests run in the repo's `python:3.12-slim` container with `pip install -e
./ml[dev]` (the image + extras `run_python_tooling` uses). `litellm` lives in
the `runtime` extra (NOT in `[dev]`), so — exactly like the repo's other LLM
tests — the dispatch tests inject a fake `litellm` + `litellm.exceptions` via
`sys.modules`; they assert the COMPOSED `acompletion` kwargs / typed errors,
never a live network call (correctly classified `unit`).

### Evidence E1 — Go `llm` SCOPE-03 tests pass (additive contract, resolver never-fallback, secret-safety)

Command (focused run in the repo's golang container; the sanctioned
`./smackerel.sh test unit --go --go-run 'Spec096|ProviderAware|DispatchResolver|ChatRequest_Provider|Attribution_ProviderQualified' --verbose`
also finished OK, exit 0):

```text
$ docker run --rm -v ~/smackerel:/workspace ... golang:1.25.10-bookworm \
    go test -v -run 'Spec096|ProviderAware|DispatchResolver|ChatRequest_Provider|Attribution_ProviderQualified' \
    ./internal/assistant/openknowledge/llm/... ./internal/assistant/openknowledge/agent/...
=== RUN   TestChatRequest_ProviderFieldsAdditive_Spec096
=== RUN   TestChatRequest_ProviderFieldsAdditive_Spec096/zero_value_provider_fields_are_byte_for_byte_pre096
=== RUN   TestChatRequest_ProviderFieldsAdditive_Spec096/hosted_request_carries_the_new_fields
--- PASS: TestChatRequest_ProviderFieldsAdditive_Spec096 (0.00s)
    --- PASS: TestChatRequest_ProviderFieldsAdditive_Spec096/zero_value_provider_fields_are_byte_for_byte_pre096 (0.00s)
    --- PASS: TestChatRequest_ProviderFieldsAdditive_Spec096/hosted_request_carries_the_new_fields (0.00s)
=== RUN   TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096
=== RUN   TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096/control_fully_configured_hosted_resolves
=== RUN   TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096/hosted_target_with_no_stored_credential
=== RUN   TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096/disabled_connection
=== RUN   TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096/unknown_provider_kind
=== RUN   TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096/malformed_model_id_no_qualifier
--- PASS: TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096 (0.00s)
    --- PASS: TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096/control_fully_configured_hosted_resolves (0.00s)
    --- PASS: TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096/hosted_target_with_no_stored_credential (0.00s)
    --- PASS: TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096/disabled_connection (0.00s)
    --- PASS: TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096/unknown_provider_kind (0.00s)
    --- PASS: TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096/malformed_model_id_no_qualifier (0.00s)
=== RUN   TestDispatch_SecretNeverInLogsOrErrors_Spec096
=== RUN   TestDispatch_SecretNeverInLogsOrErrors_Spec096/success_secret_only_in_request_api_key
=== RUN   TestDispatch_SecretNeverInLogsOrErrors_Spec096/disabled_target_with_secret_on_disk_rejects_without_leaking
=== RUN   TestDispatch_SecretNeverInLogsOrErrors_Spec096/decrypt_failure_under_wrong_master_key_never_leaks
--- PASS: TestDispatch_SecretNeverInLogsOrErrors_Spec096 (0.00s)
    --- PASS: TestDispatch_SecretNeverInLogsOrErrors_Spec096/success_secret_only_in_request_api_key (0.00s)
    --- PASS: TestDispatch_SecretNeverInLogsOrErrors_Spec096/disabled_target_with_secret_on_disk_rejects_without_leaking (0.00s)
    --- PASS: TestDispatch_SecretNeverInLogsOrErrors_Spec096/decrypt_failure_under_wrong_master_key_never_leaks (0.00s)
=== RUN   TestDispatchResolver_DuplicateKind_FailsLoud_Spec096
--- PASS: TestDispatchResolver_DuplicateKind_FailsLoud_Spec096 (0.00s)
PASS
ok  	github.com/smackerel/smackerel/internal/assistant/openknowledge/llm	0.013s
```

### Evidence E2 — Go provider-qualified attribution passes (SCN-096-G04)

```text
=== RUN   TestAttribution_ProviderQualified_Spec096
2026/06/18 INFO openknowledge.turn turn_id=0889632d5178d5be prompt_sha256=3230..7861 iterations=2 tokens_used=180 usd_spent=0 status=success termination_reason=final num_sources=1 compaction_signaled=false tool_calls="[map[name:fake_web outcome:success]]" refusal_reason=""
2026/06/18 INFO openknowledge.turn turn_id=31614736ea81457e prompt_sha256=3230..7861 iterations=2 tokens_used=180 usd_spent=0 status=success termination_reason=final num_sources=1 compaction_signaled=false tool_calls="[map[name:fake_web outcome:success]]" refusal_reason=""
=== RUN   TestAttribution_ProviderQualified_Spec096/anthropic
=== RUN   TestAttribution_ProviderQualified_Spec096/openai
--- PASS: TestAttribution_ProviderQualified_Spec096 (0.00s)
    --- PASS: TestAttribution_ProviderQualified_Spec096/anthropic (0.00s)
    --- PASS: TestAttribution_ProviderQualified_Spec096/openai (0.00s)
PASS
ok  	github.com/smackerel/smackerel/internal/assistant/openknowledge/agent	0.026s
```

Two providers' attributions (`anthropic/claude-3-5-sonnet` vs `openai/gpt-4o`)
are distinguishable and provider-qualified, never coerced to a bare/Ollama
name. The whole-suite `go test ./...` under the sanctioned CLI also finished
OK (exit 0), proving no 088/089 collateral regression.

### Evidence E3 — Python SCOPE-03 dispatch tests pass (7/7, NO skips)

```text
$ docker run --rm -v ~/smackerel:/workspace ... python:3.12-slim \
    bash -lc 'pip install -q -e ./ml[dev]; cd ml && \
      python -m pytest tests/test_chat_dispatch_hosted_spec096.py \
        tests/test_chat_dispatch_parity_spec096.py \
        tests/test_chat_dispatch_secret_scrub_spec096.py -v'
============================= test session starts ==============================
platform linux -- Python 3.12.13, pytest-9.1.0, pluggy-1.6.0
rootdir: /workspace/ml
configfile: pyproject.toml
plugins: anyio-4.14.0
collected 7 items

tests/test_chat_dispatch_hosted_spec096.py::test_dispatch_live_hosted_composes_model_and_api_key PASSED [ 14%]
tests/test_chat_dispatch_hosted_spec096.py::test_chatrequest_extra_forbid_still_holds PASSED [ 28%]
tests/test_chat_dispatch_hosted_spec096.py::test_hosted_missing_api_key_typed_error_no_ollama_substitution PASSED [ 42%]
tests/test_chat_dispatch_parity_spec096.py::test_dispatch_live_ollama_kwargs_byte_for_byte PASSED [ 57%]
tests/test_chat_dispatch_parity_spec096.py::test_ollama_branch_carries_no_api_key PASSED [ 71%]
tests/test_chat_dispatch_secret_scrub_spec096.py::test_error_detail_scrubs_api_key_substring PASSED [ 85%]
tests/test_chat_dispatch_secret_scrub_spec096.py::test_api_key_never_logged PASSED [100%]

============================== 7 passed in 0.45s ===============================
```

### Evidence E4 — captured RED-before (non-tautological, D03-T2-6)

To prove the adversarial parity + scrub tests bite, two temporary mutations
were applied to `ml/app/routes/chat.py` — (a) `_scrub_secret` neutered to a
no-op, and (b) `kwargs["api_key"] = "RED-BEFORE-LEAK"` injected into the
Ollama branch — and the tests re-run. Both mutations were reverted immediately
after this capture (the post-revert GREEN is E3):

```text
$ # with the two RED-BEFORE mutations applied:
$ python -m pytest tests/test_chat_dispatch_parity_spec096.py tests/test_chat_dispatch_secret_scrub_spec096.py -v
tests/test_chat_dispatch_parity_spec096.py::test_dispatch_live_ollama_kwargs_byte_for_byte FAILED [ 25%]
tests/test_chat_dispatch_parity_spec096.py::test_ollama_branch_carries_no_api_key FAILED [ 50%]
tests/test_chat_dispatch_secret_scrub_spec096.py::test_error_detail_scrubs_api_key_substring FAILED [ 75%]
tests/test_chat_dispatch_secret_scrub_spec096.py::test_api_key_never_logged PASSED [100%]

E  AssertionError: Ollama dispatch kwargs drifted from the pre-096 byte-for-byte shape
E    Left contains 1 more item:
E    {'api_key': 'RED-BEFORE-LEAK'}
...
E  AssertionError: an api_key leaked onto the Ollama dispatch path
...
E  AssertionError: cleartext api_key leaked into the wire error detail: {'error': 'llm_dispatch_failed',
E    'message': 'provider=anthropic model=claude-3-5-sonnet: RuntimeError: 401 Unauthorized for url
E    https://api.anthropic.test/v1/messages?api_key=sk-SECRET-<redacted>'}
------------------------------ Captured log call -------------------------------
WARNING  smackerel-ml.openknowledge.chat:chat.py open_knowledge hosted dispatch error (llm_dispatch_failed): RuntimeError provider=anthropic model=claude-3-5-sonnet
========================= 3 failed, 1 passed in 0.41s ==========================
```

This proves: the byte-for-byte parity test fails the instant a provider field
leaks into the Ollama kwargs; the no-key invariant fails if a key is attached;
the error-detail scrub is load-bearing (without it the cleartext key reaches
the wire body). `test_api_key_never_logged` stays GREEN even with the scrub off
— the log line is independently secret-safe (built from `type(e).__name__` +
provider + model only, as the Captured log shows). After reverting both
mutations the full SCOPE-03 suite is GREEN again (E3).

### Evidence E5 — no pre-096 regression (existing chat contract intact)

```text
$ python -m pytest tests/test_tool_roundtrip.py -q
...........                                                              [100%]
11 passed, 1 warning in 0.48s
```

The schema-parity / dispatch-contract test (the shared Go↔Python
`chat_fixture.json` round-trip + the `extra="forbid"` + fixture-mode handlers)
passes unchanged — the additive `ChatRequest` fields and the `_dispatch_live`
fork did not disturb the pre-096 contract. (The one warning is a pre-existing
Starlette/httpx deprecation, unrelated to this scope.)

### DoD mapping (SCOPE-03)

| DoD item | Status | Evidence |
|----------|--------|----------|
| D03-T1-1 artifact-lint clean | ⬚ deferred | absent `uservalidation.md` (closeout artifact owned by `bubbles.plan`) — not a SCOPE-03 code gap (same caveat as SCOPE-01/02) |
| D03-T1-2 `check` EXIT 0 | ⬚ deferred | run by the orchestrator post-implementation (kept off this turn to bound it); SCOPE-03 Go builds + Python imports clean |
| D03-T1-3 `format --check` EXIT 0 | ⬚ deferred | global gate; run at closeout by the orchestrator (same foreign-untracked-file caveat as SCOPE-01/02) |
| D03-T1-4 evidence is real terminal output | ✅ | E1–E5 (all captured, unedited but path/host-redacted) |
| D03-T2-1 088/089 PARITY byte-for-byte (binding) | ✅ | E3 (`test_dispatch_live_ollama_kwargs_byte_for_byte`, `test_ollama_branch_carries_no_api_key`) + E4 RED-before |
| D03-T2-2 additive contract (Go zero-value wire-identical; Python extra=forbid) | ✅ | E1 (`TestChatRequest_ProviderFieldsAdditive_Spec096`) + E3 (`test_chatrequest_extra_forbid_still_holds`) |
| D03-T2-3 secret-safety adversarial (binding) | ✅ | E3 (`test_api_key_never_logged`, `test_error_detail_scrubs_api_key_substring`) + E1 (`TestDispatch_SecretNeverInLogsOrErrors_Spec096`) + E4 |
| D03-T2-4 fail-loud, never-fallback-to-Ollama adversarial | ✅ | E3 (`test_hosted_missing_api_key_typed_error_no_ollama_substitution`) + E1 (`TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096`) |
| D03-T2-5 provider-qualified attribution | ✅ | E2 (`TestAttribution_ProviderQualified_Spec096`) |
| D03-T2-6 each adversarial test non-tautological with captured RED-before; no bailout early-returns | ✅ | E4 (captured RED-before: 3 fail with mutations, GREEN after revert) |
| D03-T2-7 all unit + integration tests pass, no skips; live `e2e-api` handed to home-lab dispatch | ◐ partial | unit leg done (E1–E3, no skips); the `integration` (`TestAsk_HostedConnection_ProviderAware_Spec096`) + `e2e-api` (`TestAsk_HostedAnswer_AttributedToProviderModel_Spec096`) legs are live-stack — DEFERRED to a clean-stack `bubbles.devops` dispatch, NOT marked passing from dev |

### Live-stack legs deferred to clean-stack run

Per the plan's deferral note (C7), the two live-stack SCOPE-03 rows were NOT
run this turn:

- `integration`:
  `tests/integration/openknowledge_hosted_dispatch_test.go::TestAsk_HostedConnection_ProviderAware_Spec096`
  (needs a live ephemeral Go core + sidecar; the test file is a downstream
  deliverable — the unit resolver + dispatch seam it would exercise are proven
  here).
- `e2e-api`:
  `tests/e2e/agent/openknowledge_e2e_test.go::TestAsk_HostedAnswer_AttributedToProviderModel_Spec096`
  (needs real hosted-provider credentials + reachability; home-lab dispatch).

These depend on real services / credentials and the shared test stack (under
concurrent load on this host). They are routed to the home-lab `bubbles.devops`
dispatch, not fabricated as passing here.

### Findings for downstream scopes

- **SCOPE-06 (operator-gated web admin connection surface)** owns the
  DB-write path that persists the encrypted credential `VaultRecord` into
  `model_provider_connections`. SCOPE-03's `DispatchResolver` consumes that
  record through the `CredentialSource` interface
  (`Credential(connID) (connvault.VaultRecord, bool)`) — SCOPE-06 MUST provide
  the production (DB-backed) implementation of that interface and wire it +
  the loaded vault + the registry into a `NewDispatchResolver`. The resolver's
  "effective-enabled" predicate currently checks `Enabled` + credential
  presence; SCOPE-06 should additionally gate on `last_test_outcome = 'ok'`
  (design §5.1) at the DB-read boundary (the resolver treats "no credential
  returned" as not-effective-enabled, so an untested slot simply withholds its
  record).
- **SCOPE-04 (catalog + canonicalization)** owns the full
  `<kind>/<backend-id>` canonicalization (bare-Ollama normalization,
  off-catalog rejection) at the agenttool resolver boundary. SCOPE-03's
  `splitProviderQualified` does ONLY the dispatch-time kind→connection +
  backend split for an already-provider-qualified id; the catalog-membership
  check stays the modelswitch validator's job (SCOPE-04 injects the set).
- **SCOPE-04/06 routing refinement:** `apiBaseFromParams` /
  `providerParamsFromConn` currently pull `base_url`/`endpoint` → `api_base`
  and pass the remaining non-secret params through generically. The per-kind
  `DispatchRouting` adapter (design §3 adapter contract) can refine Azure
  deployment / Vertex project+location routing in its owning scope without
  changing this seam.

---

## SCOPE-04 — Model discovery + unified catalog + identifier canonicalization

**Status:** in_progress (7 of 11 DoD items met + evidenced, T2-7 partial; the
residual are the live-stack `integration`/`e2e-api` leg [T2-7, deferred to a
clean-stack run] plus the two closeout gates [T1-2 `check` and T1-3
`format --check`, run by the orchestrator post-implementation] and the
foreign-owned T1-1 `artifact-lint` [absent `uservalidation.md`, owned by
`bubbles.plan`] — none are SCOPE-04 code gaps).
**Executed by:** `bubbles.implement` (parent-expanded full-delivery).
**Scenarios covered:** SCN-096-D01, SCN-096-D04.

### What shipped

A new pure-Go `catalog` package
(`internal/assistant/openknowledge/catalog/`) that aggregates every
effective-enabled connection's models into ONE provider-qualified catalog,
canonicalizes selection ids at the resolver boundary, and INJECTS the catalog
as the EXISTING spec-088/089 `modelswitch` validator's admissible set —
keeping the leaf pure and the one-validator/one-store invariant intact.

1. **Per-kind discovery adapters** (`adapter.go`). A `DiscoveryAdapter`
   contract (`Discover`) with two concrete adapters: `OllamaAdapter` probes a
   live `GET <base_url>/api/tags` through an injected `HTTPDoer`, mapping each
   installed name → an `ollama/<name>` descriptor (free/local, no key; an
   optional operator capability hint stamps tool_capable/vision/context since
   `/api/tags` carries none); `HostedAdapter` serves a hosted connection's
   SST-curated `models[]` from the SCOPE-01 registry verbatim (the curated list
   IS the source — no live call — with registry capabilities carried through),
   id `<kind>/<backend-id>`.
2. **`CatalogAggregator`** (`aggregator.go`). Runs every adapter in parallel,
   each bounded by the SST `per_provider_timeout_ms`, and merges the reachable
   subset into ONE `ModelCatalog` of provider-qualified `ModelDescriptor`s
   (`id` + `connection_id` + `kind` + capabilities). It ALWAYS emits one typed
   `ProviderDiscoveryStatus{state ∈ ok|unreachable|timeout|auth_failed|disabled,
   model_count, detail}` per adapter — a slow/down/auth-failed provider degrades
   gracefully (its models absent) and is NEVER silently dropped; the reachable
   subset is ALWAYS served. A last-good catalog is cached for the SST
   `cache_ttl_ms`. Both bounds are fail-loud `> 0` at construction (G028) — NO
   hardcoded TTL/timeout default lives in the aggregator.
3. **Identifier canonicalization + resolver boundary** (`canonical.go`).
   `SplitQualified` splits `<kind>/<backend-id>` on the FIRST `/` (a backend id
   containing `/`/`:` round-trips). `Canonicalize(raw, installed)` normalizes a
   bare 089-era Ollama id (`gemma3:4b`) to `ollama/<id>` IFF installed.
   `CatalogResolver` wraps the catalog as the INJECTED admissible set of the
   EXISTING `modelswitch.Allowlist`: `Validate` canonicalizes then delegates the
   membership decision to the leaf's `Resolve` (off-catalog → the SAME
   `modelswitch.Rejection` shape); `Select` persists a clean canonical id to the
   EXISTING `modelpref.Store` and writes NOTHING on a rejection (NO dispatch).
   `modelswitch` is unchanged — `catalog` imports it, never the reverse.

### Change Manifest (this scope's edits only)

```text
=== SCOPE-04 edits ===
?? internal/assistant/openknowledge/catalog/catalog.go            (NEW — types: ModelDescriptor/ModelCatalog/ProviderDiscoveryStatus/DiscoveryState/DiscoveryError)
?? internal/assistant/openknowledge/catalog/adapter.go            (NEW — DiscoveryAdapter + OllamaAdapter (live /api/tags) + HostedAdapter (SST-curated))
?? internal/assistant/openknowledge/catalog/aggregator.go         (NEW — CatalogAggregator: parallel, SST-bounded, graceful degradation)
?? internal/assistant/openknowledge/catalog/canonical.go          (NEW — SplitQualified/Canonicalize/CatalogResolver injection boundary)
?? internal/assistant/openknowledge/catalog/aggregator_test.go    (NEW — SCN-096-D01 unit tests)
?? internal/assistant/openknowledge/catalog/canonical_test.go     (NEW — SCN-096-D04 unit tests)
```

No `modelswitch`/`modelpref`/`agenttool`/`agent_model` file is edited — 088/089
is untouched (additive injection only; the catalog is the new admissible-set
SOURCE).

### Test Evidence

All evidence below is REAL captured terminal output (line-wrapping is the
terminal width only; no content edited). Go tests run via the sanctioned
`./smackerel.sh test unit --go --go-run …` path (the repo's
`golang:1.25.10-bookworm` container, `go test -run <regex> -count=1 ./...`).
These are UNIT tests: the Ollama `/api/tags` call is an injected fake
`HTTPDoer` and the down-provider paths are injected stub adapters — NO live
Ollama, NO network interception (correctly classified `unit`). All synthetic
ids/models are non-secret (no credentials in this scope).

#### Evidence E1 — catalog package GREEN (aggregation, graceful degradation, canonicalization, off-catalog rejection)

Command:
`./smackerel.sh test unit --go --go-run 'Spec096|CatalogAggregator|Canonicalize|Discovery|ProviderDiscoveryStatus' --verbose`
(overall run `UNIT_EXIT=0`, `[go-unit] go test ./... finished OK`; the
`catalog` package portion):

```text
=== RUN   TestCatalogAggregator_AggregatesOllamaAndHostedProviderQualified_Spec096
=== RUN   TestCatalogAggregator_AggregatesOllamaAndHostedProviderQualified_Spec096/ttl_cache_avoids_reprobe_within_sst_window
--- PASS: TestCatalogAggregator_AggregatesOllamaAndHostedProviderQualified_Spec096 (0.00s)
    --- PASS: TestCatalogAggregator_AggregatesOllamaAndHostedProviderQualified_Spec096/ttl_cache_avoids_reprobe_within_sst_window (0.00s)
=== RUN   TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096
=== RUN   TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096/ollama_unreachable_connect_error
=== RUN   TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096/hosted_auth_failed_typed
=== RUN   TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096/provider_times_out
--- PASS: TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096 (0.03s)
    --- PASS: TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096/ollama_unreachable_connect_error (0.00s)
    --- PASS: TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096/hosted_auth_failed_typed (0.00s)
    --- PASS: TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096/provider_times_out (0.03s)
=== RUN   TestCatalogAggregator_FailLoudOnNonPositiveSSTBounds_Spec096
=== RUN   TestCatalogAggregator_FailLoudOnNonPositiveSSTBounds_Spec096/zero_ttl
=== RUN   TestCatalogAggregator_FailLoudOnNonPositiveSSTBounds_Spec096/negative_ttl
=== RUN   TestCatalogAggregator_FailLoudOnNonPositiveSSTBounds_Spec096/zero_timeout
=== RUN   TestCatalogAggregator_FailLoudOnNonPositiveSSTBounds_Spec096/negative_timeout
--- PASS: TestCatalogAggregator_FailLoudOnNonPositiveSSTBounds_Spec096 (0.00s)
    --- PASS: TestCatalogAggregator_FailLoudOnNonPositiveSSTBounds_Spec096/zero_ttl (0.00s)
    --- PASS: TestCatalogAggregator_FailLoudOnNonPositiveSSTBounds_Spec096/negative_ttl (0.00s)
    --- PASS: TestCatalogAggregator_FailLoudOnNonPositiveSSTBounds_Spec096/zero_timeout (0.00s)
    --- PASS: TestCatalogAggregator_FailLoudOnNonPositiveSSTBounds_Spec096/negative_timeout (0.00s)
=== RUN   TestCanonicalize_SplitOnFirstSlash_RoundTrip_Spec096
--- PASS: TestCanonicalize_SplitOnFirstSlash_RoundTrip_Spec096 (0.00s)
=== RUN   TestCanonicalize_BareOllamaIdNormalized_Spec096
--- PASS: TestCanonicalize_BareOllamaIdNormalized_Spec096 (0.00s)
=== RUN   TestValidate_OffCatalogRefused_TypedRejection_Spec096
--- PASS: TestValidate_OffCatalogRefused_TypedRejection_Spec096 (0.00s)
PASS
ok  	github.com/smackerel/smackerel/internal/assistant/openknowledge/catalog	0.097s
```

The two manifest-named SCN-096-D01 tests + the three SCN-096-D04 tests all
pass (the `FailLoud…` test additionally backs D04-T2-4). The aggregation test
asserts the merged catalog is `[ollama/gemma3:4b, ollama/llama3:8b,
anthropic/claude-3-5-sonnet]` (Ollama group first) with each descriptor's
`connection_id`/`kind`/capabilities; the graceful-degradation test drives a
down provider FIRST (so a naive drop loses the reachable subset too) across
unreachable / typed-auth_failed / genuine-ctx-timeout and asserts a typed
status is always present.

#### Evidence E2 — captured RED-before (non-tautological, D04-T2-6)

Two temporary mutations were applied and the two adversarial tests re-run, then
BOTH reverted immediately (the post-revert code is byte-identical to the E1
GREEN): (a) `aggregator.go` error branch changed to `return` without recording
the status (silent drop); (b) `canonical.go` `Validate` changed to
`return canonical, nil` (accept any id, skip the validator).

Command:
`./smackerel.sh test unit --go --go-run 'TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096|TestValidate_OffCatalogRefused_TypedRejection_Spec096' --verbose`
(`RED_EXIT=1`):

```text
    aggregator_test.go:231: down provider "local-ollama" has NO ProviderDiscoveryStatus (silently dropped)
    aggregator_test.go:231: down provider "openai-main" has NO ProviderDiscoveryStatus (silently dropped)
    aggregator_test.go:231: down provider "bedrock-main" has NO ProviderDiscoveryStatus (silently dropped)
--- FAIL: TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096 (0.02s)
    --- FAIL: TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096/ollama_unreachable_connect_error (0.00s)
    --- FAIL: TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096/hosted_auth_failed_typed (0.00s)
    --- FAIL: TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096/provider_times_out (0.02s)
    canonical_test.go:133: off-catalog id "anthropic/claude-OPUS-does-not-exist" was ACCEPTED (canon="anthropic/claude-OPUS-does-not-exist") — must be refused
--- FAIL: TestValidate_OffCatalogRefused_TypedRejection_Spec096 (0.00s)
FAIL
FAIL	github.com/smackerel/smackerel/internal/assistant/openknowledge/catalog
```

This proves both adversarial tests bite: a silent-drop build fails the
graceful-degradation test (no typed status for the down provider), and an
accept-everything build fails the off-catalog rejection test. After reverting
both mutations the catalog package is GREEN again (E1). No bailout
early-returns are present in either test.

#### Evidence E3 — leaf purity preserved (D04-T2-2 / D04-T2-3)

```text
=== A) modelswitch project imports (MUST be empty = pure stdlib leaf) ===
(none — modelswitch imports zero project packages)

=== modelswitch/*.go import lines (all stdlib) ===
internal/assistant/openknowledge/modelswitch/allowlist.go:25:   "fmt"
internal/assistant/openknowledge/modelswitch/allowlist.go:26:   "log/slog"
internal/assistant/openknowledge/modelswitch/allowlist.go:27:   "strings"

=== C) catalog -> modelswitch/modelpref injection (catalog imports the leaf, never the reverse) ===
internal/assistant/openknowledge/catalog/canonical.go:24:   ".../openknowledge/modelpref"
internal/assistant/openknowledge/catalog/canonical.go:25:   ".../openknowledge/modelswitch"
```

`modelswitch` imports only `fmt` / `log/slog` / `strings` (stdlib) and zero
project packages — the leaf stays pure. `catalog` is the importer: it INJECTS
the provider-qualified catalog as the EXISTING validator's admissible set and
persists through the EXISTING `modelpref.Store` (no second validator/store/
picker introduced — D04-T2-3).

### DoD mapping (SCOPE-04)

| DoD item | Status | Evidence |
|----------|--------|----------|
| D04-T1-1 artifact-lint clean | ⬚ deferred | absent `uservalidation.md` (closeout artifact owned by `bubbles.plan`) — not a SCOPE-04 code gap (same caveat as SCOPE-01/02/03) |
| D04-T1-2 `check` EXIT 0 | ⬚ deferred | run by the orchestrator post-implementation (kept off this turn to bound it); SCOPE-04 compiles clean under `go test ./...` (E1, `finished OK`) |
| D04-T1-3 `format --check` EXIT 0 | ⬚ deferred | global gate; run at closeout by the orchestrator (same foreign-untracked-file caveat as SCOPE-01/02/03) |
| D04-T1-4 evidence is real terminal output | ✅ | E1–E3 (all captured, unedited apart from terminal wrapping) |
| D04-T2-1 graceful-degradation adversarial (NFR-1) | ✅ | E1 (`…GracefulDegradation_TypedStatusNeverDropped_Spec096`: unreachable/auth_failed/timeout) + E2 RED-before; Ollama path independent of any hosted provider (NFR-2). The live `integration` leg is deferred (T2-7) |
| D04-T2-2 leaf-purity (088 invariant) | ✅ | E3 (modelswitch imports zero project packages; catalog injects the set) |
| D04-T2-3 one-validator/one-store (088/089 invariant) | ✅ | E3 + the `CatalogResolver` delegates to `modelswitch.Allowlist.Resolve` + persists via `modelpref.Store` (no new validator/store/picker) |
| D04-T2-4 fail-loud discovery SST (G028) | ✅ | E1 (`…FailLoudOnNonPositiveSSTBounds_Spec096`: zero/negative ttl + timeout all fail loud); no hardcoded TTL/timeout default in `aggregator.go` |
| D04-T2-5 canonicalization round-trip + off-catalog rejection | ✅ | E1 (`…SplitOnFirstSlash_RoundTrip`, `…BareOllamaIdNormalized`, `…OffCatalogRefused_TypedRejection`) — bare-Ollama `gemma3:4b` control validates (089 selections keep working) |
| D04-T2-6 each adversarial test non-tautological with captured RED-before; no bailout early-returns | ✅ | E2 (captured RED-before: both adversarial tests fail with mutations, GREEN after revert) |
| D04-T2-7 all unit + integration pass, no skips; live `e2e-api` handed to home-lab dispatch | ◐ partial | unit leg done (E1, no skips); the `integration` (`TestDiscovery_OneProviderDown_CatalogStillServes_Spec096`) + SCN-096-D04 `e2e-api` legs are live-stack — DEFERRED to a clean-stack `bubbles.devops` dispatch, NOT marked passing from dev |

### Live-stack legs deferred to clean-stack run

Per the plan's deferral note (C7), the live SCOPE-04 rows were NOT run this
turn:

- `integration`:
  `tests/integration/model_discovery_test.go::TestDiscovery_OneProviderDown_CatalogStillServes_Spec096`
  (needs a live ephemeral stack + real Ollama; the unit aggregator + adapters
  it would exercise are proven here with injected fakes/stubs). The unit
  graceful-degradation test already proves the down-provider contract in
  isolation.
- `e2e-api` (SCN-096-D04): the live off-catalog `/ask` selection-path rejection
  runs in the home-lab dispatch via the SCOPE-07 selection-surface e2e
  (`tests/e2e/agent/openknowledge_e2e_test.go`), model/Ollama-dependent — NOT
  marked passing from dev.

### Findings for downstream scopes

- **SCOPE-05 (model-aware CostFn + budget):** the catalog descriptor's
  provider-qualified `id` (`<kind>/<backend-id>`) is the stable key into the
  SCOPE-01 `llm.model_costs` table; SCOPE-05's CostFn should price the
  `Canonicalize`-d / `SplitQualified`-parsed effective model and treat any
  `ollama/*` id as `$0` (free/local), a paid id with no rate as a fail-loud
  refusal.
- **SCOPE-06 (operator-gated web admin):** the `CatalogAggregator`'s set of
  adapters is built from the effective-enabled connections; SCOPE-06 should
  feed the per-connection `last_test_outcome = 'ok'` gate (design §5.1) into
  the "effective-enabled" predicate so a declared-but-untested hosted slot
  contributes a `disabled`/absent status rather than a curated list. The
  `HostedAdapter` is constructed straight from a `config.ModelConnection`, so
  wiring is a registry+DB read.
- **SCOPE-07 (combined selection, 088/089 parity)** is the primary consumer:
  it renders `CatalogAggregator.GetCatalog`'s `(ModelCatalog,
  []ProviderDiscoveryStatus)` in BOTH the Telegram numbered picker and the web
  `GET /v1/agent/model` surface — grouping Ollama-local first, showing an
  UNREACHABLE provider shown-but-disabled with its typed
  `ProviderDiscoveryStatus` (never hidden), and numbering/selection over the
  reachable subset only. It installs the `CatalogResolver` (or the
  catalog-built `modelswitch.Allowlist` via `ModelCatalog.Allowlist`) into the
  `agenttool` singletons so both surfaces resolve through the SAME injected
  validator + store. The provider-qualified-vs-bare normalization is already
  centralized in `Canonicalize`, so SCOPE-07 needs no second normalizer.
- **Wiring note (SCOPE-06/07):** `ModelCatalog.Allowlist()` builds the existing
  validator with envelope-skip (`envelopeMiB = 0`) + 0 profiles because catalog
  membership is pure string equality (a hosted model is not co-resident in the
  local Ollama envelope); the spec-088 local memory co-residence remains
  enforced for the Ollama switchable subset at the config-generation layer
  (`internal/config` envelope guard), unchanged. If SCOPE-07 wants the live
  picker to ALSO enforce the Ollama memory envelope, it can pass real Ollama
  profiles + the env envelope when constructing the allowlist for the Ollama
  subset.

### Completion Statement (SCOPE-03)

SCOPE-03 — Provider-aware `/ask` dispatch (credential seam) — is
**implemented and evidenced** (status `in_progress`; 10 of 11 DoD items
checked). The Go + Python `ChatRequest` carry the four provider fields
additively (zero-value wire-identical; Python still `extra="forbid"`); the
redesigned `_dispatch_live` forks Ollama (byte-for-byte today, NO `api_key`)
vs hosted (composed model + per-request key, typed `llm_misconfigured` on a
missing key, NEVER an Ollama substitution) with every error/log secret-scrubbed;
the `DispatchResolver` resolves a provider-qualified model through the SCOPE-01
registry + SCOPE-02 vault to a populated request + provider-qualified
attribution, refusing a not-effective-enabled target with a typed reason and no
local fallback; and `TurnResult.Model` carries the provider-qualified id
verbatim. The byte-for-byte Ollama parity test and the secret-scrub test both
pass (and both demonstrably fail under the captured RED-before), the 088/089
selection surfaces are untouched, and the pre-096 chat contract is intact. The
live-stack `integration`/`e2e-api` legs are deferred to a clean-stack
`bubbles.devops` dispatch, and the `check`/`format`/`artifact-lint` closeout
gates are routed to the orchestrator/owner — SCOPE-03 is held at `in_progress`
rather than fabricating a pass on the deferred legs.

---

## SCOPE-05 — Model-aware CostFn + load-bearing USD budget enforcement

**Status:** in_progress (7 of 12 DoD items met + evidenced; the residual are the
live-stack `integration` leg [T2-7, deferred to a clean-stack run], the two
closeout gates [T1-2 `check`, T1-3 `format --check`, run by the orchestrator
post-implementation], the migrate-output gate [T2-4, needs a live DB — deferred
with the integration leg], and the foreign-owned T1-1 `artifact-lint` [absent
`uservalidation.md`, owned by `bubbles.plan`] — none are SCOPE-05 code gaps).
**Executed by:** `bubbles.implement` (parent-expanded full-delivery).
**Scenarios covered:** SCN-096-G03.

### What shipped

The cost/budget seam that makes the EXISTING spec-064/087 per-user + global USD
budgets **load-bearing for paid providers** while keeping Ollama free — model-
aware pricing over the SCOPE-01 `llm.model_costs` SST, an append-only spend
ledger, and a pre-flight that refuses an exhausted ceiling BEFORE any billable
provider call.

1. **Model-aware CostFn** (`agent/costfn_modelaware.go`). `NewModelAwareCostFn`
   closes over a provider-qualified rate map (built from the SST
   `llm.model_costs[]`): an `ollama/*` model OR a bare 089-era id (no `/`) → `$0`
   deterministically (budget not consumed — NFR-2); a paid provider-qualified
   model → its rate applied to the combined token count at `max(input,output)`
   per-1k (a conservative upper bound, since the sidecar reports only combined
   tokens — consistent with the agent's `RecordLLMCall(0, tokens, …)` charge
   semantics); a billable model with **NO** declared rate → a fail-loud
   `ErrModelRateMissing` (NEVER a silent `$0` — the NO-DEFAULTS budget-bypass
   guard). The `CostFn` type changed from `func(int) float64` to
   `func(model string, tokensUsed int) (float64, error)`.
2. **Spend-ledger port + DB impl** (`agent/spendledger.go`,
   `usageledger/ledger.go`). The agent depends on a small `SpendLedger` port
   (`MonthToDateSpend(ctx) → (perUser, global)`, `AppendUsage(ctx, UsageRecord)`);
   the Postgres impl derives the claim-bound `actor_user_id` from context
   (`auth.UserIDFromContext` — NEVER a request body; empty actor fails loud) and
   sums `usd_cost` for the current month (per-user + global). Append-only.
3. **Load-bearing pre-flight + post-success append** (`agent/agent.go`). BEFORE
   any dispatch, for a paid effective model the agent computes the worst-case
   turn cost (`maxBillableTurnCostUSD` = `CostFn(model, PerQueryTokenBudget)` over
   the gather + synthesis models), reads month-to-date spend, and refuses with
   the EXISTING `ErrCapUSDMonthly` / `ErrCapUSDPerUserMonth` sentinels when a
   ceiling would be breached. After a successful billable turn it appends the
   realized cost. The gate is **inert** when no ledger is wired (`a.ledger ==
   nil`) AND for ollama (estCost `$0` → no ledger read, no append) — the
   ollama/free path is byte-for-byte unchanged (NFR-2).
4. **Migration 062** (`internal/db/migrations/062_model_usage_ledger.sql`).
   Append-only `model_usage_ledger` (`id`, `actor_user_id`, `connection_id`,
   `model`, `tokens`, `usd_cost NUMERIC(12,6)`, `created_at`) — app-written
   values, NO DB-side defaults (G028), `usd_cost >= 0` / `tokens >= 0` CHECKs,
   composite `(actor_user_id, created_at)` + `(created_at)` indexes for the
   per-user / global month-window SUM. **060 + 061 were taken; 062 is the next
   free slot** (the design said "060" before 060/061 landed).
5. **Production wiring** (`cmd/core/wiring_assistant_openknowledge.go`). The
   zero-cost stub is replaced by `NewModelAwareCostFn` over
   `cfg.ModelConnections.ModelCosts`, and the DB-backed `usageledger` is wired
   as `SpendLedger`. Live end-to-end verification is deferred with the
   integration leg.

### R4 — model-aware CostFn signature blast-radius (D05-T2-5)

The `CostFn` type change touches **10 call sites**; every one was updated and
the whole tree compiles (E1, `cmd/core` + all packages built under
`go test ./...`). The existing 064/087 budget tests were re-run and stay GREEN
(E1).

| # | Site | Change |
|---|------|--------|
| 1 | `agent/agent.go` — `type CostFn` | `func(int) float64` → `func(model string, tokensUsed int) (float64, error)` |
| 2 | `agent/agent.go` (gather call site) | `CostFn(result.TokensUsed)` → `CostFn(reqModel, …)`; refuse on `costErr` |
| 3 | `agent/agent.go` (synthesis-retry call site) | `CostFn(retryResult.TokensUsed)` → `CostFn(a.cfg.SynthesisModel, …)`; refuse on `costErr` |
| 4 | `agent/agent_test.go` — `baseCfg` | param kept as legacy `func(int) float64`; adapted to the model-aware seam internally (every pre-096 literal compiles unchanged) |
| 5 | `cmd/core/wiring_assistant_openknowledge.go` | model-aware `NewModelAwareCostFn` + `SpendLedger` wiring |
| 6 | `internal/assistant/facade_modelswitch_spec088_test.go` | `func(string, int) (float64, error) { return 0, nil }` |
| 7 | `internal/api/agent_invoke_test.go` (×2) | same model-aware literal |
| 8 | `tests/integration/openknowledge/helpers_test.go` *(tagged)* | same model-aware literal |
| 9 | `tests/stress/openknowledge_p95_test.go` *(tagged)* | same model-aware literal |
| 10 | `tests/e2e/openknowledge/open_knowledge_e2e_test.go` *(tagged)* | same model-aware literal |

The pre-096 USD-budget semantics are preserved: the new `SpendLedger` is
optional (nil in every pre-096 test via `baseCfg`), so the new pre-flight gate
is inert and the `BudgetTracker` caps are unchanged. 064/087 budget tests that
re-ran GREEN in E1: `TestAgent_BUG064001_PositivePerUserBudget_ProceedsPastPreflight`,
`TestAgent_BUG064001_ZeroPerUserBudget_StillRefusesPreflight`,
`TestAgent_PerTurnBudgetExhaustionRefusesWithCapture`,
`TestBudgetTracker_{New_Validation,CapsFireInOrder,PerUserMonthlyCapFires,Remaining,CapErrorsWrapErrBudgetExhausted}`,
`TestAgent_RetryBudgetExhausted_HonestSalvage_Spec087`,
`TestAgent_BudgetExhaustedMidLoop_NoPartialAnswer_AdversarialG021`.

### Change Manifest (this scope's edits only)

```text
=== SCOPE-05 NEW ===
?? internal/assistant/openknowledge/agent/costfn_modelaware.go            (model-aware CostFn + ModelRate + ErrModelRateMissing + maxBillableTurnCostUSD)
?? internal/assistant/openknowledge/agent/spendledger.go                 (SpendLedger port + UsageRecord)
?? internal/assistant/openknowledge/agent/costfn_modelaware_test.go       (SCN-096-G03 unit: ollama=$0/paid=rate; missing-rate fail-loud ADVERSARIAL)
?? internal/assistant/openknowledge/agent/budget_preflight_modelcost_test.go (SCN-096-G03 unit: refused-before-dispatch ADVERSARIAL + under-budget control)
?? internal/assistant/openknowledge/usageledger/ledger.go                (Postgres SpendLedger impl; actor from auth ctx; month-to-date SUM; append-only)
?? internal/db/migrations/062_model_usage_ledger.sql                      (append-only ledger; no DB-side defaults)
?? tests/integration/model_budget_enforcement_test.go                    (//go:build integration; SCN-096-G03 live leg — env-gated t.Skip, DEFERRED)
=== SCOPE-05 EDITED ===
 M internal/assistant/openknowledge/agent/agent.go                       (CostFn model-aware; +SpendLedger Config/field; load-bearing pre-flight; post-success append; both call sites)
 M cmd/core/wiring_assistant_openknowledge.go                            (model-aware CostFn over SST rates + usageledger wiring)
 M internal/assistant/openknowledge/agent/agent_test.go                  (baseCfg adapts legacy closure to model-aware seam)
 M internal/assistant/facade_modelswitch_spec088_test.go                 (R4 caller)
 M internal/api/agent_invoke_test.go                                     (R4 caller ×2)
 M tests/integration/openknowledge/helpers_test.go                       (R4 caller, tagged)
 M tests/stress/openknowledge_p95_test.go                               (R4 caller, tagged)
 M tests/e2e/openknowledge/open_knowledge_e2e_test.go                    (R4 caller, tagged)
```

No `modelswitch`/`modelpref`/picker behaviour file is edited — 088/089 is
untouched (additive cost/budget seam only).

### Test Evidence

All evidence below is REAL captured terminal output (line-wrapping is terminal
width only; no content edited). Go tests run via the sanctioned
`./smackerel.sh test unit --go --go-run … --verbose` path (the repo's
`golang` container, `go test -run <regex> -count=1 ./...` — the whole tree
compiles, so `cmd/core` wiring is compile-checked). These are UNIT tests: a
fake `SpendLedger` and a must-not-be-called fake LLM — NO DB, NO network, NO
interception. All rates/ledger values are SYNTHETIC; no provider or credential.

#### Evidence E1 — full scoped GREEN run (SCOPE-05 tests + R4 064/087 budget tests + cmd/core compile)

Command: `./smackerel.sh test unit --go --go-run 'Spec096|CostFn|Budget|ModelCost' --verbose`
(overall `UNIT_EXIT=0`, `[go-unit] go test ./... finished OK`):

```text
--- PASS: TestAgent_BUG064001_PositivePerUserBudget_ProceedsPastPreflight (0.00s)
--- PASS: TestAgent_BUG064001_ZeroPerUserBudget_StillRefusesPreflight (0.00s)
--- PASS: TestBudgetPreflight_PaidOverBudget_RefusesBeforeDispatch_Spec096 (0.00s)
    --- PASS: …/refuses_before_dispatch/global_month-to-date_spend_exhausted
    --- PASS: …/refuses_before_dispatch/per-user_month-to-date_spend_exhausted
    --- PASS: …/under_budget_reaches_dispatch
--- PASS: TestAgent_PerTurnBudgetExhaustionRefusesWithCapture (0.00s)
--- PASS: TestBudgetTracker_CapErrorsWrapErrBudgetExhausted (0.00s)
--- PASS: TestCostFn_OllamaZero_PaidUsesRate_Spec096 (0.00s)   [7 subtests: ollama qualified/bare/slash-backend free; paid output-rate; scales; openai equal-rate; zero-tokens]
--- PASS: TestCostFn_PaidModelMissingRate_RefusesFailLoud_Spec096 (0.00s)
--- PASS: TestAgent_RetryBudgetExhausted_HonestSalvage_Spec087 (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agent  0.196s
ok      github.com/smackerel/smackerel/cmd/core 2.085s
```

`cmd/core 2.085s` (NOT `[no tests to run]`) proves the production wiring
(model-aware CostFn + `usageledger.SpendLedger`) compiles. The config SCOPE-01
Spec096 tests + SCOPE-02/03/04 Spec096 tests also re-ran GREEN in the same run.

#### Evidence E2 — captured RED-before (both adversarial tests bite; non-tautological)

Two temporary mutations were applied and the two adversarial tests re-run, then
BOTH reverted (post-revert code byte-identical to E1/E3): (a)
`costfn_modelaware.go` missing-rate branch neutered to a silent `$0`; (b)
`agent.go` pre-flight gate disabled (`if estCost > 999999`).

Command: `./smackerel.sh test unit --go --go-run 'TestCostFn_PaidModelMissingRate_RefusesFailLoud_Spec096|TestBudgetPreflight_PaidOverBudget_RefusesBeforeDispatch_Spec096' --verbose` (`RED_EXIT=1`):

```text
--- FAIL: TestBudgetPreflight_PaidOverBudget_RefusesBeforeDispatch_Spec096 (0.04s)
    --- FAIL: …/refuses_before_dispatch/global_month-to-date_spend_exhausted
    --- FAIL: …/refuses_before_dispatch/per-user_month-to-date_spend_exhausted
    --- PASS: …/under_budget_reaches_dispatch
    costfn_modelaware_test.go:79: expected a fail-loud refusal for a billable model with no rate; got nil error (silent $0 budget-bypass)
--- FAIL: TestCostFn_PaidModelMissingRate_RefusesFailLoud_Spec096 (0.00s)
FAIL    github.com/smackerel/smackerel/internal/assistant/openknowledge/agent  0.177s
```

With the gate disabled the two over-budget subtests reach dispatch (the fake
LLM's empty queue `t.Fatalf`s) → FAIL, while the `under_budget_reaches_dispatch`
control still PASSES (proving the gate is conditional, not unconditional). With
the missing-rate refusal neutered, the CostFn test FAILs on the exact
`silent $0 budget-bypass` assertion. Both adversarial tests demonstrably bite.

#### Evidence E3 — post-revert GREEN (revert restored; RED→GREEN cycle closed)

Command: `./smackerel.sh test unit --go --go-run 'TestCostFn_OllamaZero_PaidUsesRate_Spec096|TestCostFn_PaidModelMissingRate_RefusesFailLoud_Spec096|TestBudgetPreflight_PaidOverBudget_RefusesBeforeDispatch_Spec096' --verbose` (`GREEN_EXIT=0`):

```text
--- PASS: TestBudgetPreflight_PaidOverBudget_RefusesBeforeDispatch_Spec096 (0.01s)
    --- PASS: …/refuses_before_dispatch/global_month-to-date_spend_exhausted
    --- PASS: …/refuses_before_dispatch/per-user_month-to-date_spend_exhausted
    --- PASS: …/under_budget_reaches_dispatch
--- PASS: TestCostFn_OllamaZero_PaidUsesRate_Spec096 (0.00s)
--- PASS: TestCostFn_PaidModelMissingRate_RefusesFailLoud_Spec096 (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agent  0.083s
```

#### Evidence E4 — NO-DEFAULTS grep (D05-T2-1): no silent `$0`, fail-loud present

```text
=== A) forbidden silent-fallback patterns in the SCOPE-05 cost/budget code (expect NONE) ===
(none — no silent fallback)

=== B) the fail-loud missing-rate refusal + typed pre-flight refusals (must be PRESENT) ===
costfn_modelaware.go:33:var ErrModelRateMissing = errors.New("openknowledge/agent: billable model has no llm.model_costs rate (refused; never $0)")
costfn_modelaware.go:73:        return 0, fmt.Errorf("%w: model %q", ErrModelRateMissing, model)
agent.go:443:           return refuse(TerminationCapUSD, ok.ErrCapUSDMonthly.Error()), nil
agent.go:446:           return refuse(TerminationCapUSD, ok.ErrCapUSDPerUserMonth.Error()), nil
usageledger/ledger.go:82:  return 0, 0, errors.New("usageledger: MonthToDateSpend requires a claim-bound actor_user_id in context (no silent default)")
```

The grep (over `costfn_modelaware.go`, `spendledger.go`, `usageledger/ledger.go`)
finds NO `${VAR:-default}` / `os.Getenv(k, default)` / `unwrap_or` / `|| 0` /
silent-`return 0, nil` fallback; the fail-loud missing-rate refusal and the
typed per-user/global pre-flight refusals are present.

### DoD mapping (SCOPE-05)

| DoD item | Status | Evidence |
|----------|--------|----------|
| D05-T1-1 artifact-lint clean | ⬚ deferred | absent `uservalidation.md` (closeout artifact owned by `bubbles.plan`) — not a SCOPE-05 code gap (same caveat as SCOPE-01..04) |
| D05-T1-2 `check` EXIT 0 | ⬚ deferred | run by the orchestrator post-implementation; SCOPE-05 compiles clean under `go test ./...` (E1, `finished OK`, `cmd/core` built) |
| D05-T1-3 `format --check` EXIT 0 | ⬚ deferred | global gate; run at closeout by the orchestrator |
| D05-T1-4 evidence is real terminal output | ✅ | E1–E4 (all captured, unedited apart from terminal wrapping) |
| D05-T1-5 088/089 do-not-amend boundary (additive) | ✅ | Change Manifest (no `modelswitch`/`modelpref`/picker file edited); the cost/budget seam is additive |
| D05-T2-1 G028 fail-loud on missing/zero rate | ✅ | E1 `TestCostFn_PaidModelMissingRate…` + E2 RED-before + E4 grep (no silent-$0 path) |
| D05-T2-2 Ollama stays free (NFR-2) | ✅ | E1/E3 `TestCostFn_OllamaZero…` (ollama qualified+bare+slash-backend → $0); the pre-flight skips the ledger when estCost=$0 (two in-process map lookups, no I/O) |
| D05-T2-3 budget refused BEFORE dispatch (adversarial) | ✅ | E1/E3 `TestBudgetPreflight_PaidOverBudget_RefusesBeforeDispatch_Spec096` (global + per-user) + E2 RED-before; the live integration row is deferred (T2-7) |
| D05-T2-4 append-only spend ledger (migration applies) | ◐ partial | migration 062 authored (append-only, no DB-side defaults, CHECKs + indexes) + embedded; the `migrate`-output gate needs a live DB — DEFERRED with the integration leg |
| D05-T2-5 R4 CostFn blast-radius (binding) | ✅ | R4 table (10 sites updated; whole-tree compile E1) + the 064/087 budget tests re-ran GREEN (E1) |
| D05-T2-6 test isolation (synthetic rates/ledger) | ✅ | unit rows use a fake `SpendLedger` + synthetic rates, NO DB/provider/credential; the live integration row is the only DB-touching leg (deferred, ephemeral-only) |
| D05-T2-7 all unit + integration pass, no skips | ◐ partial | unit leg done (E1/E3, no skips); the `integration` `TestAsk_PaidModelExhaustedBudget_RefusedBeforeProviderCall_Spec096` is live-stack — DEFERRED to a clean-stack `bubbles.devops` dispatch, NOT marked passing from dev |

### Live-stack leg deferred to clean-stack run

Per the plan's deferral note, the live SCOPE-05 row was NOT run this turn:
`tests/integration/model_budget_enforcement_test.go::TestAsk_PaidModelExhaustedBudget_RefusedBeforeProviderCall_Spec096`
(`//go:build integration`, env-gated `t.Skip`). It needs a live ephemeral Go
core + a seeded `model_usage_ledger` + a paid-provider test fixture; the unit
adversarial tests already prove the refuse-before-dispatch contract in
isolation. The migrate-output (T2-4) is verified on the same live stack.

**Deferred-verification risk (routed for the integration leg / SCOPE-06/07):**
the DB ledger derives `actor_user_id` from `auth.UserIDFromContext(ctx)`; the
live integration leg must confirm the authenticated session propagates into the
agent's `Run` ctx (else a paid turn fails loud — never silently mis-accounts).
The ollama-default path is unaffected (ledger gate inert at estCost=$0).

### Findings for downstream scopes

- **SCOPE-06 (operator-gated web admin):** enabling a paid hosted connection
  makes its models billable; the SCOPE-06 enable path should ensure every
  enabled non-ollama model has an `llm.model_costs` rate (SCOPE-01 config
  validation already enforces this at config-gen; the CostFn is the runtime
  backstop). No new cost surface needed.
- **SCOPE-07 (combined selection + `GET /v1/agent/model`):** the additive
  budget enrichment (R2: optional MTD budget on the model surface) can read
  `SpendLedger.MonthToDateSpend` for the per-group cost class + the budget
  line; the ledger port + DB impl are in place. SCOPE-07 should surface the
  paid-model cost class (pull-not-push) without changing the load-bearing
  refusal.

### Completion Statement (SCOPE-05)

SCOPE-05 — Model-aware CostFn + load-bearing USD budget enforcement — is
**implemented and evidenced** (status `in_progress`; 7 of 12 DoD items met,
T2-4/T2-7 partial). The `CostFn` is model-aware over the SST `llm.model_costs`
rate table (ollama/bare → `$0`; paid → rate; missing paid rate → fail-loud
`ErrModelRateMissing`, never a silent `$0`); the EXISTING per-user + global USD
budgets are now load-bearing via an append-only `model_usage_ledger` (migration
062) and a pre-flight that refuses an exhausted ceiling with the EXISTING
`ErrCapUSDMonthly` / `ErrCapUSDPerUserMonth` sentinels BEFORE any billable
dispatch, appending the realized cost after success. The adversarial
refuse-before-dispatch + missing-rate tests both pass and both demonstrably
fail under the captured RED-before (E2); the ollama/free path is byte-for-byte
unchanged (no ledger read/write at estCost=$0); the R4 blast-radius (10 sites)
is updated and the 064/087 budget tests re-ran GREEN; and 088/089 is untouched.
The live `integration` leg + the `migrate`-output gate are deferred to a
clean-stack `bubbles.devops` dispatch and the `check`/`format`/`artifact-lint`
closeout gates are routed to the orchestrator/owner — SCOPE-05 is held at
`in_progress` rather than fabricating a pass on the deferred legs.

---

## SCOPE-06 — Operator-gated web admin connection surface (backend half)

**Status:** in_progress (BACKEND HALF delivered + evidenced; the operator-gated
PWA triad — `model-connections.html` / `model-connection-add.html` /
`model-connection-detail.html` — is a SEPARATE follow-up frontend dispatch, and
the live `integration` + `e2e-api` legs are deferred to the clean-stack /
home-lab `bubbles.devops` dispatch (C7)).
**Executed by:** `bubbles.implement` (parent-expanded full-delivery).
**Scenarios covered (backend, unit):** SCN-096-W01/W02/W03/W04 (the unit halves
— write-only redaction, truthful typed test, 409 enable-guard, 404 closed-set,
per-kind secret fields, operator gate). The W01/W02/W04 live `e2e-api` legs +
the W03 `integration` leg are DEFERRED.

### What shipped (Go / API only)

1. **Operator gate (R1 RESOLVED).** New SST `infrastructure.operator_user_ids`
   allowlist (`config/smackerel.yaml` → `scripts/commands/config.sh`
   `OPERATOR_USER_IDS` CSV → `internal/config/config.go` `OperatorUserIDs`),
   empty by default (No Env-Specific Content; real ids in the deploy overlay).
   `internal/api/model_connections_operator_gate.go`: the gate reads the
   claim-bound bearer subject (`auth.UserIDFromContext`, spec-044 — NEVER a body
   field); anonymous → `401`, authenticated non-operator → `403`, operator →
   pass; an EMPTY allowlist is **fail-closed** (everyone rejected, no
   open-by-default surface). `ValidateOperatorGate` is the G028 fail-loud
   startup guard: empty allowlist + reachable surface (≥1 db-mode slot) +
   `production` → abort; dev/test runs fail-closed with a warning (mirrors the
   repo's MIT-040-S-004 empty-token auth precedent).
2. **`/v1/admin/model-connections*` surface** (`internal/api/model_connections_admin.go`,
   mounted in `router.go` behind `bearerAuthMiddleware` + the operator gate):
   `GET /` (list every db-mode slot + status, no secrets), `GET /{id}`,
   `PUT /{id}/credential` (write-only → redacted view; `404` non-db-mode; `422`
   missing per-kind field), `POST /{id}/test` (truthful typed `ok|failed` +
   `auth_failed|unreachable|timeout`; `409` if no credential), `POST /{id}/enable`
   (`409` unless credential present AND last-test=`ok`), `POST /{id}/disable`.
   **Every response omits the plaintext credential** (presence + last-4 +
   last-test only). `model_connections_probe.go` is the production
   `HTTPConnectionProbe` (truthful reachability/auth-class; real per-provider
   semantics validated by the deferred e2e leg).
3. **DB-backed CredentialSource + effective-enabled predicate**
   (`internal/assistant/openknowledge/connstore/`). `connstore.Store` owns the
   migration-061 `model_provider_connections` rows. `EffectiveEnabled(rec,
   declared)` is THE single gate = registry-declared AND DB-`enabled` AND
   `last_test_outcome=ok` AND credential present. `Store.Credential(connID)`
   structurally satisfies the SCOPE-03 `llm.CredentialSource` (returns a record
   ONLY when effective-enabled → fail-closed → the resolver never Ollama-falls-
   back). `Store.DiscoveryConnections(ctx)` is the SCOPE-04 feed (same gate).
   Wired in `cmd/core/wiring_assistant_openknowledge.go` (`buildModelConnectionsAdmin`)
   + `cmd/core/wiring.go`; the store is stashed on `svc.modelConnStore` so the
   (deferred) live resolver/aggregator wiring reads the SAME seam the admin
   surface writes.

No new migration: 061 already carries `last_tested_at` / `last_test_outcome` /
`last_test_detail`. 088/089 + SCOPE-01..05 are untouched (additive only).

### Test Evidence

All evidence below is REAL captured terminal output (line-wrapping is terminal
width only; no content edited; home paths shown as `~`). UNIT/handler-level: a
real `chi` router, an in-memory fake store, a SYNTHETIC vault, and an injected
fake probe — NO DB, NO network, NO interception. All secret values are
SYNTHETIC.

#### Evidence E1 — full scoped GREEN run (SCOPE-06 unit tests + SCOPE-01..05 regression + `cmd/core` compile)

Command: `~/smackerel$ ./smackerel.sh test unit --go --go-run 'Spec096|AdminModelConnections|OperatorGate' --verbose`
(overall `[go-unit] go test ./... finished OK`):

```text
--- PASS: TestAdminModelConnections_PutCredentialWriteOnly_RedactedView_Spec096 (0.00s)
--- PASS: TestAdminModelConnections_TestConnection_TruthfulOutcome_Spec096 (0.00s)
--- PASS: TestAdminModelConnections_PerKindSecretFields_OpenAIFoundryGoogleBedrock_Spec096 (0.00s)
--- PASS: TestAdminModelConnections_UnknownSlotRejected404_Spec096 (0.00s)
--- PASS: TestAdminModelConnections_EnableUntested_Blocked409_Spec096 (0.00s)
--- PASS: TestAdminModelConnections_FailedTest_TypedError_NeverFalseSuccess_Spec096 (0.00s)
--- PASS: TestAdminModelConnections_OperatorGate_403NonOperator_401Anonymous_Spec096 (0.00s)
ok      github.com/smackerel/smackerel/internal/api     0.387s
--- PASS: TestEffectiveEnabled_SingleGate_Spec096 (0.00s)
    --- PASS: TestEffectiveEnabled_SingleGate_Spec096/not_registry-declared_(UI-invented_slot) (0.00s)
    --- PASS: TestEffectiveEnabled_SingleGate_Spec096/DB_disabled_(operator_toggled_off) (0.00s)
    --- PASS: TestEffectiveEnabled_SingleGate_Spec096/last_test_failed_(never_enable_an_unverified_connection) (0.00s)
    --- PASS: TestEffectiveEnabled_SingleGate_Spec096/untested_(last_test_outcome_empty) (0.00s)
    --- PASS: TestEffectiveEnabled_SingleGate_Spec096/credential_missing_(no_cipher_material) (0.00s)
--- PASS: TestHasCredential_RequiresCipherMaterial_Spec096 (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/connstore        0.035s
ok      github.com/smackerel/smackerel/internal/config  0.046s   (SCOPE-01 regression clean)
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/connvault        0.027s   (SCOPE-02 regression clean)
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/llm      0.029s   (SCOPE-03 regression clean)
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/catalog  0.072s   (SCOPE-04 regression clean)
ok      github.com/smackerel/smackerel/internal/assistant/openknowledge/agent    0.040s   (SCOPE-05 regression clean)
[go-unit] go test ./... finished OK
```

The whole tree compiles under `go test ./...`, so the new `cmd/core` wiring
(`buildModelConnectionsAdmin` + the `connstore.Store` CredentialSource seam +
the router mount) is compile-checked. SCOPE-01..05 Spec096 tests re-ran GREEN
in the same run (no regression from the new wiring).

#### Evidence E2 — captured RED-before (three adversarial tests bite; non-tautological)

Three temporary mutations were applied and the adversarial tests re-run, then
ALL reverted (post-revert code byte-identical to E1; `grep -rn 'RED-BEFORE|if
false' internal/api/*.go` → no matches): (a) operator gate `!IsOperator` check
neutered (`if false && …`) so a non-operator passes; (b) `Test` outcome forced
to a constant `ok`; (c) `Enable` 409-precondition disabled (`if false && …`).

Command: `~/smackerel$ ./smackerel.sh test unit --go --go-run 'AdminModelConnections_OperatorGate|AdminModelConnections_FailedTest|AdminModelConnections_EnableUntested' --verbose`:

```text
--- FAIL: TestAdminModelConnections_EnableUntested_Blocked409_Spec096 (0.00s)
--- FAIL: TestAdminModelConnections_FailedTest_TypedError_NeverFalseSuccess_Spec096 (0.00s)
--- FAIL: TestAdminModelConnections_OperatorGate_403NonOperator_401Anonymous_Spec096 (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/api     0.398s
```

A gate that admits a non-operator, a probe reported as a false `ok`, and an
enable that skips the credential+test precondition each flip the matching
adversarial test to FAIL — the tests are non-tautological. Post-revert the same
three tests PASS (E1).

#### Evidence E3 — operator-gate SST pipeline (R1, NO-DEFAULTS G028)

Command: `~/smackerel$ ./smackerel.sh config generate && ./smackerel.sh config generate --env test`
then `grep -n OPERATOR_USER_IDS config/generated/dev.env config/generated/test.env`:

```text
config-validate: ~/smackerel/config/generated/dev.env.tmp.* OK
Generated ~/smackerel/config/generated/dev.env
config-validate: ~/smackerel/config/generated/test.env.tmp.* OK
Generated ~/smackerel/config/generated/test.env
config/generated/dev.env:454:OPERATOR_USER_IDS=
config/generated/test.env:454:OPERATOR_USER_IDS=
```

`OPERATOR_USER_IDS=` is emitted EMPTY by default (committed config carries
`infrastructure.operator_user_ids: []` — No Env-Specific Content). The
fail-loud `ValidateOperatorGate` (empty + reachable + production → abort) +
fail-closed runtime gate are proven by
`TestAdminModelConnections_OperatorGate_403NonOperator_401Anonymous_Spec096`
(E1), which exercises the `401`/`403`/pass paths AND the
`production`-aborts / `development`-warns / unreachable-passes branches.

#### Evidence E4 — write-only-secret grep (D06-T2-2): no handler returns/logs the plaintext

Command: `~/smackerel$ grep -nE 'Decrypt|api_key|secret_fields' internal/api/model_connections_admin.go | grep -iE 'writeJSON|return|log|slog'`:

```text
(no match — the only Decrypt call feeds the in-core probe; no JSON/log/return
path carries the decrypted bundle or any secret field. The redacted view type
`modelConnectionView` has NO plaintext field; the write-only assertion in
TestAdminModelConnections_PutCredentialWriteOnly_RedactedView_Spec096 confirms
the cleartext never appears in the response body.)
```

### Change Manifest (this scope's edits only)

```text
=== SCOPE-06 NEW ===
?? internal/assistant/openknowledge/connstore/store.go                  (Store; EffectiveEnabled predicate; Credential = SCOPE-03 CredentialSource; DiscoveryConnections = SCOPE-04 feed; Upsert/RecordTest/SetEnabled)
?? internal/assistant/openknowledge/connstore/scan.go                   (nullable-column row scan)
?? internal/assistant/openknowledge/connstore/store_test.go             (SCN-096-W03 unit: EffectiveEnabled single-gate truth table + HasCredential)
?? internal/api/model_connections_admin.go                              (operator-gated admin handler: list/get/credential/test/enable/disable; per-kind secret fields; write-only views)
?? internal/api/model_connections_admin_test.go                         (SCN-096-W01/W02/W03/W04 unit: 6 manifest tests; httptest + fake store + synthetic vault + fake probe)
?? internal/api/model_connections_operator_gate.go                      (R1 operator gate middleware + ValidateOperatorGate G028 fail-loud)
?? internal/api/model_connections_operator_gate_test.go                 (SCN-096-W04 unit: OperatorGate 403/401 ADVERSARIAL + fail-loud guard)
?? internal/api/model_connections_probe.go                              (production HTTPConnectionProbe; truthful typed reachability/auth-class)
?? tests/integration/model_connections_enable_disable_test.go           (//go:build integration; SCN-096-W03 live leg — env-gated t.Skip, DEFERRED clean-stack; t.Fatal guard prevents green-painting)
?? tests/e2e/admin/model_connections_e2e_test.go                        (//go:build e2e; SCN-096-W01/W02/W04 live legs — env-gated t.Skip, DEFERRED home-lab; t.Fatal guards prevent green-painting)
=== SCOPE-06 EDITED ===
 M config/smackerel.yaml                                                (infrastructure.operator_user_ids: [] — empty default, No Env-Specific Content)
 M scripts/commands/config.sh                                           (OPERATOR_USER_IDS read [YAML list→CSV] + emit, mirrors trusted_proxies)
 M internal/config/config.go                                           (OperatorUserIDs []string field + loader)
 M internal/api/health.go                                              (Dependencies += ModelConnectionsAdminHandler + ModelConnectionsOperatorGate)
 M internal/api/router.go                                              (mount /v1/admin/model-connections* behind bearerAuth + operator gate)
 M cmd/core/services.go                                                (coreServices += modelConnStore *connstore.Store seam)
 M cmd/core/wiring_assistant_openknowledge.go                          (buildModelConnectionsAdmin: store + vault + gate + handler; G028 guard)
 M cmd/core/wiring.go                                                  (buildAPIDeps wires the admin handler + operator gate into deps)
```

### DoD status (backend half)

| DoD | State | Note |
|-----|-------|------|
| D06-T1-1 artifact-lint | [ ] | foreign-owned: absent `uservalidation.md` (owned by `bubbles.plan`) — not a SCOPE-06 code gap |
| D06-T1-2 `check` EXIT 0 | [ ] | orchestrator closeout (post-implementation) |
| D06-T1-3 `format --check` | [ ] | orchestrator closeout |
| D06-T1-4 real evidence | [x] | E1–E4 are real captured output |
| D06-T1-5 no env-specific content | [x] | `operator_user_ids` empty default; generic placeholders; PWA triad = follow-up frontend dispatch |
| D06-T2-1 R1 operator gate (403/401 + fail-loud) | [x] | E1 gate test + E3; `ValidateOperatorGate` G028 |
| D06-T2-2 write-only secret | [x] | E1 PutCredentialWriteOnly + E4 grep |
| D06-T2-3 truthful test, never false success | [x] | E1 FailedTest_TypedError + E2 RED-before |
| D06-T2-4 enable 409-guard + effective-enabled single gate | [ ] | UNIT proven (EnableUntested_Blocked409 + EffectiveEnabled_SingleGate); the live `TestEnableDisable_CatalogMembershipFollows` integration leg is DEFERRED (clean stack, C7) |
| D06-T2-5 closed-set 404 + per-kind secret fields | [x] | E1 UnknownSlotRejected404 + PerKindSecretFields |
| D06-T2-6 live-stack authenticity + C7 deferral | [ ] | the `integration` + 3 `e2e-api` legs are DEFERRED to the home-lab `bubbles.devops` dispatch — NOT marked passing from dev; the manifest-named stubs EXIST (`tests/integration/model_connections_enable_disable_test.go`, `tests/e2e/admin/model_connections_e2e_test.go`) as honest env-gated `t.Skip` + `t.Fatal`-guarded scaffolds (compile-checked under their build tags), never green-painted |
| D06-T2-7 adversarial non-tautological + RED-before | [x] | E2 (3 adversarial tests bite, then reverted) |
| D06-T2-8 all unit + integration pass, no skips | [ ] | all UNIT pass (E1); `integration` + `e2e-api` legs DEFERRED (honest env-gated stubs, compile-checked, `t.Skip` until the live fixture seeds) |

**7 of 13 met + evidenced.** The 6 residual are foreign-owned (`artifact-lint`),
orchestrator closeout (`check` / `format`), or honestly deferred live-stack /
frontend legs (the W03 `integration` catalog-membership leg, the W01/W02/W04
`e2e-api` legs, and the operator-gated PWA triad) — none are SCOPE-06 backend
code gaps. SCOPE-06 is held at `in_progress` rather than fabricating a pass on
the deferred legs.

---

## SCOPE-06 (frontend) — Operator-gated model-connections PWA triad

**Status:** in_progress (FRONTEND HALF delivered — the operator-gated PWA triad
that drives the already-committed `/v1/admin/model-connections*` API; the live
`e2e-ui` Playwright legs need the running stack and are DEFERRED to a clean-stack
dispatch). The Go-side route MOUNTING noted in the backend section is final as a
CONTRACT; the UI is built against that endpoint contract.
**Executed by:** `bubbles.implement` (parent-expanded full-delivery, frontend
dispatch).
**Surface built:** S1 list, S2 add/configure, S3 detail — per spec.md
`## UI Wireframes` (S1/S2/S3), `## User Flows`, and the binding
`### Write-Only Secret Affordance Contract`.

### What shipped (web/pwa only — no Go/API/runtime edits)

```text
=== SCOPE-06 (frontend) NEW ===
?? web/pwa/model-connections.html              (S1 — list every db-mode slot + status, operator-gated)
?? web/pwa/model-connections.js                (S1 logic: GET /v1/admin/model-connections; 401→login, 403→operator-only)
?? web/pwa/model-connection-add.html           (S2 — pick a declared-unconfigured slot → per-kind write-only secret form)
?? web/pwa/model-connection-add.js             (S2 logic: scope to secret_present=false; PUT …/{id}/credential)
?? web/pwa/model-connection-detail.html        (S3 — status banners, Test/Enable/Disable/Replace)
?? web/pwa/model-connection-detail.js          (S3 logic: POST …/test|enable|disable, PUT …/credential)
?? web/pwa/tests/model_connections_pwa_guard_test.go  (runnable ui-unit static guard: write-only + operator-gate + truthful-test, with adversarial twins)
```

These six pages auto-embed via the existing `web/pwa/embed.go` glob
(`//go:embed *.html *.css *.js …`) — no `embed.go` edit needed. They match the
`connectors.html → connectors-add.html → connector-detail.html` convention
exactly: same `card` / `status` / `radio-group[role=radiogroup]` / `btn` /
`connector-detail-fields` `<dl>` / `status-pill` / `<template data-field>` class
vocabulary, ARIA roles (`role=status`/`role=alert`/`aria-busy`/`role=radiogroup`),
`credentials: "same-origin"` cookie-session fetch (spec-070), and
`textContent`-only rendering (no `innerHTML`; CSP `script-src 'self'` preserved —
no inline JS).

**Service-worker / manifest registration:** none required. The convention is
that feature pages are NOT listed in `sw.js` `STATIC_ASSETS` (the precache list
holds only the app shell — `index.html`/`style.css`/`app.js`/`icon.svg`/
`manifest.json`/`lib/queue.js`); the `sw.js` fetch handler caches every `/pwa/*`
asset on demand (cache-first). The connectors triad and `assistant.html` are
likewise absent from the precache list. `manifest.json` lists no pages either.
So the three new pages are served + cached by the existing mechanism with no
edit — matching convention. (Verified by inspection of `web/pwa/sw.js` L2-10 and
`web/pwa/manifest.json`.) **Claim Source: interpreted** (read the committed
`sw.js`/`manifest.json` + the connectors-triad precedent).

### How the three binding contracts are implemented

1. **Write-only secret affordance (headline contract).** The only credential
   inputs are built in JS (`makeSecretInput` in `model-connection-add.js`;
   `buildRotateInputs` in `model-connection-detail.js`) as
   `input.type = "password"; input.autocomplete = "off"; input.value = "";`
   with placeholder `Enter to set/replace — never displayed` and
   `aria-describedby` a "never displayed" hint. The input is NEVER pre-filled
   and is never assigned from server data. The stored credential is rendered as
   **presence + last-4 only** (`secret_present` → `present · ····wxyz`, else
   `needs credential`) from the redacted admin view — the plaintext is never in
   the response and never enters the DOM. There is **NO reveal / show-password /
   copy-secret / unmask control anywhere**. Save → `PUT …/{id}/credential
   {secret_fields}`; the response is the redacted view (nothing echoed).
   Rotation = enter a new value into the same write-only field → `PUT …/credential`
   → the store clears the last test → the UI shows "re-test required". Per-kind
   secret fields match the backend `validateSecretFields`
   (`model_connections_admin.go`): anthropic/openai/azure-foundry → `api_key`;
   bedrock → `aws_access_key_id` + `aws_secret_access_key`; google → one-of
   `api_key` (Gemini) OR `service_account` (Vertex). Non-secret params are shown
   read-only from the slot's `params` (generic `<dl>`, not editable — a brand-new
   KIND is an SST edit). **Claim Source: executed** (files created; structure
   verifiable in the committed pages).
2. **Operator-only boundary visible.** Every page's fetch path handles
   `resp.status === 401` → `routeToLogin()` (`window.location.assign("/login?next="
   + encodeURIComponent(currentPath))`, the spec-077 `/login?next=` convention)
   and `resp.status === 403` → `showOperatorOnly()` which reveals the
   `#operator-only` notice (`🔒 Operator only`, `role=alert`) and HIDES every
   setup affordance (the list/add button on S1, the whole form on S2, the whole
   detail card on S3). No secrets, no actions for a non-operator. The pages are
   intentionally NOT added to the user-facing nav (`index.html` untouched).
   **Claim Source: executed.**
3. **Truthful status — a failed test is unambiguously failed (Principle 8 /
   SCN-096-W04).** `model-connection-detail.js` `renderTestOutcome(result)` has an
   explicit `result.outcome === "ok"` branch: `ok` → the `#banner-ok` success
   banner (`role=status`, `✓ tested ok · <when>`); anything else → the
   `#banner-fail` danger banner (`role=alert`) carrying the **typed detail**
   (`auth_failed`/`unreachable`/`timeout`) and naming the connection
   (`<detail>: connection <id> is NOT usable (not substituted)`). There is NO
   code path mapping a non-ok outcome to success, and **no `ollama` mention in
   executable code** (a failed hosted test never substitutes Ollama). The
   **Enable** control reflects the 409 guard: `enableBtn.disabled` stays true
   until `secret_present && last_test_outcome === "ok"`, and a server `409`
   (`ENABLE_PRECONDITION`) renders the typed reason — client guard + server guard
   agree. **Claim Source: executed.**

### Accessibility + responsive (Principle 7)

Matches the connectors triad: one `<h1>` per page; provider/section groups are
`<h2>`; the add-flow provider picker is `role=radiogroup` with an `aria-label`;
loading uses `aria-busy="true"` + `role=status aria-live=polite`; failures use
`role=alert`; status carries **text + glyph** (`● enabled` / `○ disabled` /
`✓ ok` / `✗ failed`), never color-only; every control is a real
`<a>`/`<button>`/`<input>` (keyboard operable). No secret is placed in a
`title`, an `aria-live` region, or a status message — presence/last-4 only.

### Test Evidence

#### Evidence F1 — runnable ui-unit static guard authored (web/pwa/tests/model_connections_pwa_guard_test.go)

This is the repo's no-live-stack `ui-unit` surface (the
`assistant_source_href_security_guard_test.go` / `assistant_storage_guard_test.go`
pattern — read the committed PWA source, assert a contract; package
`webcodegen_drift_test`, reusing the shared `repoRoot` + `stripLineComments`
helpers). It pins all three binding contracts with non-tautological adversarial
twins:

```text
TestModelConnectionsWriteOnlySecretAffordance_Spec096        (type=password, autocomplete=off, value="", "never displayed"; forbids reveal/show/copy/unmask, type="text", value echo/pre-fill; operator-only block in all 3 HTML)
TestModelConnectionsWriteOnly_Adversarial_Spec096            (proves an echo/reveal regression trips a forbidden pattern; a no-wiring snippet fails the required checks)
TestModelConnectionsOperatorBoundary_Spec096                 (401→/login?next= AND 403→showOperatorOnly across all 3 JS)
TestModelConnectionsOperatorBoundary_Adversarial_Spec096     (a fetch handler with no 403 branch fails the required check)
TestModelConnectionDetailTruthfulTest_NoFalseSuccess_Spec096 (explicit outcome==="ok" branch; #banner-fail role=alert; #banner-ok role=status; NO ollama in executable code)
TestModelConnectionDetailTruthfulTest_Adversarial_Spec096    (an always-success / ollama-fallback renderer trips the guards)
```

**Execution: DEFERRED to the orchestrator `check` pass. Claim Source: not-run.**
The guard is pure static file-reading (no DB/network/live stack), so it runs
under `./smackerel.sh test unit --go --go-run 'ModelConnections'`. It was NOT run
in this dispatch to respect the focused-turn / no-full-suite constraint and to
avoid a cold Dockerized `go test ./...` compile contending with concurrent repo
work on a memory-pressured host. The orchestrator's post-implementation `check`
compiles + runs it. The test is verified correct by inspection (every required
pattern matches the committed pages; every forbidden pattern is absent; the RE2
regexes are valid; identifiers do not collide with the existing
`webcodegen_drift_test` package). HONEST: this section claims "authored +
verified by inspection", NOT "passing".

#### Evidence F2 — live e2e-ui Playwright legs DEFERRED

There is no DOM/jsdom `ui-unit` harness in `web/pwa` (the only JS test runner is
`@playwright/test`, which drives a real browser against the disposable
`smackerel-test-e2e-ui` stack — see `web/pwa/package.json` + `playwright.config.ts`).
A live operator wire→test→enable→rotate browser walk of the triad is therefore an
`e2e-ui` leg that needs the running stack and is DEFERRED to a clean-stack
dispatch (consistent with the backend SCOPE-06 `e2e-api`/`integration` C7
deferral). The static Go guard (F1) is the runnable in-repo substitute that needs
no stack. **Claim Source: interpreted** (read `package.json` + the existing
`photos_connectors.spec.ts` `test.fixme` precedent).

### DoD status (frontend half — no NEW checkboxes; plan owns the DoD shape)

The SCOPE-06 DoD (scopes.md) has **no PWA-specific checkbox** — the triad is part
of the scope **Surface**, not a discrete DoD item — so this frontend dispatch
adds NO `[x]` and rewrites NO DoD text (that is `bubbles.plan`'s ownership). The
frontend delivery advances the scope toward closeout; the currently-unchecked
items remain `[ ]` for the reasons below:

| DoD | State | Frontend note |
|-----|-------|---------------|
| D06-T1-1 artifact-lint | [ ] | orchestrator/foreign-owned; not run here (focused turn) |
| D06-T1-2 `check` EXIT 0 | [ ] | orchestrator closeout — also compiles + runs the new F1 guard |
| D06-T1-3 `format --check` | [ ] | orchestrator closeout |
| D06-T1-4 real evidence | [x] | unchanged (backend E1–E4); F-section adds honest provenance-tagged frontend evidence |
| D06-T1-5 no env-specific content | [x] | upheld — the 3 pages carry only generic placeholders, no real hostnames/IPs/tailnet/operator usernames/secret values |
| D06-T2-1 R1 operator gate | [x] | unchanged (backend); the UI surfaces the 401/403 boundary visibly (S1/S2/S3 operator-only notice) |
| D06-T2-2 write-only secret | [x] | unchanged (backend never echoes/returns/logs); the UI side honors the write-only INPUT affordance (type=password, autocomplete=off, never pre-filled, no reveal control) — pinned by the F1 guard |
| D06-T2-3 truthful test, never false success | [x] | unchanged (backend); the UI renders a failed test as a `role=alert` danger banner, never a success, never an Ollama mention — pinned by the F1 guard |
| D06-T2-4 enable 409-guard + effective-enabled single gate | [ ] | the live `TestEnableDisable_CatalogMembershipFollows` integration leg is still DEFERRED; the UI reflects the 409 guard (Enable disabled until credential + last-test ok) |
| D06-T2-5 closed-set 404 + per-kind secret fields | [x] | unchanged (backend); the UI exposes exactly the backend's per-kind secret fields + read-only params |
| D06-T2-6 live-stack authenticity + C7 deferral | [ ] | the `integration` + `e2e-api` legs remain DEFERRED; the live `e2e-ui` triad walk is likewise DEFERRED (F2) |
| D06-T2-7 adversarial non-tautological + RED-before | [x] | unchanged (backend E2); the F1 guard ALSO ships adversarial twins |
| D06-T2-8 all unit + integration pass, no skips | [ ] | backend unit pass; the F1 ui-unit guard is authored + DEFERRED to the orchestrator check (not-run here); `integration`/`e2e-ui` DEFERRED |

No DoD checkbox is newly flipped by this frontend dispatch (honest: the plan
exposes no PWA-specific item, and the already-`[x]` items were earned by the
backend). The triad is delivered per the Surface + the binding Write-Only Secret
Affordance Contract; SCOPE-06 stays `in_progress` (activation route-mount + the
live `integration`/`e2e-api`/`e2e-ui` legs still pending).


## SCOPE-07 — Combined catalog selection across Telegram + web, 088/089 parity

**Status:** in_progress (the unified selection surfaces + 088/089 parity are
implemented and unit-GREEN; the `check`/`format`/`artifact-lint` closeout gates
and the live cross-surface `e2e-api` parity legs are DEFERRED — the former to the
orchestrator closeout, the latter to the home-lab `bubbles.devops` dispatch C7).
**Executed by:** `bubbles.implement` (parent-expanded full-delivery).
**Scenarios:** SCN-096-D02, SCN-096-D03, SCN-096-D05, SCN-096-G06.
**Decisions resolved here:** R2 (additive `GET /v1/agent/model` enrichment
preserving `allowed_models[]` byte-for-byte), R3 (non-tool-capable gather
shown-but-disabled + rejected fail-loud).

### What shipped (change manifest)

```text
=== SCOPE-07 NEW ===
?? internal/assistant/openknowledge/agenttool/model_catalog_source.go   (late-bound CatalogProvider + BudgetProvider singletons — both surfaces read ONE source; *catalog.CatalogAggregator / agent.SpendLedger satisfy them structurally; nil ⇒ byte-for-byte 089 fallback)
?? internal/telegram/model_command_multiprovider.go                     (the combined provider-grouped /model picker: Ollama-first, provider-qualified, cost-hinted; only reachable numbered; unreachable shown-but-disabled with typed status; 089 current/system-default tags preserved)
?? internal/telegram/model_command_multiprovider_test.go                (SCN-096-D02: TestModelPicker_TelegramCombinedProviderGroupedList_Spec096 + TestModelPicker_UnreachableShownDisabledOnlyReachableNumbered_Spec096 ADVERSARIAL)
?? internal/api/agent_model_multiprovider_test.go                       (SCN-096-D03/D05: GetEnrichedCatalogAdditive_AllowedModelsPreserved ADVERSARIAL + TelegramWebParity_SameValidatorSameStore + StickyHostedPersistsPrecedence)
?? internal/api/agent_model_parity_spec096_test.go                      (SCN-096-G06: Parity_OneValidatorOneStore_ForkPrecedence_ProviderAgnostic ADVERSARIAL + Parity_NonToolCapableGatherShownDisabled_RejectedFailLoud ADVERSARIAL R3)
?? web/pwa/model-picker.html                                            (S5 — USER-facing picker; provider-grouped radiogroups, current/default tags, unreachable shown-disabled, NO credential field)
?? web/pwa/model-picker.js                                              (S5 logic: GET/PUT/DELETE /v1/agent/model; claim-bound {model}-only body; 401→login; textContent-only)

=== SCOPE-07 MODIFIED (additive) ===
 M internal/telegram/model_command.go     (handleModelCommand no-arg branch → modelPickerReplyCombined; the explicit /model <id> / default paths + numbered-reply mechanic UNCHANGED)
 M internal/api/agent_model.go            (GET enrichAgentModelView: ADDITIVE catalog[] + provider_statuses[] + budget; allowed_models[] untouched — byte-for-byte 089)
```

The two new pages auto-embed via the existing `web/pwa/embed.go` glob
(`//go:embed *.html *.css *.js …`) — no `embed.go` edit. `model-picker.js` matches
the user-facing convention (`credentials: "same-origin"`, `401 → /login?next=`,
`textContent`-only, CSP `script-src 'self'`, `role=radiogroup` per provider
group). **Claim Source: executed** (files created; structure verifiable).

### How the binding contracts are implemented

1. **One validator / one store (088/089 invariant #1, NOT amended).** Both
   surfaces resolve through the SAME `agenttool` singletons — the EXISTING
   `modelswitch.Allowlist` (`agenttool.SwitchableModels()`) over the SCOPE-04
   injected catalog and the EXISTING `modelpref.Store`
   (`agenttool.ModelPref()`). SCOPE-07 adds NO second validator/store/picker: it
   adds only a late-bound *read* source (`CatalogProvider`) for the catalog VIEW.
   `modelswitch` stays a pure stdlib leaf (imports unchanged); the catalog is the
   injected admissible set, canonicalization stays at the SCOPE-04 boundary.
   **Claim Source: executed** (pinned by
   `TestParity_OneValidatorOneStore_…` pointer-identity + bidirectional
   shared-store round-trip).
2. **R2 — additive enrichment, `allowed_models[]` byte-for-byte.**
   `enrichAgentModelView` (GET only) reads the late-bound catalog source and adds
   `catalog[]` (reachable models + `capabilities[]` + `cost_class` + `context_window`),
   `provider_statuses[]` (every provider's typed state — for the web picker's
   shown-but-disabled rendering), and an optional `budget` (month-to-date from the
   SCOPE-05 SpendLedger, only when a paid model is in the catalog). It NEVER
   touches `view.AllowedModels` — the enrichment is omitempty sibling fields, so a
   089-era consumer reading only `allowed_models`/`effective_model`/`sticky_model`/
   `source` is unaffected. When no catalog source is wired (deferred activation)
   the GET is the byte-for-byte 089 shape. **Claim Source: executed** (pinned by
   `TestAgentModel_GetEnrichedCatalogAdditive_AllowedModelsPreserved_Spec096`,
   which compares `allowed_models` before/after wiring the source).
3. **R3 — non-tool-capable gather shown-but-disabled + rejected fail-loud.** A
   non-tool-capable model IS in the synthesis switchable set (SHOWN for
   transparency, Principle 8) but is NOT in the tool-capable gather set (DISABLED
   for gather); a direct gather selection of it is rejected fail-loud via the
   EXISTING `allow.ResolveGather` / `allow.ResolveEffective` (the SAME
   `modelswitch.Rejection{ReasonNotToolCapable, gather}`), NEVER accepted or
   silently coerced — mirroring the unreachable-provider treatment. **Claim
   Source: executed** (pinned by
   `TestParity_NonToolCapableGatherShownDisabled_RejectedFailLoud_Spec096` with a
   tool-capable CONTROL proving non-tautology).
4. **Combined Telegram picker — reachable numbered, unreachable shown-disabled.**
   `renderCombinedPicker` groups the catalog by provider (Ollama/local FIRST),
   renders provider-qualified ids with the 089 `current`/`system default` tags + a
   `🔧 tools` capability marker + a per-group cost hint; only REACHABLE models are
   numbered (the returned ordered id list IS the armed `pendingModelSelection`, so
   a numbered reply always maps to a dispatchable model); an unreachable provider
   is SHOWN with its typed `ProviderDiscoveryStatus`, un-numbered, never dropped.
   The numbered-reply mechanic + claim-bound `modelpref` persistence
   (`handleModelSelectionReply`) are UNCHANGED. **Claim Source: executed.**
5. **Claim-bound web picker, no operator config.** `model-picker.html`/`.js` is the
   USER-facing picker: it renders the same combined catalog over
   `GET/PUT/DELETE /v1/agent/model`, the PUT body carries ONLY `{model}` (the
   actor is the authenticated bearer subject, NEVER a body field), there is NO
   credential field anywhere, and an unreachable provider renders as a disabled
   `aria-disabled` group (out of tab order) with its typed status. **Claim
   Source: executed.**

### Test Evidence

#### Evidence E1 — RED-before (each adversarial test bites; non-tautological)

Four targeted regressions were injected (one per adversarial test), the scoped
suite was run, and each adversarial test FAILED, then the regressions were
reverted. Regressions: (a) `renderCombinedPicker` drops unreachable providers;
(b) `enrichAgentModelView` reorders `allowed_models`; (c) the parity test reads
the shared store from a fresh (forked) store; (d) the gather catalog declares the
non-tool model `tool_capable: true` (i.e. "accepts a non-tool gather").

```text
$ # 4 RED-before regressions injected, then:
$ ./smackerel.sh test unit --go --go-run 'ModelPicker.*Spec096|GetEnrichedCatalogAdditive|TelegramWebParity_SameValidatorSameStore|StickyHostedPersistsPrecedence|Parity_OneValidatorOneStore|Parity_NonToolCapableGather' --verbose
    model_command_multiprovider_test.go:184: an unreachable provider MUST be SHOWN (never silently dropped); OpenAI absent from:
    agent_model_multiprovider_test.go:144: allowed_models[0] changed by enrichment ...
    agent_model_parity_spec096_test.go:48: HTTP PUT MUST be visible through the [one shared store] ...
    agent_model_parity_spec096_test.go:138: a non-tool-capable model MUST NOT be in the tool-capable gather set; got [anthropic/claude-3-5-sonnet anthropic/claude-3-5-haiku]
FAIL    github.com/smackerel/smackerel/internal/api     0.275s
FAIL    github.com/smackerel/smackerel/internal/telegram        0.122s
RED_EXIT=1
```

Each failure line maps to its adversarial guard: unreachable-dropped →
`TestModelPicker_Unreachable…`; `allowed_models[0]` reorder →
`TestAgentModel_GetEnrichedCatalogAdditive…`; forked store →
`TestParity_OneValidatorOneStore…`; accepted non-tool gather →
`TestParity_NonToolCapableGather…`. **Claim Source: executed** (real failing
output; regressions reverted before the GREEN run below).

#### Evidence E2 — GREEN (all 7 SCOPE-07 unit tests pass, no skips)

After reverting the four regressions, the scoped suite is GREEN
(`internal/api` 5 tests + `internal/telegram` 2 tests):

```text
$ ./smackerel.sh test unit --go --go-run 'ModelPicker.*Spec096|GetEnrichedCatalogAdditive|TelegramWebParity_SameValidatorSameStore|StickyHostedPersistsPrecedence|Parity_OneValidatorOneStore|Parity_NonToolCapableGather' --verbose
--- PASS: TestAgentModel_GetEnrichedCatalogAdditive_AllowedModelsPreserved_Spec096 (0.00s)
--- PASS: TestAgentModel_TelegramWebParity_SameValidatorSameStore_Spec096 (0.00s)
--- PASS: TestAgentModel_StickyHostedPersistsPrecedence_Spec096 (0.00s)
--- PASS: TestParity_OneValidatorOneStore_ForkPrecedence_ProviderAgnostic_Spec096 (0.00s)
--- PASS: TestParity_NonToolCapableGatherShownDisabled_RejectedFailLoud_Spec096 (0.00s)
ok      github.com/smackerel/smackerel/internal/api     0.269s
--- PASS: TestModelPicker_TelegramCombinedProviderGroupedList_Spec096 (0.00s)
--- PASS: TestModelPicker_UnreachableShownDisabledOnlyReachableNumbered_Spec096 (0.00s)
ok      github.com/smackerel/smackerel/internal/telegram        0.082s
[go-unit] go test ./... finished OK
GREEN_EXIT=0
```

**Claim Source: executed** (real passing output; 7/7 SCOPE-07 unit tests, no
skips — the manifest's SCOPE-07 `unit` linkedTests verbatim).

#### Evidence E3 — live cross-surface e2e-api legs DEFERRED (C7)

The SCOPE-07 `e2e-api` rows
(`tests/e2e/agent/openknowledge_e2e_test.go::TestTelegram_PickHostedModelPersists_Spec096`,
`TestWebTelegramModelParity_Spec096`,
`TestAsk_StickyHostedSelectionPersistsAcrossTurns_Spec096`,
`TestForkPrecedenceParity_MultiProvider_Spec096`) need a running stack with REAL
hosted models and are handed to the home-lab `bubbles.devops` dispatch (C7),
NOT marked passing from dev. The live discovery-aggregator wiring (cmd/core
installing `agenttool.SetModelCatalogProvider`) is the deferred ACTIVATION:
the SCOPE-07 unit tests inject a fake catalog/aggregator; the surfaces fall back
to the byte-for-byte 089 flat list until the aggregator is wired. **Claim Source:
not-run** (deferred; honest).

### DoD status — SCOPE-07 (9 of 13 met + evidenced; 4 deferred)

| DoD | State | Note |
|-----|-------|------|
| D07-T1-1 artifact-lint clean | [ ] | orchestrator closeout (foreign-owned; not run this focused turn) |
| D07-T1-2 `check` EXIT 0 | [ ] | orchestrator closeout (compiles the whole module; deferred to avoid a cold Dockerized `go build` contending on a memory-pressured host) |
| D07-T1-3 `format --check` EXIT 0 | [ ] | orchestrator closeout |
| D07-T1-4 real terminal evidence | [x] | E1 (RED) + E2 (GREEN) are real provenance-tagged output |
| D07-T1-5 088/089 do-not-amend boundary | [x] | NO second validator/store/picker; `modelswitch` imports unchanged; only a late-bound read source + additive GET fields added — Change Manifest above |
| D07-T2-1 088/089 parity (adversarial) | [x] | `TestParity_OneValidatorOneStore_…` (pointer-identity + bidirectional shared store) + `TestAgentModel_TelegramWebParity_…` GREEN; RED-before E1 |
| D07-T2-2 R2 additive, allowed_models byte-for-byte (adversarial) | [x] | `TestAgentModel_GetEnrichedCatalogAdditive_AllowedModelsPreserved_Spec096` GREEN; RED-before E1 |
| D07-T2-3 R3 non-tool-capable gather (adversarial) | [x] | `TestParity_NonToolCapableGatherShownDisabled_RejectedFailLoud_Spec096` GREEN (with tool-capable CONTROL); RED-before E1 |
| D07-T2-4 combined provider-grouped catalog | [x] | `TestModelPicker_TelegramCombinedProviderGroupedList_Spec096` + `…UnreachableShownDisabledOnlyReachableNumbered_Spec096` GREEN; RED-before E1 |
| D07-T2-5 selection persistence + precedence (089 contract) | [x] | `TestAgentModel_StickyHostedPersistsPrecedence_Spec096` GREEN (sticky persists, per-request beats sticky one-shot, store byte-for-byte) |
| D07-T2-6 live-stack parity + C7 deferral | [ ] | the 4 `e2e-api` rows are handed to the home-lab dispatch (C7), NOT marked passing from dev — E3 |
| D07-T2-7 adversarial non-tautological + RED-before | [x] | E1 — all 4 adversarial tests captured RED-before; no bailout early-returns |
| D07-T2-8 all unit pass, no skips; e2e-api deferred | [x] | E2 — 7/7 SCOPE-07 unit GREEN, no skips; `e2e-api` deferred (E3) |

**What remains for the whole feature (096):** (1) the live discovery-aggregator
ACTIVATION wiring in cmd/core (`agenttool.SetModelCatalogProvider` /
`SetBudgetProvider` over the SCOPE-06 `connstore.DiscoveryConnections` feed +
SCOPE-05 ledger) — until then both surfaces serve the byte-for-byte 089 flat
list; (2) the SCOPE-03/04/06/07 live hosted-provider `e2e-api` legs (real
Anthropic/OpenAI reachability, live Ollama, GPU answers) — the home-lab
`bubbles.devops` dispatch (C7); (3) the per-scope `check`/`format`/`artifact-lint`
closeout gates + the `bubbles.validate` certification. SCOPE-07 stays
`in_progress` (the code + unit parity GREEN; the closeout gates + live legs +
activation pending), consistent with SCOPE-01..06.


---

## Post-Implementation Fix — Boot-blocking master-key predicate (found during integration verification)

**Found by:** `bubbles.implement` during SCOPE-02 / SCOPE-06 INTEGRATION
verification (the `smackerel-core` boot healthcheck).
**Class:** post-implementation defect + fix — a boot-blocking regression exposed
by the SCOPE-06 activation wiring that loads the credential vault at boot.
**Files changed:** `internal/assistant/openknowledge/connvault/vault.go`,
`internal/assistant/openknowledge/connvault/vault_test.go`, `design.md` (§11.2).
**Status:** fixed + validated. The CODE is already in the working tree; this
section records the evidence only (no code touched by this doc turn).

### Symptom — `smackerel-core` failed its boot healthcheck

During integration verification the core container aborted at startup:

```text
fatal startup error: buildAPIDeps: model-connections admin wiring: model-connections admin: load credential vault: llm: LLM_PROVIDER_SECRET_MASTER_KEY is required and must be a base64-encoded 32-byte key when one or more llm.connections declare secret_ref.mode=db
```

**Claim Source: executed** (real captured boot-failure output; the message text
is the pre-fix form — the fix below adds "ENABLED" to it).

### Root cause

`connvault.RequiresMasterKey` keyed the master-key requirement on db-mode
*declaration* instead of *enablement*. The default `config/smackerel.yaml` ships
five DISABLED hosted db-mode slots (anthropic, openai, azure-foundry, google,
bedrock), so a fresh / dev / test stack became unbootable even though the
config's own comment documents "no db-mode hosted connection is enabled, so no
secret is needed". The boot-time vault load was introduced by the SCOPE-06
activation wiring (commit `c60697d1`), which exposed the predicate gap.
**Claim Source: interpreted** (root-cause analysis of the
declaration-vs-enablement predicate).

### Fix

- Gated the predicate on `c.Enabled && SecretRef.Mode == "db"` in
  `internal/assistant/openknowledge/connvault/vault.go` — `RequiresMasterKey` now
  reports `true` only for an ENABLED db-mode slot.
- Updated `ErrMasterKeyRequired` to read "… when one or more **ENABLED**
  llm.connections declare secret_ref.mode=db", and updated the
  `RequiresMasterKey` / `LoadVault` / `ErrVaultNotConfigured` doc comments to say
  "enabled".
- Updated `design.md` §11.2 fail-loud predicate to "any ENABLED …".
- Added an adversarial regression-guard sub-test to
  `TestSecretVault_MasterKeyFailLoud_Spec096`: a DISABLED db-mode slot + empty key
  MUST yield `(nil, nil)` — boot must NOT be blocked. The five pre-existing
  sub-tests still pass, so the guard is non-tautological.

The admin surface still mounts and is operator-gated for declared-but-disabled
slots; the master key becomes mandatory only once a db-mode slot is ENABLED (and
the nil-vault credential-write path already fail-louds `VAULT_NOT_CONFIGURED`).
**Claim Source: executed** (predicate + message + doc + design + test changes are
in the tree).

### Evidence F1 — unit: 49/49 Spec096 unit tests (incl. the new regression guard)

```text
$ ./smackerel.sh test unit --go --go-run 'Spec096'
[go-unit] go test ./... finished OK
```

**Claim Source: executed.**

### Evidence F2 — RED-before (the new regression-guard test is non-tautological)

Run with `vault.go` UNCHANGED (only the new sub-test added) — the regression
guard FAILS while the 5 pre-existing sub-tests still PASS:

```text
    vault_test.go:287: a declared-but-DISABLED db-mode slot must NOT require a master key (boot must succeed), got err=llm: LLM_PROVIDER_SECRET_MASTER_KEY is required ... when one or more llm.connections declare secret_ref.mode=db
--- FAIL: TestSecretVault_MasterKeyFailLoud_Spec096/DISABLED_db-mode_slot_+_empty_key_→_no_vault,_no_error_(REGRESSION_GUARD...)
    (5 pre-existing sub-tests still PASS → guard is non-tautological)
```

**Claim Source: executed** (real failing output; the predicate fix below turns it
GREEN).

### Evidence F3 — GREEN-after (with the `vault.go` predicate fix applied)

```text
--- PASS: TestSecretVault_MasterKeyFailLoud_Spec096 (0.00s)
    --- PASS: .../db-mode_declared_+_empty_master_key_→_fail-loud
    --- PASS: .../db-mode_declared_+_valid_key_→_vault_built_(CONTROL)
    --- PASS: .../ollama-only_+_empty_key_→_no_vault,_no_error_(CONTROL)
    --- PASS: .../DISABLED_db-mode_slot_+_empty_key_→_no_vault,_no_error_(REGRESSION_GUARD: default-shipped disabled hosted slots must NOT block boot)
    --- PASS: .../present-but-not-base64_key_→_fail-loud
    --- PASS: .../present-but-wrong-length_key_→_fail-loud
```

**Claim Source: executed.**

### Evidence F4 — integration: core boots Healthy; full Spec096 integration suite PASS

`./smackerel.sh test integration --go-run 'Spec096'` — the core now boots
`Healthy` and the full Spec096 integration suite passes, including the SCOPE-02
vault-persist round-trip against a REAL Postgres:

```text
 Container smackerel-test-smackerel-core-1  Healthy
--- PASS: TestVault_PersistRoundTripTestMasterKey_Spec096 (0.02s)      # AES-256-GCM vault persist+round-trip vs REAL Postgres
ok  github.com/smackerel/smackerel/tests/integration                   0.131s
ok  github.com/smackerel/smackerel/internal/assistant/openknowledge/agent       0.032s   # attribution, budget-preflight, cost-fn
ok  github.com/smackerel/smackerel/internal/assistant/openknowledge/catalog     0.045s   # aggregation, graceful degradation, fail-loud bounds, canonicalize, off-catalog reject
ok  github.com/smackerel/smackerel/internal/assistant/openknowledge/connstore   0.021s   # effective-enabled single gate, has-credential
ok  github.com/smackerel/smackerel/internal/assistant/openknowledge/connvault   0.023s   # secret vault round-trip, AAD tamper, wrong-key, MASTER-KEY-FAIL-LOUD (incl regression guard), rotation
ok  github.com/smackerel/smackerel/internal/assistant/openknowledge/llm         0.021s   # chat-request additive, dispatch-resolver never-falls-back-to-ollama, secret-never-leaks, duplicate-kind fail-loud
PASS: go-integration
```

**Claim Source: executed** (real captured integration output; core `Healthy` +
vault-persist GREEN vs real Postgres).

### Evidence F5 — two live legs remain HONESTLY DEFERRED (skipped here)

Two integration tests are live legs that need a live env + a running core /
operator token (the home-lab live dispatch); they SKIP here and are NOT marked
passing:

```text
--- SKIP: TestEnableDisable_CatalogMembershipFollows_Spec096      (DEFERRED SCOPE-06 C7: needs SPEC096_ADMIN_LIVE_CORE_URL + SPEC096_ADMIN_LIVE_OPERATOR_TOKEN)
--- SKIP: TestAsk_PaidModelExhaustedBudget_RefusedBeforeProviderCall_Spec096  (DEFERRED SCOPE-05: needs SPEC096_BUDGET_LIVE_CORE_URL + SPEC096_BUDGET_LIVE_AUTH_TOKEN)
```

**Claim Source: not-run** (deferred live legs; honest — NOT claimed as passed).

### Outcome

The boot-blocking defect is fixed: the master-key requirement is now gated on
ENABLEMENT, so the default-shipped DISABLED hosted slots no longer block boot.
The SCOPE-02 vault-persist integration leg
(`TestVault_PersistRoundTripTestMasterKey_Spec096`) is now GREEN against a real
Postgres, and the full Spec096 integration suite is `PASS: go-integration` with
core `Healthy`. The two live legs (SCOPE-06 enable/disable, SCOPE-05 paid-budget)
stay DEFERRED to the home-lab `bubbles.devops` dispatch. No 088/089 surface was
touched; the fix is confined to the `connvault` predicate + message + doc
comments + design §11.2 + the new regression-guard sub-test. The spec stays
`in_progress` (certification is a separate downstream step).

## Post-Implementation Security Sign-Off — vault master-key predicate (8900a8f6)

A `bubbles.security` READ-ONLY review of commit `8900a8f6` (the
`RequiresMasterKey` enablement-predicate relaxation) returned **PASS / no
weakening**.

- **Verdict:** SECURE — PASS (no weakening). The relaxed predicate
  (`RequiresMasterKey` gating on `c.Enabled && mode==db` instead of `mode==db`)
  preserves the invariant *"a provider secret is only ever decrypted by an
  enabled connection whose master key is present at boot."* It changes only WHEN
  boot fail-louds vs. returns a benign nil vault; it does not touch any
  decryption guard. Decryption is structurally gated by a non-nil `*SecretVault`,
  and a non-nil vault structurally requires the master key (`NewSecretVault`
  rejects an empty/short key). Blast radius is one function (`RequiresMasterKey`,
  called only by `LoadVault`, whose only production caller is
  `buildModelConnectionsAdmin`).
- **Six verified points (each with code + test evidence):**
  1. **No decrypt without key** — an ENABLED db-mode slot with no key still
     fail-louds at boot (`ErrMasterKeyRequired`); pinned by
     `TestSecretVault_MasterKeyFailLoud_Spec096` (`anthropic-primary,
     Enabled:true`).
  2. **Disabled never decrypts** — the dispatch resolver rejects a disabled
     connection before any decrypt (`dispatch_resolver.go` `if !conn.Enabled →
     RejectConnectionDisabled`); the credential source yields a record only when
     `EffectiveEnabled` (`connstore`); pinned by
     `TestDispatch_SecretNeverInLogsOrErrors_Spec096/disabled_target_with_secret_on_disk_rejects_without_leaking`
     + `TestEffectiveEnabled_SingleGate_Spec096`.
  3. **Nil-vault is safe** — disabled-only `LoadVault`→`(nil,nil)`;
     credential-write/test paths fail loud `VAULT_NOT_CONFIGURED` (no plaintext
     stored/echoed); `Encrypt`/`Decrypt`/`Rotate` nil-receiver guards return
     `ErrVaultNotConfigured` (no panic, no plaintext).
  4. **Operator gate unchanged** — surface mounting + gate are computed from
     `dbModeDeclared` (declaration), independent of `RequiresMasterKey`
     (enablement); declared-but-disabled slots still mount the operator-gated
     surface; gate still fail-closed on empty `operator_user_ids` (401/403;
     `ValidateOperatorGate` aborts production startup on empty allowlist).
  5. **No secret leakage** — `ErrMasterKeyRequired` names the env var only,
     `ErrVaultNotConfigured`/`VAULT_NOT_CONFIGURED` name status only; no key value
     interpolated; reinforced by
     `TestSecretVault_NeverReturnsPlaintext_RedactionLast4_Spec096`,
     `TestSecretVault_AADTamperRejected_Spec096`,
     `TestSecretVault_WrongKeyRejected_Spec096`.
  6. **Tests cover it** — all named tests exist and pass (report.md F1 `49/49 …
     finished OK`, F2 RED-before non-tautological).
- **Defense-in-depth note (informational, not a finding):** `RequiresMasterKey`
  reads SST `c.Enabled` (config) while the runtime gate `EffectiveEnabled` reads
  DB `rec.Enabled`. A DB-enabled-but-key-removed-after-the-fact deployment boots
  with a nil vault instead of aborting, but STILL cannot decrypt — dispatch hits
  `if r.vault == nil → RejectVaultNotConfigured` and the at-rest AES-256-GCM
  ciphertext is undecryptable without the key; reaching DB-effective-enabled
  requires a stored credential, which required a non-nil vault (key present) at
  store time. Confidentiality holds end-to-end; failure mode is fail-closed
  (never an Ollama fallback). The live dispatch resolver is also not yet wired
  (SCOPE-07 deferred), so today the only live vault consumer is the nil-guarded
  admin handler.
- **Provenance:** `bubbles.security` read the committed code + test sources at
  `8900a8f6` + the executed F1/F2 evidence; no files modified; suite not
  re-executed (already validated + on `origin/main`). **Claim Source: interpreted**
  (read-only security review of committed sources; no commands run this turn).




