# Design: Deployment Secret and Auth Contract

## Current Truth

The R11 gaps probe (see [report.md](report.md) → "Gaps Probe Results — Round 11") confirmed that:

1. **Spec 044 already implements PASETO v4.public (Ed25519) signing material**, routed by `kid`. The previously-named `auth.signing.hmac_key` and `auth.signing.issuer` never existed in the live system. Spec 051 reconciles its requirements to the live contract.
2. **Production-mode auth validation is partially implemented** at the SST loader (`internal/config/config.go::loadAuthConfig`) and at runtime (`internal/auth/startup.go::ValidateRuntimeAuthStartup`). The bootstrap-token gate at config-load is missing.
3. **The default Postgres password (`smackerel`)** is currently accepted by the SST loader for ALL `TARGET_ENV` values including `home-lab`, and there is no runtime check that catches it when injected via `DATABASE_URL`. This is the highest-impact deployment-readiness gap.
4. **No security-static log-redaction test** exists. Operators have no automated proof that secret values stay out of error messages.
5. **No docs-static lint** asserts the canonical secret key names appear in [docs/Deployment.md](../../docs/Deployment.md) or [docs/Operations.md](../../docs/Operations.md). Manual inspection shows the names ARE there, but a regression could remove them silently.
6. **`auth.at_rest_hashing_key`** is already validated at both layers under spec 044. Spec 051 credits this and folds it into the docs-static + log-redaction sweep.

## Token Format Decision (single, consistent terminology)

**PASETO v4.public (Ed25519) is the only supported token format.** Every artifact in this feature MUST use the spec 044 names:

| Live name (canonical) | Forbidden alias (rejected) |
|-----------------------|----------------------------|
| `auth.signing.active_private_key` | `auth.signing.hmac_key`, `signing_secret` |
| `auth.signing.active_key_id` (`kid`) | `auth.signing.issuer`, `iss` |
| `auth.at_rest_hashing_key`        | `at_rest_hmac_key`, `hmac_at_rest_key` |
| `auth.bootstrap_token`            | `bootstrap_secret`, `enrollment_token` |

Any future PR that introduces an alias from the right column is a regression of FR-051-001/002 and must be rejected by review.

## Proposed Design

### Required Production Values

When `runtime.environment=production` AND `auth.enabled=true`:

- `AUTH_SIGNING_ACTIVE_PRIVATE_KEY` — non-empty (FR-051-001)
- `AUTH_SIGNING_ACTIVE_KEY_ID` — non-empty (FR-051-002)
- `AUTH_AT_REST_HASHING_KEY` — non-empty AND distinct from the signing key (FR-051-003)
- `AUTH_BOOTSTRAP_TOKEN` — non-empty until operator clears it after first-user enrollment (FR-051-004)

When `TARGET_ENV` ∈ `{home-lab}` (SST boundary) OR `runtime.environment=production` (runtime boundary):

- `infrastructure.postgres.password` (and the resolved `POSTGRES_PASSWORD` env var, and the `DATABASE_URL` password component) MUST NOT equal the local-dev value `smackerel` or any other documented dev-default value (FR-051-005).

> **Footnote (spec 052 cross-reference):** Spec 052 (Bundle Secret Injection
> Contract) introduces a production-class placeholder mode in which the SST
> loader emits the deterministic marker `__SECRET_PLACEHOLDER__POSTGRES_PASSWORD__`
> for `home-lab`/`production` targets and the deploy adapter substitutes the
> real value at apply time. FR-051-005 is **not weakened** — its dev-default
> rejection still fires for any literal env-override path (e.g. an operator
> manually exporting `POSTGRES_PASSWORD=smackerel` before running `apply`).
> The spec 052 placeholder marker is **not** itself a dev-default literal:
> Layer 3 in spec 052 (`internal/config/config.go::Validate()` FR-052-007 loop)
> rejects the placeholder with a distinct error
> (`<KEY> still equals placeholder marker — adapter substitution failed`),
> separate from the FR-051-005 dev-default error path. The two checks are
> orthogonal — both fire under their respective conditions, neither
> supersedes the other (BS-052-006 layered-rejection contract).


### Defense-In-Depth Enforcement Layers

