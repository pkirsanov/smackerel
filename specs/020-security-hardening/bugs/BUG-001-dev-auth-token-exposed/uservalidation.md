# User Validation: [BUG-001] Dev Auth Token Exposed as Functional Default

**Bug ID:** BUG-020-001
**Feature:** 020-security-hardening
**Created:** 2026-04-19

---

## Checklist

- [x] Bug behavior reproduced before fix — baseline acknowledgement that the committed default `dev-token-smackerel-2026` is currently accepted by `config.Validate()`.

### [Bug Fix] Dev auth token exposed as functional default
- [ ] **What:** Reject the committed default `dev-token-smackerel-2026` and change YAML default to empty string
  - **Steps:**
    1. Set `SMACKEREL_AUTH_TOKEN=dev-token-smackerel-2026` and start the core — must get a clear error
    2. Set `SMACKEREL_AUTH_TOKEN=dev-token-anything-else` — must also get an error
    3. Set `SMACKEREL_AUTH_TOKEN=` (empty) — must start with a WARN (dev mode)
    4. Set `SMACKEREL_AUTH_TOKEN` to a valid random token — must start normally
    5. Verify `config/smackerel.yaml` has `auth_token: ""`
  - **Expected:** Guessable defaults are rejected at startup; empty string triggers WARN but allows dev mode
  - **Verify:** `./smackerel.sh test unit`
  - **Evidence:** report.md#scope-1
