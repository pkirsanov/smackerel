# Design: 052 Bundle Secret Injection Contract

## Design Brief

**Current State.** Spec 051 added a defense-in-depth gate at [scripts/commands/config.sh:413-433](../../scripts/commands/config.sh) that rejects dev-default Postgres passwords when `TARGET_ENV=home-lab`. The CI matrix in [.github/workflows/build.yml:191-211](../../.github/workflows/build.yml) builds three bundles per source SHA (`dev`, `test`, `home-lab`) by invoking `./smackerel.sh config generate --env <env> --bundle ...`. The home-lab leg trips that gate because `infrastructure.postgres.password: "smackerel"` flows through unchanged, blocking every push to `main`. The bundle's value is also semantically meaningless at apply time because [knb/smackerel/home-lab/apply.sh:660-668](../../../knb/smackerel/home-lab/apply.sh) already overrides it via a second `--env-file` from the sops-decrypted secrets file. (Bundle generation block: [scripts/commands/config.sh:1460-1556](../../scripts/commands/config.sh).)

**Target State.** A single SST-owned secret-key manifest at `infrastructure.secret_keys` declares which keys must be substituted at apply time rather than inlined into the bundle. For production-class targets (declared via `infrastructure.production_class_targets`), the SST loader emits `__SECRET_PLACEHOLDER__<KEY>__` instead of literal yaml values, and the bundle ships a sibling `secret-keys.yaml` enumerating the expected placeholder set. The knb home-lab adapter validates substitution completeness AND scans post-up that zero placeholder markers remain in resolved env. The Go runtime refuses to start if any placeholder slips through. Defense in depth at three layers (loader, adapter, runtime).

**Patterns to Follow.**
- **SST + Go-mirror + shell-mirror triple, drift-detected by contract test** — established by spec 051 with [internal/config/secrets.go:18-25](../../internal/config/secrets.go) (`var DevDBPasswords`) mirrored to [scripts/commands/config.sh:421-431](../../scripts/commands/config.sh). Spec 052 extends this pattern: yaml manifest → [internal/config/secret_keys.go](../../internal/config/secret_keys.go) (new) → shell mirror in `scripts/commands/config.sh` placeholder-emit block (new).
- **Contract test in `internal/deploy/`** — established by [internal/deploy/build_workflow_bundle_hash_contract_test.go](../../internal/deploy/build_workflow_bundle_hash_contract_test.go), [internal/deploy/compose_contract_test.go](../../internal/deploy/compose_contract_test.go), [internal/deploy/build_workflow_vuln_gate_contract_test.go](../../internal/deploy/build_workflow_vuln_gate_contract_test.go). Adversarial sub-tests are mandatory.
- **Two-`--env-file` Compose override semantics** — established by [knb/smackerel/home-lab/apply.sh:660-668](../../../knb/smackerel/home-lab/apply.sh) (`--env-file "$COMPOSE_DIR/app.env" --env-file "$SECRETS_TMPFILE"`). Compose's documented "later wins" semantics already do the substitution; spec 052 adds validation, NOT a new substitution mechanism.
- **Fail-loud SST without colon-dash defaults** — Gate G028 (`smackerel-no-defaults` skill). The placeholder marker is the explicit "missing-real-value-here" signal; an unsubstituted placeholder reaching runtime is a fail-loud event, not a silent fallback.
- **FR-051-007 redaction discipline** — error messages name the offending KEY, never the value. New runtime error in `Validate()` follows the same shape as [internal/config/config.go:1457-1461](../../internal/config/config.go).

**Patterns to Avoid.**
- ❌ Don't introduce a new `--placeholder-mode` CLI flag on `./smackerel.sh config generate`. Implicit behavior driven by `TARGET_ENV ∈ production_class_targets` keeps the CI invocation in [.github/workflows/build.yml:203](../../.github/workflows/build.yml) unchanged across all 3 matrix legs.
- ❌ Don't add per-section flags like `infrastructure.postgres.password.secret: true`. That scatters the contract across yaml sections — drift-prone, hard to grep, hard to diff. Top-level array is one file location.
- ❌ Don't include a source-SHA nonce in the placeholder. It would couple the bundle's secret-key surface to source SHA visibly and break determinism reasoning. Pure key-derived `__SECRET_PLACEHOLDER__<KEY>__` is grep-trivial.
- ❌ Don't introduce an explicit pre-substitution step that produces `app.env.resolved`. The existing two-`--env-file` Compose semantics already work; spec 052 should not invent a parallel substitution path the trust boundary already provides.
- ❌ Don't sign the `secret-keys.yaml` separately. The whole bundle is already cosign-signed and sha256-pinned (spec 047 R13); tampering with `secret-keys.yaml` breaks the bundle hash → caught by the existing adapter sha256 verification BEFORE extraction.
- ❌ Don't default `connector.enabled` connectors into the manifest in this spec. IP-052-004 backfill is a separate follow-up; spec 052 ships the manifest infrastructure with the 4 already-required keys only.

