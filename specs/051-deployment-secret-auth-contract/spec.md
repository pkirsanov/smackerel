# Feature: Deployment Secret and Auth Contract

## Status

In Progress - planning packet created

## Review Findings

- V-020: Deployment readiness docs and config do not yet prove required auth keys are provisioned before first start.
- SEC-DEP-003: Default, placeholder, or missing secrets must fail loudly before deployment runtime begins.

## Outcome Contract

**Intent:** Require Smackerel deployments to provide all authentication and storage secrets before first start, with fail-loud validation and no default secret path.

**Success Signal:** Configuration validation refuses to start without `auth.signing.hmac_key`, `auth.signing.issuer`, `auth.at_rest_hashing_key`, `auth.bootstrap_token`, and a non-default Postgres password; documentation and readiness checklists show how these values are provisioned without committing secret values.

**Hard Constraints:**

- No literal secret values may be committed.
- No fallback/default auth key values are allowed.
- Generated config remains generated from SST; generated files are not hand-edited.
- Target-specific secret injection remains a target adapter responsibility.

**Failure Condition:** Smackerel can start from deployable runtime configuration with missing auth keys, default auth values, default Postgres password, or undocumented bootstrap-token behavior.

## Requirements

- **FR-051-001:** Config validation MUST require `auth.signing.hmac_key`.
- **FR-051-002:** Config validation MUST require `auth.signing.issuer`.
- **FR-051-003:** Config validation MUST require `auth.at_rest_hashing_key`.
- **FR-051-004:** Config validation MUST require `auth.bootstrap_token`.
- **FR-051-005:** Deployment validation MUST reject default or placeholder database credentials.
- **FR-051-006:** Documentation MUST describe product-required names and target adapter secret injection responsibility without embedding values.

## User Scenarios (Gherkin)

```gherkin
Scenario: SCN-051-S01 Missing auth signing key fails before runtime start
  Given deployment configuration omits auth.signing.hmac_key
  When config validation runs
  Then validation fails with a clear missing-secret error
  And Smackerel runtime does not start

Scenario: SCN-051-S02 Default database password is rejected for deployment
  Given deployment configuration still uses a local dev database password
  When first-start validation runs
  Then validation fails before services start
  And the operator sees the required secret name without seeing any secret value

Scenario: SCN-051-S03 Bootstrap token is required and not logged
  Given auth.bootstrap_token is provided by the secret injection path
  When Smackerel starts and logs configuration status
  Then startup succeeds
  And logs never include the raw bootstrap token value
```

## Product Principle Alignment

This spec supports Principle 8 by making auth readiness explicit and auditable. It also supports Principle 6 because missing secrets fail before runtime instead of creating noisy broken services.

## Non-Goals

- Building a new secret manager.
- Storing target-specific secret values in Smackerel source.
- Implementing target adapter secret injection.
