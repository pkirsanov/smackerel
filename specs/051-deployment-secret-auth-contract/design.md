# Design: Deployment Secret and Auth Contract

## Current Truth

The readiness review found that deployment first start needs a sharper secret and auth gate. The product config should declare required names and fail loudly when values are absent, placeholders, or defaults. Target adapters should provide target-specific values.

## Proposed Design

### Required Values

- `auth.signing.hmac_key`
- `auth.signing.issuer`
- `auth.at_rest_hashing_key`
- `auth.bootstrap_token`
- non-default database password for deployment runtime

### Validation

- Add config validation for missing values.
- Add deployment validation rejecting known local-dev placeholder values.
- Ensure errors name the missing key and never print secret values.

### Documentation

- Document product-required key names.
- Document that target-specific injection belongs to target adapters.
- Update generic deployment docs to list required names without values.

## Test Strategy

| Test ID | Type | Purpose |
|---------|------|---------|
| T-051-001 | unit/config | Missing each auth key fails validation. |
| T-051-002 | unit/config | Default database password is rejected for deployment. |
| T-051-003 | security-static | Startup logs and errors do not include raw secret values. |
| T-051-004 | docs-static | Docs list required names and target adapter injection boundary. |
| T-051-005 | artifact | Artifact lint passes for this feature. |

## Risk Controls

- Keep all secret values out of committed files and evidence.
- Avoid fallback values and optional auth in deployable runtime configuration.
- Keep adapter-specific names, hosts, and paths out of Smackerel artifacts.