**Resolved Decisions.**
- OQ-052-01 → top-level array `infrastructure.secret_keys: [...]`.
- OQ-052-02 → `__SECRET_PLACEHOLDER__<KEY>__` (deterministic, key-derived, no nonce).
- OQ-052-03 → existing two-`--env-file` semantics + post-up scan (no new substitution step).
- OQ-052-04 → Go contract test in `internal/deploy/bundle_secret_contract_test.go`.
- OQ-052-05 → opt-in via `infrastructure.production_class_targets: [home-lab]` (explicit > implicit).
- OQ-052-06 → out of scope; bundle cosign + sha256 already cover tampering with sibling files.

**Open Questions.** None. All 6 OQ-052-NN have resolutions captured in the Decision Summary.

---

## Decision Summary

| OQ | Question | Resolution | Rationale (1 line) |
|----|----------|------------|---------------------|
| OQ-052-01 | Where does the secret-key manifest live? | Top-level array `infrastructure.secret_keys: [...]` in [config/smackerel.yaml](../../config/smackerel.yaml). | Single grep target; one diff to add a new secret; mirrors spec 051's `var DevDBPasswords` shape. |
| OQ-052-02 | Placeholder format? | `__SECRET_PLACEHOLDER__<KEY>__` (no nonce). | Deterministic; pure-key-derived; trivial grep; uniqueness from format alone. |
| OQ-052-03 | Substitution mechanism? | Existing two-`--env-file` Compose override + post-up `docker compose config` scan. | Preserves spec 051 trust boundary; adapter only adds a verification step, not a new substitution path. |
| OQ-052-04 | Where does the contract test live? | `internal/deploy/bundle_secret_contract_test.go` (Go). | Aligns with spec 047's existing contract-test pattern in same package. |
| OQ-052-05 | How are new production targets opted in? | Explicit declaration in `infrastructure.production_class_targets: [home-lab]`. | Explicit > implicit; prevents accidental opt-in via target-name typo; forces spec 052 contract awareness when adding a new target. |
| OQ-052-06 | Does the contract need additional bundle-time provenance for `secret-keys.yaml`? | No. The whole bundle is already cosign-signed and sha256-verified (spec 047 R13). Tampering with `secret-keys.yaml` breaks the bundle hash → caught by existing adapter sha256 check BEFORE extraction. | No additional signing surface needed; existing layer covers the threat. |

---

## Architecture (3-Layer Defense-in-Depth)

```
┌──────────────────────────────────────────────────────────────────────┐
│ L1: SST LOADER (build time, in CI or operator workstation)            │
│ scripts/commands/config.sh + internal/config/secret_keys.go (mirror)  │
│                                                                       │
│   for KEY in infrastructure.secret_keys:                              │
│     if TARGET_ENV in infrastructure.production_class_targets:         │
│       app.env: KEY=__SECRET_PLACEHOLDER__<KEY>__                      │
│       (skip FR-051-005 dev-default check for this key)                │
│     else:                                                             │
│       app.env: KEY=<literal yaml value>                               │
│       (FR-051-005 dev-default check still fires for actual literals)  │
│                                                                       │
│   bundle ships sibling: secret-keys.yaml (enumerates declared keys)   │
└─────────────────────────────────┬────────────────────────────────────┘
                                  │ tar.gz, deterministic
                                  │ cosign-signed, sha256-pinned
                                  ▼
┌──────────────────────────────────────────────────────────────────────┐
│ L2: KNB ADAPTER (apply time, on target host with sops + age key)      │
│ knb/smackerel/home-lab/apply.sh                                       │
│                                                                       │
│   1. Verify bundle cosign signature (existing)                        │
│   2. Verify bundle sha256 against build manifest (existing)           │
│   3. Extract bundle → COMPOSE_DIR (existing)                          │
│   4. NEW: parse secret-keys.yaml from extracted bundle                │
│   5. NEW: assert every declared key has placeholder in app.env        │
│   6. sops -d secrets file → tmpfile chmod 0600 (existing)             │
│   7. NEW: assert every declared key has real value in tmpfile         │
│           (non-empty AND not equal to its placeholder marker)         │
│   8. docker compose --env-file app.env --env-file tmpfile up (existing│
│      — Compose's "later wins" override does the substitution)         │
│   9. NEW: docker compose --env-file ... config | grep __SECRET_       │
│           → MUST find zero placeholder markers in resolved env        │
│  10. Audit log: secrets_substituted=N placeholders_remaining=0 (NEW)  │
└─────────────────────────────────┬────────────────────────────────────┘
                                  │ docker compose up -d
                                  ▼
┌──────────────────────────────────────────────────────────────────────┐
│ L3: GO RUNTIME (startup time, inside smackerel-core container)        │
│ internal/config/config.go::Validate()                                 │
│ internal/auth/startup.go::ValidateRuntimeAuthStartup()                │
│                                                                       │
│   for KEY in internal/config/secret_keys.go::SecretKeys():            │
│     if env[KEY] == __SECRET_PLACEHOLDER__<KEY>__:                     │
│       return fmt.Errorf("KEY still equals placeholder marker         │
│                          (spec 052 FR-052-007)")                      │
│       (FR-051-007 redaction: name KEY, never echo placeholder/value)  │
└──────────────────────────────────────────────────────────────────────┘
```

