# Report: [BUG-002] No TLS / Reverse Proxy Documentation

**Bug ID:** BUG-020-002
**Feature:** 020-security-hardening
**Created:** 2026-04-19

---

## Summary

| Scope | Name | Status | Evidence |
|-------|------|--------|----------|
| 1 | Create deployment guide + update references | Done | Commit 43e93cf + smackerel.md reference added |

## Completion Statement

`docs/Deployment.md` created with Caddy and nginx reverse proxy examples, Telegram webhook HTTPS documentation, OAuth redirect URI HTTPS documentation, Docker Compose production overrides, and port exposure summary. `README.md` links to deployment guide. `docs/smackerel.md` references deployment guide at two locations. All examples use `smackerel.example.com` placeholder domain.

## Test Evidence

Documentation-only bug — manual review confirms all 6 Gherkin scenarios satisfied:
- SCN-BUG002-01: Caddy section with Caddyfile and security headers ✓
- SCN-BUG002-02: nginx section with proxy_pass and certbot ✓
- SCN-BUG002-03: Telegram webhook HTTPS section with polling/webhook trade-off ✓
- SCN-BUG002-04: OAuth redirect URI HTTPS section with Google Cloud Console steps ✓
- SCN-BUG002-05: `docs/smackerel.md` references `Deployment.md` at lines 2000 and 2254 ✓
- SCN-BUG002-06: `README.md` links to `docs/Deployment.md` at line 127 ✓
