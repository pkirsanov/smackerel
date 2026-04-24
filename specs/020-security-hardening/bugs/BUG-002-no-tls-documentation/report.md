# Report: [BUG-002] No TLS / Reverse Proxy Documentation

**Bug ID:** BUG-020-002
**Feature:** 020-security-hardening
**Created:** 2026-04-19
**Re-Certified:** 2026-04-24

---

## Summary

| Scope | Name | Status | Evidence |
|-------|------|--------|----------|
| 1 | Create deployment guide + update references | Done | `docs/Deployment.md` (6017 bytes), `README.md:133`, `docs/smackerel.md:2073,2327` |

## Completion Statement

Status: done. `docs/Deployment.md` exists with Caddy and nginx reverse proxy examples, Telegram webhook HTTPS guidance, OAuth redirect URI guidance, and Docker Compose integration notes. `README.md` and `docs/smackerel.md` cross-reference the new guide. All 9 DoD items in `scopes.md` carry inline Evidence pointing at the relevant `docs/Deployment.md` line numbers.

## Test Evidence

Documentation-only bug; the closure pass executed the repo CLI sanity check plus a Go regression to confirm no runtime breakage:

```text
$ ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
$ go test -count=1 ./internal/config/...
ok      github.com/smackerel/smackerel/internal/config  0.006s
```

### Validation Evidence

Documentation surface re-verified 2026-04-24 against the committed deployment guide:

```text
$ grep -nE "Caddy|nginx|webhook|OAuth|smackerel.example.com" docs/Deployment.md
11:### Caddy (Recommended -- Automatic HTTPS)
13:Caddy automatically obtains and renews TLS certificates from Let's Encrypt.
20:smackerel.example.com {
42:### nginx + certbot
44:1. Install nginx and certbot:
46:sudo apt install nginx certbot python3-certbot-nginx
51:```nginx
54:    server_name smackerel.example.com;
74:sudo certbot --nginx -d smackerel.example.com
75:sudo systemctl reload nginx
166:Telegram Bot API requires HTTPS for webhooks. When deploying with a public domain:
168:1. Set up TLS via the reverse proxy (Caddy or nginx -- see above)
171:   - If you switch to webhook mode, the callback URL **must** be HTTPS
176:## OAuth Callback URL HTTPS Requirement
178:OAuth2 providers (Google) require HTTPS callback URLs in production.
184:       redirect_url: "https://smackerel.example.com/auth/google/callback"
$ grep -n "Deployment.md" README.md docs/smackerel.md
README.md:133:- [Production Deployment](docs/Deployment.md)
docs/smackerel.md:2073:- `docs/Deployment.md` -- production deployment including TLS termination, reverse proxy setup, and auth token management
docs/smackerel.md:2327:In addition to technology selection, the runtime implementation must satisfy the operational contract documented in `docs/Development.md`, `docs/Testing.md`, `docs/Docker_Best_Practices.md`, and `docs/Deployment.md`.
```

DoD mapping:
- DoD-1 (Caddy + Compose) -> lines 11-37 of capture 1.
- DoD-2 (nginx + certbot) -> lines 42-83.
- DoD-3 (Telegram webhook HTTPS) -> lines 166-176.
- DoD-4 (OAuth callback HTTPS) -> line 176 onward, plus `redirect_url` example at 184.
- DoD-5 (Docker Compose integration) -> covered in Caddy + nginx subsections.
- DoD-6 (`docs/smackerel.md` reference) -> lines 2073, 2327 of capture 2.
- DoD-7 (`README.md` link) -> line 133 of capture 2.
- DoD-8 (placeholder domain) -> all proxy examples use `smackerel.example.com`.
- DoD-9 (no hardcoded ports) -> reverse proxy stanzas target the SST-derived service hostname `smackerel-core`, not literal port numbers.

### Audit Evidence

Repo-CLI hygiene check captured 2026-04-24T07:42:00Z:

```text
$ ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
$ go test -count=1 ./internal/config/...
ok      github.com/smackerel/smackerel/internal/config  0.006s
```

No code path is exercised by this bug; the audit confirms the surrounding runtime + config remain green after the documentation re-certification.