**Trust boundary.** L1 runs in CI with no production secret access. L2 runs on the operator's target host with sops + age key access. L3 runs inside the container with only env vars. Each layer assumes the layer below it may be compromised AND each layer fails loud. Compromising any one layer does not leak production secrets nor allow a placeholder-as-credential boot.

---

## Manifest Schema

Add to [config/smackerel.yaml](../../config/smackerel.yaml) under the existing `infrastructure:` block:

```yaml
infrastructure:
  # ... existing keys (postgres, nats, ollama, ...) ...

  # Spec 052 FR-052-001 — SST-owned secret-key manifest.
  # Keys listed here are NEVER inlined into bundles for targets in
  # production_class_targets below. Instead, the SST loader emits
  # __SECRET_PLACEHOLDER__<KEY>__ in app.env and the deploy adapter
  # substitutes the real value at apply time from a sops-encrypted
  # secrets file (knb/smackerel/secrets/<target>.enc.env).
  #
  # Mirror locations (drift detected by contract test):
  #   - Go: internal/config/secret_keys.go::SecretKeys()
  #   - Shell: scripts/commands/config.sh placeholder-emit block
  #
  # To add a new managed secret: (1) add to this list, (2) add to Go
  # mirror, (3) add to shell mirror, (4) ship a real value in
  # knb/smackerel/secrets/<target>.enc.env. The contract test catches
  # the gap if any of (1)/(2)/(3) is forgotten.
  secret_keys:
    - POSTGRES_PASSWORD
    - AUTH_SIGNING_ACTIVE_PRIVATE_KEY
    - AUTH_AT_REST_HASHING_KEY
    - AUTH_BOOTSTRAP_TOKEN

  # Spec 052 FR-052-002 — explicit opt-in for placeholder emission.
  # When TARGET_ENV ∈ this list, the SST loader emits placeholders for
  # every key in secret_keys above. dev/test environments are NEVER in
  # this list (they keep inline values for local-dev convenience per
  # FR-052-011). Adding a new production target = explicit one-line
  # change here AND adapter follow-up in the knb repo.
  production_class_targets:
    - home-lab
```

**Initial values rationale.**
- `POSTGRES_PASSWORD` — spec 051 FR-051-005 already requires real value for home-lab; this routes it through the placeholder pipeline.
- `AUTH_SIGNING_ACTIVE_PRIVATE_KEY`, `AUTH_AT_REST_HASHING_KEY` — spec 044 [internal/auth/startup.go:46-58](../../internal/auth/startup.go) requires non-empty values for production with auth enabled. Today they ship empty in the bundle and the adapter `home-lab.enc.env` provides them; making them placeholders makes the contract explicit AND lets the adapter validation catch a missing entry.
- `AUTH_BOOTSTRAP_TOKEN` — spec 051 FR-051-004 always-required-in-production semantics (until operator clears).
- **NOT in initial set:** connector API keys (LLM, Telegram, Discord, Hospitable, Twitter, GuestHost, Finnhub, FRED, AirNow, QF). Per spec.md IP-052-004, those are gated by `<connector>.enabled=false` today and are backfilled in a separate follow-up spec once the manifest infrastructure is proven. Spec 052 ships the rails; IP-052-004 widens the train.

---

## SST Loader Behavior

**File:** [scripts/commands/config.sh](../../scripts/commands/config.sh)

**Today (lines 413-433, spec 051 FR-051-005):**
```bash
case "$TARGET_ENV" in
  home-lab)
    case "$(printf '%s' "$POSTGRES_PASSWORD" | tr '[:upper:]' '[:lower:]')" in
      smackerel|postgres|password|changeme|change-me|default)
        echo "ERROR: ... dev-default value ..." >&2
        exit 1 ;;
    esac ;;
esac
```

**Spec 052 changes (apply in this order, after the per-key `required_value` resolution and BEFORE the dev-default rejection):**

