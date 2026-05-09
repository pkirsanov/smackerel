# Spec 044: Per-User Bearer Auth Foundation — Implementation Report

**Status:** in_progress (Scopes not yet executed)

This report is a placeholder for in-progress execution evidence. It will be populated as Scopes 01, 02, 03, 04 are implemented per [`scopes.md`](./scopes.md).

---

## Summary

Spec 044-per-user-bearer-auth was scaffolded to close MIT-040-S-008 (carry-forward from MIT-040-S-003 partial close at commit `4e399a4`), MIT-038-S-003 (cloud-drive Connect body-sourced `owner_user_id`), and the actor-source segment of MIT-027-TRACE-001 (annotation actor_source). The analyst phase authored spec.md (11 scenarios, 21 functional requirements, 8 non-functional requirements, 11 acceptance criteria, 10 design-owned open questions). The design phase authored design.md (13 sections, 14 SST keys under `auth.*` block, 4-phase rollout plan, all 10 OQs resolved). The plan phase authored scopes.md (4 scopes matching the 4 design rollout phases). Implementation has not yet begun. This report file exists to satisfy artifact-lint required-artifact presence and will be populated with execution evidence as scopes are landed.

## Completion Statement

This spec is **NOT yet complete**. Status remains `in_progress` until all 4 scopes are implemented, tested, validated, audited, and certified. The closure will be marked when:

- Scope 01 (SST Foundation + Token Subsystem) lands all 14 SST keys, the `internal/auth/` and `internal/auth/revocation/` packages, the `cmd/core/cmd_auth.go` CLI commands, the `internal/api/auth_handlers.go` admin HTTP endpoints, the DB migrations, and the startup fail-loud validation.
- Scope 02 (Hot-Path Middleware Integration + MIT Closures) refactors `bearerAuthMiddleware`, `MintReveal`, `drive.Connect`, and the annotation pipeline; closes MIT-040-S-008 in spec 040 state.json, MIT-038-S-003 in spec 038 state.json, and the MIT-027-TRACE-001 actor-source segment in spec 027 state.json.
- Scope 03 (Web Surfaces + Telegram Connector) updates PWA, extension, and Telegram connector to send/derive per-user PASETO tokens; admin token-management UI lands.
- Scope 04 (Deprecation Pathway + Documentation Freshness) defaults `auth.production_shared_token_fallback_enabled: false`; updates `docs/Operations.md`, `docs/Deployment.md`, `docs/Development.md`, `docs/smackerel.md`; lands Prometheus metrics emitters; runs regression-baseline-guard.

## Test Evidence

No execution evidence yet. Test files planned for authoring during scope implementation are listed below for trace-guard report-evidence reference. Each test file path will be exercised by `./smackerel.sh test unit`, `./smackerel.sh test integration`, or `./smackerel.sh test e2e` as appropriate when scopes are implemented.

---

## Planned Implementation Order

Per [`design.md`](./design.md) §12 Rollout Plan and [`scopes.md`](./scopes.md):

1. **Scope 01 — SST Foundation + Token Subsystem** — pending (bubbles.implement)
2. **Scope 02 — Hot-Path Middleware Integration + MIT Closures** — pending (bubbles.implement)
3. **Scope 03 — Web Surfaces + Telegram Connector** — pending (bubbles.implement)
4. **Scope 04 — Deprecation Pathway + Documentation Freshness** — pending (bubbles.implement, bubbles.docs)

---

## Planned Evidence References (placeholders for trace-guard)

The following test files will be authored as scopes are implemented:

- `internal/config/validate_test.go` — Scope 1 SST validation tests
- `internal/auth/issue_test.go` — Scope 1 token issuance tests
- `internal/auth/verify_test.go` — Scope 1+2 PASETO verification tests
- `internal/auth/revocation/cache_test.go` — Scope 1+2 revocation cache tests
- `internal/auth/sst_grep_guard_test.go` — Scope 1 SST grep guard
- `internal/api/router_test.go` — Scope 2 middleware tests
- `internal/metrics/auth_metrics_test.go` — Scope 4 Prometheus metrics tests
- `tests/integration/auth_bootstrap_test.go` — Scope 1 bootstrap integration test
- `tests/integration/auth_startup_test.go` — Scope 1 startup fail-loud tests
- `tests/integration/auth_mintreveal_test.go` — Scope 2 MintReveal claim-binding + adversarial regression tests
- `tests/integration/auth_drive_connect_test.go` — Scope 2 drive.Connect claim-binding tests
- `tests/integration/auth_annotation_test.go` — Scope 2 annotation pipeline claim-binding tests
- `tests/integration/auth_rotation_test.go` — Scope 2 rotation grace window tests
- `tests/integration/auth_revocation_test.go` — Scope 2 revocation propagation tests
- `tests/integration/auth_no_body_header_actor_id_test.go` — Scope 2 AC-11 grep guard
- `tests/e2e/auth/pwa_per_user_test.go` — Scope 3 PWA E2E test
- `tests/e2e/auth/extension_per_user_test.go` — Scope 3 extension E2E test
- `tests/e2e/auth/telegram_per_user_test.go` — Scope 3 Telegram bridge E2E test
- `tests/e2e/auth/admin_ui_test.go` — Scope 3 admin UI E2E test

---

## Cross-Spec Closure Plan

This spec's completion will close the following routed backlog items:

- **MIT-040-S-008** (routed in spec 040 commit `4e399a4` carry-forward from MIT-040-S-003 partial close) — fully resolved when Scope 2 lands.
- **MIT-038-S-003** — cloud-drive Connect body-sourced `owner_user_id` resolved when Scope 2 lands.
- **MIT-027-TRACE-001 actor-source segment** — annotation actor_source resolved when Scope 2 lands.
- **VAL-FINDING-040-S-003** — header-trust workaround eliminated in production when Scope 2 lands; AC-11 grep guard provides ongoing enforcement.

---

## References

- [`spec.md`](./spec.md) — feature specification (11 SCN-AUTH-NNN scenarios + 21 FR-AUTH-NNN requirements + 8 NFR-AUTH-NNN + 11 AC + 10 OQ)
- [`design.md`](./design.md) — 13-section design (system context, component diagram, SST plan, lifecycle, hot-path anatomy, failure modes, performance budget, backward compat, security, risks, rollout, OQ resolutions)
- [`scopes.md`](./scopes.md) — 4 scopes per design rollout plan
- [`scenario-manifest.json`](./scenario-manifest.json) — scenario → evidence-ref manifest (planned status)
- `specs/040-cloud-photo-libraries/state.json` — MIT-040-S-008 routing entry (closure target)
- `specs/038-cloud-drives-integration/state.json` — MIT-038-S-003 routing entry (closure target)
- `specs/027-user-annotations/state.json` — MIT-027-TRACE-001 actor-source segment (closure target)
- `.github/skills/bubbles-config-sst/SKILL.md` — SST zero-defaults compliance
- `.github/skills/bubbles-test-environment-isolation/SKILL.md` — test-isolated DB pattern
