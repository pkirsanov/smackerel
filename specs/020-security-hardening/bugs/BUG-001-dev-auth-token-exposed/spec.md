# Bug: Dev Auth Token Exposed as Functional Default

**Bug ID:** BUG-020-001
**Severity:** High
**Found by:** System review (TR-001)
**Feature:** 020-security-hardening

---

## Problem

`config/smackerel.yaml` commits `auth_token: "dev-token-smackerel-2026"` as a functional default value. While the comment says "REQUIRED: set a secure random token", the value itself is a valid 24-character string that passes all existing validation:

1. **Not in placeholder reject list:** `internal/config/config.go` lines 596–603 reject `development-change-me`, `changeme`, `change-me`, `placeholder`, `test-token`, and `default` — but NOT `dev-token-smackerel-2026`.
2. **Passes length check:** The value is 24 characters, exceeding the minimum 16-character requirement at line 609.
3. **Guessable:** The token follows a predictable `dev-token-{project}-{year}` pattern and is committed to the public repository.

A user who deploys Smackerel without changing this value has a fully functional but insecure API. Any person with access to the repository (or who guesses the pattern) can authenticate to every protected surface: API, Web UI, ML sidecar, and NATS.

### Concrete Impact

| Surface | Risk |
|---------|------|
| Core API (`bearerAuthMiddleware`) | Full read/write access to all artifacts, digests, connectors |
| Web UI (`webAuthMiddleware`) | Browse all captured knowledge, settings, connector configs |
| ML sidecar (`verify_auth`) | Invoke LLM, embeddings, vision processing at operator's cost |
| NATS (`nats.conf` authorization) | Inject/read messages on all streams |
| OAuth token store (AES key derived from auth_token) | Decrypt all stored OAuth tokens |

### Root Cause

The 020-security-hardening spec (SEC-021) addressed _empty_ auth tokens with a startup warning and length validation, but did not address the case where the committed default is itself a guessable, functional value. The placeholder reject list was populated with generic strings but missed the project-specific default.

---

## Fix

Two changes required:

1. **Add `dev-token-smackerel-2026` to the placeholder reject list** in `internal/config/config.go` `Validate()`. Also add the pattern-generalizable prefix `dev-token-` as a prefix check to catch future variants like `dev-token-smackerel-2027`.

2. **Change the default in `config/smackerel.yaml`** from `"dev-token-smackerel-2026"` to `""` (empty string). This aligns with the SST secrets management pattern documented in `.github/copilot-instructions.md`: "Empty-string placeholders in `smackerel.yaml` are the intended dev pattern."

The existing startup WARN for empty `auth_token` (from SEC-021) already covers the developer experience — operators see a clear warning that auth is disabled.

## Regression Test Cases

1. `Validate()` with `SMACKEREL_AUTH_TOKEN=dev-token-smackerel-2026` → must return error mentioning "placeholder"
2. `Validate()` with `SMACKEREL_AUTH_TOKEN=dev-token-anything` → must return error (prefix match)
3. `Validate()` with `SMACKEREL_AUTH_TOKEN=DEV-TOKEN-SMACKEREL-2026` → must return error (case-insensitive)
4. `Validate()` with a secure random token ≥16 chars → must still pass (no regression)
5. `Validate()` with empty `SMACKEREL_AUTH_TOKEN` → must pass (dev mode, handled by startup WARN)
6. `config/smackerel.yaml` `auth_token` field → must be empty string, not a functional value