1. **Read the manifest** (new helper, near the top of the file alongside `yaml_get`):
   ```bash
   # Returns space-separated list of keys; empty if manifest missing.
   secret_keys_list() { yaml_array_get "infrastructure.secret_keys"; }
   production_class_targets_list() { yaml_array_get "infrastructure.production_class_targets"; }
   ```
   Implementation: a parsing helper that walks the yaml block and emits each `- ITEM` value on its own line (mirrors existing `yaml_get` discipline; no new dependencies).

2. **Build the canonical Go-mirror+shell-mirror reference list** at the top of `config.sh`:
   ```bash
   # MUST mirror internal/config/secret_keys.go::SecretKeys().
   # Drift detected by internal/deploy/bundle_secret_contract_test.go.
   SHELL_SECRET_KEYS=(POSTGRES_PASSWORD AUTH_SIGNING_ACTIVE_PRIVATE_KEY \
                      AUTH_AT_REST_HASHING_KEY AUTH_BOOTSTRAP_TOKEN)
   ```

3. **At each managed-key resolution point** (e.g., after `POSTGRES_PASSWORD="$(required_value infrastructure.postgres.password)"` at [config.sh:412](../../scripts/commands/config.sh)), wrap the value with placeholder logic:
   ```bash
   # Spec 052 FR-052-002 — placeholder emission for production-class targets.
   if is_production_class_target "$TARGET_ENV" && in_secret_keys POSTGRES_PASSWORD; then
     POSTGRES_PASSWORD="__SECRET_PLACEHOLDER__POSTGRES_PASSWORD__"
   else
     # Existing FR-051-005 dev-default check fires HERE (only for literal values).
     case "$TARGET_ENV" in
       home-lab) case "$(printf '%s' "$POSTGRES_PASSWORD" | tr ...)" in ... ;; esac ;;
     esac
   fi
   ```

   **Critical reorder per spec.md FR-052-010:** the FR-051-005 dev-default check now fires only when emitting a literal value — i.e., placeholder mode short-circuits the check for the placeholder path. The check still fires when an operator explicitly env-overrides with a dev-default (e.g., `POSTGRES_PASSWORD=smackerel ./smackerel.sh config generate --env home-lab --bundle`), because in that case `required_value` returns the env-override literal which is no longer in placeholder mode (env override beats yaml; the resulting literal must still pass the dev-default gate). This preserves BS-052-006 (regression scenario).

4. **Bundle now ships `secret-keys.yaml`** (new, in the bundle staging block at [config.sh:1495-1525](../../scripts/commands/config.sh)):
   ```bash
   # Spec 052 FR-052-003 — sibling manifest enumerating declared keys.
   {
     echo "# Spec 052 FR-052-003 — keys substituted at apply time."
     echo "secretKeys:"
     for key in "${SHELL_SECRET_KEYS[@]}"; do
       echo "  - $key"
     done
   } > "$STAGE_DIR/secret-keys.yaml"
   chmod 0644 "$STAGE_DIR/secret-keys.yaml"
   ```
   Add `"secret-keys.yaml"` to the `TAR_FILES` array (sorted: between `prompt_contracts` and the trailing entries) and to the `bundle-manifest.yaml` `files:` list.

5. **Determinism preserved.** Placeholder is purely key-derived (no timestamp, no random, no source SHA). Bundle reproduces byte-identically across two invocations with identical inputs. Per spec.md NFR "Determinism".

6. **Unclassified-key drift detection** is the contract test's job (see Test Plan §8).

---

## Knb Adapter Behavior

**File:** [knb/smackerel/home-lab/apply.sh](../../../knb/smackerel/home-lab/apply.sh)

**Today** (lines 562-668): adapter decrypts secrets file, computes sha256, runs `docker compose ... --env-file app.env --env-file <tmpfile> up -d --pull never`. The two-`--env-file` order means Compose resolves variables from `tmpfile` last, overriding `app.env`. No validation that placeholder substitution is complete.

**Required diff (spec 052 ships smackerel-side; knb adapter PR is separate):**

| Step | Insertion point | Action |
|------|------------------|--------|
| A | After bundle extraction, before sops decrypt | Parse `secret-keys.yaml` from extracted bundle. Read declared keys into `BUNDLE_SECRET_KEYS` array. Fail loud if file missing. |
| B | After step A, before sops decrypt | Validate every key in `BUNDLE_SECRET_KEYS` exists in extracted `app.env` AS a placeholder marker (literal `__SECRET_PLACEHOLDER__<KEY>__`). Fail loud (and name the offending KEY only) if any key is missing or has a literal value. |
| C | After sops decrypt, before docker compose up | Validate every key in `BUNDLE_SECRET_KEYS` exists in decrypted tmpfile with a non-empty AND non-placeholder value. Fail loud (KEY name only) on any miss. |
| D | After existing `docker compose up`, before `verify.sh` | Run `docker compose --env-file "$COMPOSE_DIR/app.env" --env-file "$SECRETS_TMPFILE" --project-name "$COMPOSE_PROJECT" config` and grep stdout for `__SECRET_PLACEHOLDER__`. MUST find zero matches. Fail loud if any remain (defense against misconfigured Compose override). |
| E | Audit log line ([apply.sh:155](../../../knb/smackerel/home-lab/apply.sh)) | Add `secrets_substituted=<N>` (count of declared keys validated) and `placeholders_remaining=<M>` (always 0 on success path, ≥1 on the failure path that caused step D to abort). |

