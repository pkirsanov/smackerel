# User Validation: BUG-071-001 Canonical metrics endpoint

## Checklist

- [x] The assistant observability E2E uses the same live core endpoint as the rest of the repository harness.
- [x] The test reads real Prometheus exposition and fails when either required metric family is absent.
- [x] The assistant package can be selected without launching every E2E package.
