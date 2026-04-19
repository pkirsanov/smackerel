# Report: [BUG-001] Dev Auth Token Exposed as Functional Default

**Bug ID:** BUG-020-001
**Feature:** 020-security-hardening
**Created:** 2026-04-19

---

## Summary

| Scope | Name | Status | Evidence |
|-------|------|--------|----------|
| 1 | Reject default token + change YAML default | Done | Commit 43e93cf, unit tests pass |

## Completion Statement

All DoD items verified. `dev-token-smackerel-2026` added to reject list, `dev-token-*` prefix check added (case-insensitive), `config/smackerel.yaml` auth_token changed to empty string. Regression tests cover literal, prefix, case-insensitive, valid token, and empty token cases.

## Test Evidence

- `./smackerel.sh test unit` — all Go and Python tests pass (214 Python, all Go packages green)
- `TestValidate_AuthTokenDevTokenPrefixRejected` — covers literal default, arbitrary prefix, and mixed-case tokens
- No existing tests broken