**Compose `up` invocation unchanged** ([apply.sh:660-668](../../../knb/smackerel/home-lab/apply.sh)). The two-`--env-file` order is correct as written; Compose's "later wins" semantics resolve every placeholder when the second `--env-file` declares the real value. Spec 052 only adds verification, not a new substitution path.

**Required-knb-follow-up flag: YES.** Spec 052 design, scopes, and implementation cover the smackerel-side changes (loader, manifest, runtime, contract test, bash unit test). The matching knb adapter PR (steps A-E above) is filed under a separate spec in the knb repo and tracked as a hard cross-repo dependency for go-live: smackerel-side spec 052 lands first (CI green); knb-side adapter spec lands before the next home-lab apply.

---

## Runtime Behavior

**File 1:** [internal/config/config.go](../../internal/config/config.go), `func (c *Config) Validate()` ([config.go:1418-1499+](../../internal/config/config.go))

**Insertion point:** After the existing FR-051-005 dev-default check at line 1457-1461, BEFORE the auth-token format checks.

```go
// Spec 052 FR-052-007 — defense-in-depth: refuse to start if any
// secret env var still equals its placeholder marker. Catches an
// adapter-layer regression that fails to substitute. The error names
// the offending KEY without echoing the placeholder string OR the
// real value (FR-051-007 redaction contract extended).
for _, key := range SecretKeys() {
    if v := os.Getenv(key); IsPlaceholder(v) {
        return fmt.Errorf("%s still equals placeholder marker — adapter substitution failed (spec 052 FR-052-007)", key)
    }
}
```

Note: the actual value comparison reads from the resolved `Config` struct fields (e.g., `c.Postgres.Password`) for keys already mapped into struct fields, falling back to `os.Getenv` for non-struct keys. Implementation phase determines per-key dispatch; design contract is "every declared key MUST be checked against `IsPlaceholder` before `Validate()` returns nil".

**File 2:** [internal/auth/startup.go](../../internal/auth/startup.go), `func ValidateRuntimeAuthStartup` ([startup.go:43-62](../../internal/auth/startup.go))

**Insertion point:** After each non-empty check at lines 50, 53, 56, before the inequality check at line 59. For each AUTH_* key in the secret manifest, also call `config.IsPlaceholder` and return the same FR-051-007-shaped error if it matches.

**File 3 (new):** [internal/config/secret_keys.go](../../internal/config/secret_keys.go)

```go
package config

import "strings"

// Spec 052 FR-052-001 — Go-side mirror of the SST secret-key manifest in
// config/smackerel.yaml::infrastructure.secret_keys. Drift between this list,
// the yaml manifest, and the shell mirror in scripts/commands/config.sh is
// detected by internal/deploy/bundle_secret_contract_test.go.
//
// To add a managed secret: add the KEY here AND in the two mirror locations.
var secretKeys = []string{
    "POSTGRES_PASSWORD",
    "AUTH_SIGNING_ACTIVE_PRIVATE_KEY",
    "AUTH_AT_REST_HASHING_KEY",
    "AUTH_BOOTSTRAP_TOKEN",
}

// SecretKeys returns the canonical list of env var names whose values are
// substituted at apply time by the deploy adapter rather than inlined into
// the SST bundle. Returned slice is a defensive copy; callers may mutate.
func SecretKeys() []string {
    out := make([]string, len(secretKeys))
    copy(out, secretKeys)
    return out
}

// IsPlaceholder reports whether v equals the canonical
// __SECRET_PLACEHOLDER__<KEY>__ marker for any KEY in SecretKeys(). Empty
// input returns false (empty handling is the caller's responsibility; the
// SST loader rejects empty values via required_value before this helper is
// ever consulted at runtime).
func IsPlaceholder(v string) bool {
    if !strings.HasPrefix(v, "__SECRET_PLACEHOLDER__") || !strings.HasSuffix(v, "__") {
        return false
    }
    for _, k := range secretKeys {
        if v == "__SECRET_PLACEHOLDER__"+k+"__" {
            return true
        }
    }
    return false
}
```

