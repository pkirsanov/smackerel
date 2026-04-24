# User Validation: [BUG-002] No TLS / Reverse Proxy Documentation

**Bug ID:** BUG-020-002
**Feature:** 020-security-hardening
**Created:** 2026-04-19

---

## Checklist

- [x] Bug behavior reproduced before fix — baseline acknowledgement that no `docs/Deployment.md` reverse-proxy/TLS guide existed at the time the bug was filed.

### [Bug Fix] No TLS / reverse proxy documentation
- [ ] **What:** Create deployment guide with reverse proxy TLS setup for Telegram webhooks and OAuth
  - **Steps:**
    1. Verify `docs/Deployment.md` exists
    2. Review Caddy section — must have a complete Caddyfile and Docker Compose snippet
    3. Review nginx section — must have a complete nginx.conf and Docker Compose snippet
    4. Review Telegram section — must explain webhook HTTPS requirement and polling alternative
    5. Review OAuth section — must explain HTTPS redirect URI requirement
    6. Verify `docs/smackerel.md` references the deployment guide
    7. Verify `README.md` links to the deployment guide
  - **Expected:** Operator can follow the guide to set up TLS for Telegram webhooks and OAuth callbacks
  - **Verify:** Manual review of documentation
  - **Evidence:** report.md#scope-1
