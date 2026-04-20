# User Validation Checklist

## Checklist

- [x] Baseline checklist initialized for BUG-006 test auth token provisioning
- [x] Config generator produces non-empty SMACKEREL_AUTH_TOKEN in test.env when SST value is empty
- [x] Generated test token is at least 48 hex characters
- [x] Dev environment dev.env still has empty SMACKEREL_AUTH_TOKEN when SST value is empty
- [x] `./smackerel.sh test integration` runs without auth token crash
- [x] `./smackerel.sh test e2e` compose stack starts successfully
- [x] No hardcoded tokens in any source-controlled file
- [x] Each `./smackerel.sh config generate` produces a different test token

Unchecked items indicate a user-reported regression.