**Redaction contract.** Every error path in `Validate()` and `ValidateRuntimeAuthStartup` that mentions a placeholder MUST name the KEY only. Never echo the placeholder marker (which would teach an attacker the exact format), never echo the resolved value. Spec 051 FR-051-007 extended.

---

## CI Wiring

**File:** [.github/workflows/build.yml](../../.github/workflows/build.yml), `build-bundles` matrix ([lines 191-211](../../.github/workflows/build.yml))

**Change required: NONE.** The matrix invocation:
```yaml
- name: Generate config bundle for ${{ matrix.env }}
  run: |
    ./smackerel.sh config generate --env ${{ matrix.env }} --bundle --output-dir dist/config-bundles/
```
continues to work unchanged for all 3 entries (`dev`, `test`, `home-lab`). Placeholder mode is implicit when `TARGET_ENV ∈ infrastructure.production_class_targets`. dev and test legs continue to ship inline values per FR-052-011. The home-lab leg now succeeds because the spec 051 FR-051-005 dev-default check no longer fires for the `POSTGRES_PASSWORD` placeholder path.

**No new GitHub Actions secret required.** No `CI_BUNDLE_POSTGRES_PASSWORD` or equivalent. CI's zero-production-secret-access invariant is preserved per spec.md Hard Constraint 3.

`publish-build-manifest` ([build.yml:253-330](../../.github/workflows/build.yml)) continues to enumerate all 3 bundles unchanged; the home-lab `configBundles[*]` entry now writes successfully with a real sha256 per source SHA.

---

## Test Plan

| Test Type | Category | File / Location | Description | Command |
|-----------|----------|------------------|-------------|---------|
| **unit (bash)** | `unit` | `tests/config/placeholder_emit_test.sh` (new) | Pinned input → pinned output: invoke `./smackerel.sh config generate --env home-lab --bundle` against a fixture yaml, assert `app.env` contains `POSTGRES_PASSWORD=__SECRET_PLACEHOLDER__POSTGRES_PASSWORD__` for each declared key. | `./smackerel.sh test unit` |
| **unit (Go)** | `unit` | `internal/config/secret_keys_test.go` (new) | (a) `SecretKeys()` returns a defensive copy, (b) `IsPlaceholder("__SECRET_PLACEHOLDER__POSTGRES_PASSWORD__") == true`, (c) `IsPlaceholder("smackerel") == false`, (d) `IsPlaceholder("__SECRET_PLACEHOLDER__UNKNOWN_KEY__") == false`, (e) `IsPlaceholder("") == false`. | `./smackerel.sh test unit` |
| **contract (Go)** | `contract` | `internal/deploy/bundle_secret_contract_test.go` (new) | Runs `./smackerel.sh config generate --env home-lab --bundle --output-dir <tmp>`, extracts the resulting tar.gz, asserts: **(a)** every key in `config.SecretKeys()` appears in `app.env` as `__SECRET_PLACEHOLDER__<KEY>__`. **(b)** No value from `internal/config/secrets.go::DevDBPasswords` appears anywhere in `app.env` (grep for each literal). **(c)** Sibling `secret-keys.yaml` exists, parses as yaml, and its `secretKeys` field equals `config.SecretKeys()`. **(d) ADVERSARIAL 1:** mutate the shell mirror's `SHELL_SECRET_KEYS` array to drop one key, re-run loader, assert test fails (drift detector). **(e) ADVERSARIAL 2:** patch `config/smackerel.yaml` so a manifest key resolves to a literal in `app.env` (i.e., placeholder mode bypassed), assert test fails (leakage detector). **(f) ADVERSARIAL 3:** invoke loader twice, sha256 the two bundles, assert equal (determinism detector — fails if a non-deterministic placeholder is reintroduced). **(g) ADVERSARIAL 4:** patch `config/smackerel.yaml` to remove `home-lab` from `production_class_targets`, re-run loader, assert test fails (regression detector for the explicit-opt-in invariant). | `go test -v -count=1 -run BundleSecretContract ./internal/deploy/` |
| **integration** | `integration` | `tests/config/bundle_home_lab_integration_test.sh` (new or extension of existing config-test) | End-to-end: `./smackerel.sh config generate --env home-lab --bundle --output-dir <tmp>` succeeds (exit 0). Bundle SHA stable across 2 invocations. `tar tzf <bundle>` shows `secret-keys.yaml`. `tar xzf <bundle> ./app.env -O \| grep -c __SECRET_PLACEHOLDER__` returns ≥4 hits (one per declared key). `tar xzf <bundle> ./app.env -O \| grep -E '^POSTGRES_PASSWORD=smackerel$'` returns 0 lines. | `./smackerel.sh test integration` |
| **regression (bash)** | `regression` | Existing `scripts/commands/config_secret_rejection_test.sh` ([line 2](../../scripts/commands/config_secret_rejection_test.sh)) — extend OR add `tests/config/postgres_dev_default_env_override_test.sh` | Spec 051 FR-051-005 still fires when operator runs `POSTGRES_PASSWORD=smackerel ./smackerel.sh config generate --env home-lab --bundle` (env override of dev-default literal). Asserts non-zero exit AND error message names `infrastructure.postgres.password` AND error message does NOT contain the literal `smackerel` (FR-051-007 redaction). | `./smackerel.sh test unit` |

