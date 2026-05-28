# Feature: Deployment Secret and Auth Contract

**Status:** Done (certified per state.json)

## Status

In Progress — full-delivery active. Reconciled with the live spec 044 PASETO v4 (Ed25519) implementation contract and the SST + runtime defense-in-depth boundary.

## Review Findings

- V-020: Deployment readiness docs and config do not yet prove required auth keys are provisioned before first start.
- SEC-DEP-003: Default, placeholder, or missing secrets must fail loudly before deployment runtime begins.

## Reconciliation With Spec 044 (R11 Gap Closure)

The original FR-051-001 / FR-051-002 named `auth.signing.hmac_key` and `auth.signing.issuer`. Those names never existed in the live system: spec 044 implements PASETO v4.public (Ed25519) routed by `kid`, not HMAC + `iss`. This spec is reconciled to the live contract; the prior naming is rejected outright. See [report.md](report.md) → "Gaps Probe Results — Round 11" for the full evidence trail.

## Outcome Contract

**Intent:** Require Smackerel deployments to provide all authentication and database secrets before first start, with fail-loud validation at BOTH the SST loader boundary and the runtime startup boundary, and with no default/placeholder secret path.

**Success Signal:** Configuration validation refuses to start when any of the spec 044 production secrets is missing (`auth.signing.active_private_key`, `auth.signing.active_key_id`, `auth.at_rest_hashing_key`, `auth.bootstrap_token`) or when a known local-dev placeholder is used for any of those keys or for the database password. Documentation and readiness checklists show how these values are provisioned without committing secret values, and a docs-static lint asserts the canonical key names appear in [docs/Deployment.md](../../docs/Deployment.md) and [docs/Operations.md](../../docs/Operations.md).

**Hard Constraints:**

- No literal secret values may be committed.
- No fallback/default auth or database secret values are permitted in production.
- Generated config remains generated from SST; generated files are not hand-edited.
- Target-specific secret injection remains a target adapter responsibility.
- Defense-in-depth: every required production secret is enforced at the SST loader (`scripts/commands/config.sh`) AND at runtime startup (`internal/config/config.go::Validate()` plus `internal/auth/startup.go::ValidateRuntimeAuthStartup()`).
- Token format is reconciled with spec 044's live implementation: PASETO v4.public (Ed25519). The earlier "HMAC signing key" / "issuer" terminology in this spec is rejected because it does not match the running system.

**Failure Condition:** Smackerel can start from deployable runtime configuration with missing auth secrets, default/placeholder auth values, the local-dev Postgres password (`smackerel`), or undocumented bootstrap-token semantics. A regression that re-introduces the spec-044-incompatible HMAC/issuer naming or that drops one of the two enforcement layers also fails this contract.

## Requirements

### Authentication contract (PASETO v4 / Ed25519, reconciled with spec 044)

- **FR-051-001:** Config validation MUST require `auth.signing.active_private_key` (PASETO v4 Ed25519 private key, prefixed `k4.secret.`) when `runtime.environment=production` AND `auth.enabled=true`. Empty or missing value MUST fail loud at config load. Reconciles `G-051-IMPL-01` from R11; supersedes the prior `auth.signing.hmac_key` naming which never existed in the live system.
- **FR-051-002:** Config validation MUST require `auth.signing.active_key_id` (operator-chosen short identifier used as PASETO `kid` for routing) when `runtime.environment=production` AND `auth.enabled=true`. Empty or missing value MUST fail loud at config load. Reconciles `G-051-IMPL-02`; supersedes the prior `auth.signing.issuer` naming which would force a token-format change spec 044 explicitly rejected (PASETO routes by `kid`, not by `iss`).
- **FR-051-003:** Config validation MUST require `auth.at_rest_hashing_key` (HMAC-SHA-256 secret used for at-rest token hashing) when `runtime.environment=production` AND `auth.enabled=true`. The value MUST differ from `auth.signing.active_private_key`. Already implemented under spec 044; this requirement formalises the contract and adds spec 051's defense-in-depth runtime check via `ValidateRuntimeAuthStartup`.
- **FR-051-004:** `auth.bootstrap_token` MUST be REQUIRED at config load when `runtime.environment=production` AND `auth.enabled=true`. The value is documented as "consumed exactly once via `./smackerel.sh auth bootstrap` and then cleared by the operator". Spec 051's contract: defense-in-depth treats the bootstrap token as required configuration on every production deployment until the operator explicitly clears it after first-user enrollment. Reconciles `G-051-IMPL-04`.

