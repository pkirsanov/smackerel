# Scopes: [BUG-002] No TLS / Reverse Proxy Documentation

## Execution Outline

### Phase Order
1. **Scope 1 — Create deployment guide + update references:** Write `docs/Deployment.md` with Caddy and nginx reverse proxy examples, Telegram webhook config, OAuth redirect URI config. Update `docs/smackerel.md` and `README.md` with cross-references.

### New Types & Signatures
None — documentation only.

### Validation Checkpoints
- After Scope 1: `docs/Deployment.md` exists with Caddy, nginx, Telegram, and OAuth sections. `docs/smackerel.md` references the deployment guide. `README.md` links to it.

---

## Scope Summary

| # | Name | Surfaces | Key Tests | Status |
|---|------|----------|-----------|--------|
| 1 | Create deployment guide + update references | `docs/Deployment.md` (new), `docs/smackerel.md`, `README.md` | Manual review: all sections present, examples syntactically valid | [x] Done |

---

## Scope 1: Create deployment guide + update references
**Status:** [x] Done

### Gherkin Scenarios

```gherkin
Feature: TLS reverse proxy deployment documentation

  Scenario: SCN-BUG002-01 — Caddy reverse proxy example exists
    Given docs/Deployment.md exists
    When the "Caddy" section is reviewed
    Then it contains a Caddyfile with reverse_proxy to smackerel-core
    And it contains a Docker Compose service snippet for Caddy

  Scenario: SCN-BUG002-02 — nginx reverse proxy example exists
    Given docs/Deployment.md exists
    When the "nginx" section is reviewed
    Then it contains an nginx.conf with proxy_pass to smackerel-core
    And it contains a Docker Compose service snippet for nginx

  Scenario: SCN-BUG002-03 — Telegram webhook HTTPS documented
    Given docs/Deployment.md exists
    When the "Telegram" section is reviewed
    Then it explains that Telegram webhooks require HTTPS
    And it documents the webhook URL configuration
    And it notes polling mode as an alternative

  Scenario: SCN-BUG002-04 — OAuth redirect URI HTTPS documented
    Given docs/Deployment.md exists
    When the "OAuth" section is reviewed
    Then it explains that OAuth providers require HTTPS redirect URIs
    And it documents how to configure callback URLs

  Scenario: SCN-BUG002-05 — Architecture doc references deployment guide
    Given docs/smackerel.md is read
    When the deployment-related content is reviewed
    Then it references docs/Deployment.md

  Scenario: SCN-BUG002-06 — README links to deployment guide
    Given README.md is read
    When the documentation links are reviewed
    Then it includes a link to docs/Deployment.md
```

### Implementation Plan

| File | Change |
|------|--------|
| `docs/Deployment.md` | New file: reverse proxy setup (Caddy + nginx), Telegram webhook HTTPS, OAuth redirect URIs, Docker Compose integration examples |
| `docs/smackerel.md` | Add "Deployment" subsection referencing `docs/Deployment.md`, document Telegram polling vs. webhook trade-off |
| `README.md` | Add "Production Deployment" link to `docs/Deployment.md` in documentation section |

### Test Plan

| Type | File | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| Manual | `docs/Deployment.md` | Verify Caddy section present | SCN-BUG002-01 |
| Manual | `docs/Deployment.md` | Verify nginx section present | SCN-BUG002-02 |
| Manual | `docs/Deployment.md` | Verify Telegram section present | SCN-BUG002-03 |
| Manual | `docs/Deployment.md` | Verify OAuth section present | SCN-BUG002-04 |
| Manual | `docs/smackerel.md` | Verify cross-reference | SCN-BUG002-05 |
| Manual | `README.md` | Verify link | SCN-BUG002-06 |

### Definition of Done

- [x] `docs/Deployment.md` exists with Caddy reverse proxy example (Caddyfile + Compose snippet)
- [x] `docs/Deployment.md` contains nginx reverse proxy example (nginx.conf + Compose snippet)
- [x] `docs/Deployment.md` covers Telegram webhook HTTPS configuration
- [x] `docs/Deployment.md` covers OAuth redirect URI HTTPS configuration
- [x] `docs/Deployment.md` includes Docker Compose integration guidance
- [x] `docs/smackerel.md` references the deployment guide
- [x] `README.md` links to the deployment guide
- [x] All config examples use placeholder domain `smackerel.example.com`
- [x] No hardcoded ports — examples reference `config/smackerel.yaml` port values
