# Scopes: Deployment Secret and Auth Contract

Links: [spec.md](spec.md) | [design.md](design.md)

## Scope 1: Fail-loud auth and database secret validation

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

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
```

### Implementation Plan

1. Add validation for the four required auth keys.
2. Reject default local-dev database passwords in deployable runtime configuration.
3. Ensure validation errors name keys without printing values.
4. Add unit/config tests for every missing-key permutation.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-051-001 | unit/config | config validation tests | SCN-051-S01 | Missing auth.signing.hmac_key fails validation. |
| T-051-002 | unit/config | config validation tests | SCN-051-S01 | Missing issuer, at-rest key, or bootstrap token fails validation. |
| T-051-003 | unit/config | config validation tests | SCN-051-S02 | Default database password is rejected for deployment. |

### Definition of Done

- [ ] T-051-001 passes and proves missing signing key fails before runtime start.
- [ ] T-051-002 passes for issuer, at-rest hashing key, and bootstrap token permutations.
- [ ] T-051-003 passes and proves deployment validation rejects the default database password.

## Scope 2: Secret-safe docs and log redaction proof

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: SCN-051-S03 Bootstrap token is required and not logged
  Given auth.bootstrap_token is provided by the secret injection path
  When Smackerel starts and logs configuration status
  Then startup succeeds
  And logs never include the raw bootstrap token value
```

### Implementation Plan

1. Add log-redaction checks for auth and database secret values.
2. Update docs to list required secret names and target adapter injection boundary.
3. Add docs-static tests for the generic deployment checklist entries.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-051-004 | security-static | startup log tests | SCN-051-S03 | Raw secret values do not appear in logs or errors. |
| T-051-005 | docs-static | deployment docs | SCN-051-S03 | Required key names and injection ownership are documented. |
| T-051-006 | artifact | spec folder | all | Artifact lint passes for this feature. |

### Definition of Done

- [ ] T-051-004 passes and proves raw auth/bootstrap secrets are not logged.
- [ ] T-051-005 passes and docs show required key names plus target adapter injection boundary.
- [ ] T-051-006 passes and this planning packet remains lint-clean.
