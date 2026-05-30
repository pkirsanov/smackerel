# Report — Spec 063

## Summary
Implementing username/password login for the smackerel web UI on top
of the existing shared-token cookie mechanism. Adds a credential layer
(table + argon2id hasher + repo), extends the `/v1/web/login` handler
to verify user+pass, exposes a CLI for operator user management, and
updates the login form HTML. Cookie value on success is the existing
shared `AuthToken` — same trust band as today's token-form login.

## Test Evidence
(filled per scope close)