```
                ┌────────────────────────────────────────┐
                │ Operator runs `./smackerel.sh build`   │
                └─────────────────┬──────────────────────┘
                                  │
                ┌─────────────────▼──────────────────────┐
   LAYER 1 →   │ scripts/commands/config.sh             │
   SST loader  │  - resolves config/smackerel.yaml      │
                │  - reject if POSTGRES_PASSWORD=='smackerel'
                │    AND TARGET_ENV != dev|test         │
                │  - reject if any required AUTH_* is   │
                │    empty AND TARGET_ENV=home-lab AND  │
                │    SMACKEREL_ENV=production           │
                └─────────────────┬──────────────────────┘
                                  │ produces config/generated/<env>.env
                ┌─────────────────▼──────────────────────┐
                │ Operator runs `./smackerel.sh up`     │
                └─────────────────┬──────────────────────┘
                                  │
                ┌─────────────────▼──────────────────────┐
   LAYER 2 →   │ internal/config/config.go::Validate()  │
   runtime     │  - reject dev-default DB password if   │
                │    SMACKEREL_ENV==production          │
                └─────────────────┬──────────────────────┘
                                  │
                ┌─────────────────▼──────────────────────┐
   LAYER 2 →   │ internal/config/config.go::            │
   runtime     │   loadAuthConfig() (production block)  │
                │  - reject empty active_private_key     │
                │  - reject empty active_key_id          │
                │  - reject empty at_rest_hashing_key    │
                │  - reject equal hashing/signing keys   │
                │  - reject empty bootstrap_token        │
                │    (NEW under spec 051)                │
                └─────────────────┬──────────────────────┘
                                  │
                ┌─────────────────▼──────────────────────┐
   LAYER 3 →   │ internal/auth/startup.go::             │
   wiring      │   ValidateRuntimeAuthStartup()         │
                │  (already wired, spec 044)            │
                └────────────────────────────────────────┘
```

The "BOTH SST loader AND runtime startup" requirement from FR-051-005 is satisfied by Layer 1 (SST) + Layer 2 (runtime). Both layers MUST be in place — removing either is a regression.

### Postgres Password Dev-Default Detection

Place a single source of truth for the dev-default list in `internal/config/secrets.go` (new file) so the runtime check and the docs-static check share it. The SST shell loader keeps a parallel grep-friendly list as a defense-in-depth duplicate (`scripts/commands/config.sh` block adjacent to the `POSTGRES_PASSWORD="$(required_value ...)"` resolution).

```go
// DevDBPasswords lists known-bad Postgres password values that
// MUST be rejected outside dev/test. Keep in sync with the parallel list
// in scripts/commands/config.sh (defense-in-depth).
var DevDBPasswords = []string{
    "smackerel",        // config/smackerel.yaml dev default
    "postgres",         // upstream image default
    "password",         // docs example smell
    "changeme",         // common default
    "change-me",
    "default",
}
```

### Bootstrap-Token Production-Load Gate

Add to `loadAuthConfig` production-mode block (right after the existing at-rest-hashing-key check):

```go
if cfg.Auth.BootstrapToken == "" {
    authErrors = append(authErrors,
        "AUTH_BOOTSTRAP_TOKEN (REQUIRED in production with auth.enabled=true; "+
            "clear after first-user enrollment via `./smackerel.sh auth bootstrap`)")
}
```

This treats the bootstrap token as required configuration on every production deployment until the operator explicitly clears it. If an operator wants to remove the value after first-user enrollment, they MUST also flip `runtime.environment` away from production OR keep the value injected (the deploy adapter is the source of truth).

### Log-Redaction Adversarial Test

`internal/config/log_redaction_test.go` (new) MUST:

1. Set every secret env var to a sentinel value containing a unique recognizable substring (e.g., `LEAKCANARY-active-priv-XXXX`, `LEAKCANARY-bootstrap-XXXX`, `LEAKCANARY-pgpass-XXXX`).
2. Trigger every error path in `loadAuthConfig`, `Validate`, and `ValidateRuntimeAuthStartup` that touches a secret env var.
3. Assert that NO returned error message and NO captured stderr line contains any sentinel substring.
4. Assert that the same error messages DO contain the offending KEY names so operators can act.

### Docs-Static Lint

`internal/config/docs_required_keys_test.go` (new) MUST:

1. Read [docs/Deployment.md](../../docs/Deployment.md) and [docs/Operations.md](../../docs/Operations.md) from disk.
2. Assert each canonical key name appears at least once in each doc:
   - `AUTH_SIGNING_ACTIVE_PRIVATE_KEY`
   - `AUTH_SIGNING_ACTIVE_KEY_ID`
   - `AUTH_AT_REST_HASHING_KEY`
   - `AUTH_BOOTSTRAP_TOKEN`
   - `POSTGRES_PASSWORD` (or `infrastructure.postgres.password`)
3. Assert each forbidden alias from the table above does NOT appear:
   - `auth.signing.hmac_key`, `auth.signing.issuer`
4. Fail with a clear pointer to spec 051 FR-051-006 if a name is missing.

### SST Loader Test

`scripts/commands/config_secret_rejection_test.sh` (new bash test, called by the unit test driver as a runnable shell sub-process) MUST:

1. Invoke `scripts/commands/config.sh --env home-lab --output-dir <temp>` with a temporary `config/smackerel.yaml` whose Postgres password is `smackerel`.
2. Assert the loader exits non-zero.
3. Assert stderr contains the words `infrastructure.postgres.password` AND `forbidden` AND does NOT contain the word `smackerel` as a free-standing token (the value MUST NOT be echoed).

The home-lab gate uses `TARGET_ENV` (not `SMACKEREL_ENV`) because the SST boundary fires per build target.

## Test Strategy

| Test ID | Type | Location | Purpose |
|---------|------|----------|---------|
| T-051-001 | unit/config | [internal/config/validate_test.go](../../internal/config/validate_test.go) | Missing each AUTH_* fails production validation (covers FR-051-001..004 individually). |
| T-051-002 | unit/config | [internal/config/validate_test.go](../../internal/config/validate_test.go) | `AUTH_BOOTSTRAP_TOKEN` empty in production with `auth.enabled=true` is rejected (NEW — FR-051-004). |
| T-051-003 | unit/config | [internal/config/validate_test.go](../../internal/config/validate_test.go) | `DATABASE_URL` containing the dev-default password is rejected when `SMACKEREL_ENV=production` (NEW — FR-051-005 runtime layer). |
| T-051-004 | security-static | [internal/config/log_redaction_test.go](../../internal/config/log_redaction_test.go) | No secret value appears in any error returned by `loadAuthConfig`, `Validate`, or `ValidateRuntimeAuthStartup` (NEW — FR-051-007). |
| T-051-005 | docs-static | [internal/config/docs_required_keys_test.go](../../internal/config/docs_required_keys_test.go) | `docs/Deployment.md` and `docs/Operations.md` mention every canonical secret key name and no forbidden alias (NEW — FR-051-006). |
| T-051-006 | shell | [scripts/commands/config_secret_rejection_test.sh](../../scripts/commands/config_secret_rejection_test.sh) | SST loader rejects `infrastructure.postgres.password=smackerel` for `TARGET_ENV=home-lab` and does not echo the value (NEW — FR-051-005 SST layer). |
| T-051-007 | artifact | spec folder | Artifact lint, traceability guard, and state-transition guard pass. |

## Risk Controls

- Keep all secret values out of committed files and evidence (handled by gitleaks pre-commit + repo-local PII rules).
- Do not introduce a parallel dev-default list with different content; the Go list and the shell list are explicit duplicates and a downstream hardening spec can introduce code-generation if drift becomes a concern.
- The bootstrap-token always-required gate trades a one-time operator step (clear or re-inject after enrollment) for the much larger payoff of never letting a fresh production deployment start without an enrollment path. Spec 051 accepts this trade-off explicitly.
- Defense-in-depth duplication is intentional: a regression that disables one layer is caught by the other.
- The log-redaction test uses sentinel values with unique substrings so a future contributor accidentally embedding `%s` in an error message is caught immediately.

## Operational Notes

- Documentation updates land in [docs/Deployment.md](../../docs/Deployment.md) "Per-User Bearer Auth" section and in [docs/Operations.md](../../docs/Operations.md) auth env-var table. The docs-static test asserts the canonical names are still present after any doc rewrite.
- A downstream hardening spec can extend the dev-default Postgres list and centralise it via code-gen; spec 051 keeps the list explicit and short.