**Adversarial coverage (per Test Integrity Skill):**
- **A1** (drift detector) catches Go-mirror missing entries.
- **A2** (leakage detector) catches loader-side placeholder bypass.
- **A3** (determinism detector) catches non-deterministic placeholder regressions (e.g., timestamped or sourceSha-suffixed).
- **A4** (regression detector) catches accidental `production_class_targets` removal.
- **Bash regression** catches removal/weakening of FR-051-005 dev-default rejection (the spec 051 contract that spec 052 explicitly preserves).

Every Test Plan row maps 1:1 to a DoD checkbox in scopes.md (created by `/bubbles.plan`). Every Gherkin scenario in spec.md (BS-052-001 through BS-052-008) maps to at least one row above.

---

## Documentation Impact

| Doc | Change |
|-----|--------|
| [specs/051-postgres-password-managed-secret/design.md](../051-postgres-password-managed-secret/design.md) | Append a footnote near the FR-051-005 section: "See spec 052 for production-class placeholder mode; FR-051-005 still fires for literal env-override paths per BS-052-006." No FR change. |
| [README.md](../../README.md) | Update the "Configuration" / "Secrets" section (or equivalent) to describe placeholder discipline + adapter substitution + how to add a new managed secret. |
| [docs/Deployment.md](../../docs/Deployment.md) | Add a "Bundle Secret Injection" subsection under Build-Once Deploy-Many: explain the L1/L2/L3 layering, the two-`--env-file` Compose semantics, and where the manifest lives. |
| [docs/Architecture.md](../../docs/Architecture.md) | Add a "Secret Boundary" diagram or paragraph describing the trust boundary between CI, knb adapter, and runtime. |
| [docs/Operations.md](../../docs/Operations.md) | Add an "Auditor Inspection" workflow: how to pull a published bundle, verify cosign sig, extract `app.env`, read `secret-keys.yaml`, and confirm zero literal secrets. Also document the operator's secret rotation procedure (UC-052-004). |
| [specs/047-ci-image-vulnerability-gate/report.md](../047-ci-image-vulnerability-gate/report.md) Surfaced Findings § F-047-B | At spec 052 finalize phase, append: "**RESOLVED** by spec 052 close-out commit `<sha>` — CI matrix `build-bundles (home-lab)` now succeeds via SST placeholder + adapter substitution contract. See `specs/052-bundle-secret-injection-contract/`." Close-out SHA filled at finalize. |

The knb-side `apply.sh` and `knb/smackerel/home-lab/README.md` updates are tracked in the separate knb spec (see "Required knb follow-up" below) and are out of scope for spec 052's documentation impact list.

---

## Convergence Definition

Lifted verbatim from spec.md Outcome Contract Success Signal, plus enforceable mechanical assertions:

1. **All scopes Done with ≥10 lines raw evidence per DoD item** ([scopes.md](scopes.md), per-DoD per-scope).
2. **Contract test PASSES.** `go test -v -count=1 -run BundleSecretContract ./internal/deploy/` exits 0 with all 4 adversarial sub-tests (A1, A2, A3, A4) reporting PASS in the test output.
3. **Bash unit test PASSES.** `tests/config/placeholder_emit_test.sh` exits 0.
4. **`./smackerel.sh test unit` PASSES with zero regressions** — no previously-green test fails.
5. **Local home-lab placeholder bundle is deterministic + zero literal secret values.** Two invocations of `./smackerel.sh config generate --env home-lab --bundle --output-dir <tmp>` produce identical sha256. `tar xzf <bundle> ./app.env -O | grep -E '^(POSTGRES_PASSWORD|AUTH_SIGNING_ACTIVE_PRIVATE_KEY|AUTH_AT_REST_HASHING_KEY|AUTH_BOOTSTRAP_TOKEN)=__SECRET_PLACEHOLDER__'` returns exactly 4 lines. `tar xzf <bundle> ./app.env -O | grep -E '^POSTGRES_PASSWORD=smackerel$'` returns 0 lines.
6. **CI `build-bundles` matrix GREEN** for `dev` + `test` + `home-lab` on the new HEAD SHA. `publish-build-manifest` writes a `build-manifest-<sha>.yaml` containing all 3 `configBundles[*]` entries with valid sha256 digests.
7. **F-047-B marked RESOLVED** in [specs/047-ci-image-vulnerability-gate/report.md](../047-ci-image-vulnerability-gate/report.md) Surfaced Findings with the spec 052 close-out commit SHA inline.

