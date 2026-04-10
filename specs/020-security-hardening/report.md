# Report: 020 Security Hardening — Docker Binding, Auth Enforcement, Crypto Hygiene

**Feature:** 020-security-hardening
**Created:** 2026-04-10

---

## Summary

| Scope | Name | Status | Evidence |
|-------|------|--------|----------|
| 1 | Docker Port Binding + NATS Config File | Not started | — |
| 2 | ML Sidecar Auth + Web UI Auth + OAuth Rate Limiting | Not started | — |
| 3 | Decrypt Fail-Closed + Startup Auth Warning | Not started | — |

## Test Evidence

### Scope 1: Docker Port Binding + NATS Config File

| Test Type | Command | Result | Timestamp |
|-----------|---------|--------|-----------|
| Unit | `./smackerel.sh test unit` | — | — |
| Integration | `./smackerel.sh test integration` | — | — |
| E2E | `./smackerel.sh test e2e` | — | — |
| Config generate | `./smackerel.sh config generate` | — | — |

### Scope 2: ML Sidecar Auth + Web UI Auth + OAuth Rate Limiting

| Test Type | Command | Result | Timestamp |
|-----------|---------|--------|-----------|
| Unit (Go) | `./smackerel.sh test unit` | — | — |
| Unit (Python) | `./smackerel.sh test unit` | — | — |
| Integration | `./smackerel.sh test integration` | — | — |
| E2E | `./smackerel.sh test e2e` | — | — |

### Scope 3: Decrypt Fail-Closed + Startup Auth Warning

| Test Type | Command | Result | Timestamp |
|-----------|---------|--------|-----------|
| Unit (Go) | `./smackerel.sh test unit` | — | — |
| Unit (Python) | `./smackerel.sh test unit` | — | — |
| E2E | `./smackerel.sh test e2e` | — | — |

## Completion Statement

Not yet complete. All scopes pending implementation.