### Database secret contract (defense-in-depth)

- **FR-051-005:** Deployment validation MUST reject the literal local-dev Postgres password (`smackerel`) and any other documented placeholder values for non-dev SST targets (`TARGET_ENV` ∈ `{home-lab}`) AND at runtime when `runtime.environment=production`. Both layers MUST fail with a clear error that names the offending key without printing the value. Reconciles `G-051-IMPL-05`.

### Documentation and log-redaction

- **FR-051-006:** Documentation MUST describe product-required key names and the target adapter secret injection responsibility without embedding values, and a docs-static lint MUST assert the canonical key names appear in [docs/Deployment.md](../../docs/Deployment.md) and [docs/Operations.md](../../docs/Operations.md). Reconciles `G-051-IMPL-06`.
- **FR-051-007:** A security-static adversarial test MUST prove that no auth or database secret VALUE appears in any error message produced by the SST loader or by the runtime config-validation/auth-startup paths. Error messages MAY name the offending KEY but MUST NOT echo the offending VALUE. Closes `G-051-TST-04`.

## User Scenarios (Gherkin)

```gherkin
Scenario: SCN-051-S01 Missing auth signing key fails before runtime start
  Given runtime.environment=production AND auth.enabled=true
  And deployment configuration omits AUTH_SIGNING_ACTIVE_PRIVATE_KEY
  When config validation runs
  Then validation fails with a clear missing-secret error
  And the error names "AUTH_SIGNING_ACTIVE_PRIVATE_KEY"
  And the Smackerel runtime does not start

Scenario: SCN-051-S02 Default database password is rejected for deployment
  Given TARGET_ENV=home-lab
  And infrastructure.postgres.password is the local-dev placeholder "smackerel"
  When the SST loader (scripts/commands/config.sh) runs
  Then config generation fails before any env file is written
  And the error names "infrastructure.postgres.password" without printing the value
  And the same defense-in-depth check at runtime startup also rejects the placeholder when SMACKEREL_ENV=production

Scenario: SCN-051-S03 Bootstrap token is required and never logged
  Given runtime.environment=production AND auth.enabled=true
  And AUTH_BOOTSTRAP_TOKEN is empty
  When config validation runs
  Then validation fails and names "AUTH_BOOTSTRAP_TOKEN"
  And no error message contains the value of any auth secret or the database password
  When AUTH_BOOTSTRAP_TOKEN is provided by the secret injection path
  Then config validation passes
  And startup error paths never include the raw bootstrap token value
```

## Product Principle Alignment

This spec supports Principle 8 (Trust Through Transparency) by making auth readiness explicit and auditable, and by proving via a security-static adversarial test that secret values never leak into error messages. It also supports Principle 6 (Invisible By Default, Felt Not Heard) because missing or placeholder secrets fail before runtime instead of creating noisy broken services and surprise notifications.

## Non-Goals

- Building a new secret manager.
- Storing target-specific secret values in Smackerel source.
- Implementing target adapter secret injection.
- Changing the spec 044 token format, key rotation contract, or revocation propagation. This spec consumes spec 044's contract; it does not redesign it.
- Introducing a JWKS server, an `iss` claim, or any HMAC-based signing alternative. Spec 044's PASETO v4.public + `kid` routing is the only supported shape.