If any of (1)-(7) is FALSE, status remains `in_progress` or `blocked`. Spec 052 cannot reach `done` until ALL 7 pass with raw evidence in `report.md`.

---

## Required Knb Follow-Up

**Flag: YES.**

Spec 052 ships smackerel-side changes only. The knb-side adapter changes are tracked in a separate spec/PR within the [knb repo](../../../knb/) and are a hard cross-repo dependency for go-live (CI green from spec 052 + adapter substitution from knb spec must both ship before the next home-lab apply).

**Expected changes to [knb/smackerel/home-lab/apply.sh](../../../knb/smackerel/home-lab/apply.sh):**

- **A.** Parse `secret-keys.yaml` from extracted bundle into `BUNDLE_SECRET_KEYS` array. Fail loud if file missing.
- **B.** Validate every key in `BUNDLE_SECRET_KEYS` exists in extracted `app.env` AS literal `__SECRET_PLACEHOLDER__<KEY>__`. Fail loud (KEY name only) on mismatch.
- **C.** After sops decrypt: validate every declared key has non-empty + non-placeholder value in decrypted tmpfile.
- **D.** After existing `docker compose ... up`: run `docker compose ... config` and grep stdout for `__SECRET_PLACEHOLDER__`. Zero matches required.
- **E.** Audit log line ([apply.sh:155](../../../knb/smackerel/home-lab/apply.sh)) gains `secrets_substituted=<count>` and `placeholders_remaining=<n>`.

The two-`--env-file` `docker compose up` invocation at [apply.sh:660-668](../../../knb/smackerel/home-lab/apply.sh) is unchanged. The Compose "later wins" override is the canonical substitution mechanism — spec 052 only adds validation around it.

The knb-side `preconditions.sh`, `bootstrap.sh`, `verify.sh`, `rollback.sh`, and `teardown.sh` do NOT need changes for spec 052. The encrypted secrets file `knb/smackerel/secrets/home-lab.enc.env` MUST contain real values for all 4 declared keys before the knb adapter PR ships (operator pre-flight responsibility).

---

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|-------------|
| Drift between 3 manifest mirrors (yaml, Go, shell) | Medium (humans forget mirrors) | High (silent secret leakage OR loader crash) | Contract test adversarial A1 catches drift on every CI run. Spec 052 lands the test in same change set as the manifest, NOT later. |
| Placeholder format collides with a real secret value | Negligible (format is `__SECRET_PLACEHOLDER__<KEY>__` — extremely unlikely real value) | High (would boot with placeholder as credential) | (a) Format uniqueness — no realistic password contains this exact prefix+suffix shape. (b) Runtime adversarial check: `IsPlaceholder()` refuses any value matching the format AT runtime, so even a hypothetical attacker who set a real secret to that exact string would fail-loud at L3. |
| Operator forgets to add a new secret to `home-lab.enc.env` | Medium (especially for new keys via IP-052-004 backfill) | Medium (apply.sh fails BEFORE container start, no broken deploy) | Knb adapter step C (per "Required Knb Follow-Up" above) validates every declared key has a real value in the decrypted tmpfile. Fails loud with KEY name (no value echo per FR-051-007). Operator fixes the gap and retries. |
| Future production target added without manifest update | Low (explicit-opt-in design forces awareness) | High (target would inherit dev-mode literal-value behavior, leaking secrets in bundle) | Opt-in via `infrastructure.production_class_targets` is required AND visible in PR diff. Add a CI lint OR a separate spec for any new production target that mechanically enforces the manifest update. (Out of scope for spec 052 — first new target is the natural enforcement event.) |
| Placeholder substitution silently fails (Compose override misconfigured) | Low | High (placeholder reaches runtime as credential) | 3-layer defense: L2 step D scans resolved Compose env for placeholder markers; L3 `Validate()` refuses to start if any env var equals a placeholder. Either layer alone catches the regression; both layers in series make it impossible to ship. |
| Spec 051 FR-051-005 inadvertently weakened by reorder | Low | High (spec 051 contract regression) | BS-052-006 regression test asserts the dev-default check still fires when `POSTGRES_PASSWORD=smackerel` is provided via env-override on a non-CI invocation. Shipped as part of spec 052 Test Plan. |
| Bundle determinism breaks via accidental nonce | Low (decision OQ-052-02 is explicit) | High (G074 invariant violation) | Contract test adversarial A3 hashes two bundles back-to-back and asserts equality. Catches any reintroduction of timestamps, randoms, or sourceSha-suffixed placeholders. |
