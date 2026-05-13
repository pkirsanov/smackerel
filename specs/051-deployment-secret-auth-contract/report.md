# Report: Deployment Secret and Auth Contract

## Summary

Planning artifacts created for findings V-020 and SEC-HL-003. The feature scopes fail-loud validation for auth signing, issuer, at-rest hashing, bootstrap token, and non-default Postgres password requirements.

## Completion Statement

This feature is not complete. Product-to-planning created the Bubbles packet only. No runtime/source/config/docs implementation edits are claimed.

## Test Evidence

No runtime tests were executed for this planning-only artifact creation. Artifact lint was run separately by the workflow and its result is reported in the workflow result envelope.

## Gaps Probe Results — Round 11 (gaps-to-doc, 2026-05-13)

Stochastic-quality-sweep round 11 ran the `gaps-to-doc` contract against this
spec. The probe compared each declared requirement (FR-051-001..006) against
the live Smackerel codebase, the spec 044 implementation, and the existing
deployment docs. No source/config edits were applied during this probe — every
material gap requires planning reconciliation or specialist work that exceeds
the round's mechanical-fix budget. Concerns are surfaced in the result
envelope; the planning packet remains lint-clean and `Not Started`.

### Probe scope

- Source: `internal/config/config.go::loadAuthConfig`,
  `internal/auth/startup.go`, `internal/config/validate_test.go`,
  `internal/auth/startup_test.go`.
- Config generator: `scripts/commands/config.sh` (auth and Postgres key
  resolution).
- Docs: `docs/Deployment.md` (Per-User Bearer Auth section),
  `docs/Operations.md` (auth.* env-var table).
- Deploy contract: `deploy/contract.yaml`, `deploy/compose.deploy.yml`.

### Implementation gaps vs spec.md requirements

| ID | Requirement | Implementation evidence | Gap classification |
|----|-------------|-------------------------|--------------------|
| G-051-IMPL-01 | FR-051-001 — require `auth.signing.hmac_key` | Live system uses `auth.signing.active_private_key` (PASETO v4 Ed25519) — no HMAC signing key exists. Spec name does not match the spec 044 implementation contract. | Planning reconciliation — spec/design rename or formal exception required. NOT a mechanical edit. |
| G-051-IMPL-02 | FR-051-002 — require `auth.signing.issuer` | No `issuer` field exists in `AuthConfig` or in the env-var contract. Spec 044 routes by `kid`, not `iss`. | Planning reconciliation — design must decide whether to add `issuer` or drop the requirement. |
| G-051-IMPL-03 | FR-051-003 — require `auth.at_rest_hashing_key` | Validated in production at `internal/config/config.go:1061` and at `internal/auth/startup.go:55-58`. Adversarial test exists at `internal/config/validate_test.go:1206`. | Already implemented under spec 044. Scope 1 DoD can credit this with an evidence link once reconciled. |
| G-051-IMPL-04 | FR-051-004 — require `auth.bootstrap_token` | Loaded at `internal/config/config.go:981`, but production-mode `loadAuthConfig` does NOT add it to `authErrors`. Spec 044 treats the value as required at bootstrap-time, not at config-load time. | Planning reconciliation — design must align contract with spec 044's runtime-bootstrap gate. |
| G-051-IMPL-05 | FR-051-005 — reject default Postgres password for deployment | `scripts/commands/config.sh:359` reads `infrastructure.postgres.password` via `required_value`; no allow-list / deny-list rejects the literal local-dev password for production / home-lab bundles. The Go core `Validate()` dev-default check (`config.go:1158-1170`) covers `SMACKEREL_AUTH_TOKEN` only. | Specialist work — needs design decision on where the check lives (config generator vs Go core vs adapter preconditions) plus implementation + adversarial test. |
| G-051-IMPL-06 | FR-051-006 — docs name required keys without values | `docs/Deployment.md` lines 268-281 list `AUTH_SIGNING_ACTIVE_PRIVATE_KEY`, `AUTH_SIGNING_ACTIVE_KEY_ID`, `AUTH_AT_REST_HASHING_KEY`, `AUTH_BOOTSTRAP_TOKEN` against current spec 044 names. No automated docs-static check enforces the list, and the spec/design language still says `hmac_key` / `issuer`. | Planning reconciliation + specialist work — align spec/design language with spec 044, then add docs-static check. |

### Test gaps vs scopes.md test plan

| ID | Planned test | Status | Gap |
|----|--------------|--------|-----|
| G-051-TST-01 | T-051-001 — missing signing key fails validation | Partial — `internal/config/validate_test.go` covers `AUTH_AT_REST_HASHING_KEY`. No equivalent for the spec-named `hmac_key` because the field does not exist. | Resolves once IMPL-01/IMPL-02 reconciliations land. |
| G-051-TST-02 | T-051-002 — missing issuer/at-rest/bootstrap fails | Partial — at-rest covered. Bootstrap and issuer absent. | Specialist work after planning reconciliation. |
| G-051-TST-03 | T-051-003 — default DB password rejected | Missing entirely. | Specialist work — depends on IMPL-05 design decision. |
| G-051-TST-04 | T-051-004 — startup logs do not contain raw secrets | Missing — no security-static test asserts redaction of `AUTH_BOOTSTRAP_TOKEN`, `AUTH_SIGNING_ACTIVE_PRIVATE_KEY`, or `POSTGRES_PASSWORD` in startup output. | Specialist work — needs new security-static test target. |
| G-051-TST-05 | T-051-005 — docs-static for required key names | Missing — no automated check verifies the Deployment.md / Operations.md key tables. | Specialist work — new lint-class test. |
| G-051-TST-06 | T-051-006 — artifact lint passes | Already green this round (see baseline lint output captured by orchestrator). | None. |

### Cross-cutting gaps (observability / rollback / secret rotation)

- **Observability:** No metric counts startup auth-config validation failures.
  Spec 044 emits `smackerel_auth_*` metrics for runtime token activity but not
  for configuration boot-validation outcomes. Useful to wire into the
  observability spec (`specs/030-observability/`); not part of spec 051's contract.
- **Rollback:** Spec 044 Operations.md documents key rotation; no
  contract-specific rollback is missing. No new gap.
- **Secret rotation:** Spec 044 covers signing-key rotation. The bootstrap
  token's "use once and clear" is documented in Operations.md but there is no
  startup-time assertion that fails loud if the bootstrap token remains set
  after first-user enrollment. Recorded as a downstream hardening concern for a
  separate spec extension; tracked in concerns list, not part of this contract.

### Mechanical fixes applied this round

None. All material gaps require planning reconciliation between spec 051's
naming and spec 044's live implementation, or specialist work (deployment
password rejection, security-static log-redaction test, docs-static lint).
This round produced documentation only.

### Round outcome

`done_with_concerns` — gap probe complete, no spec/code state advanced,
concerns recorded for downstream specialist routing. Scope statuses remain
`Not Started`; planning packet remains in `in_progress` / `planning`.

---

## Round 12 (2026-05-13): Planning Reconciliation

### Summary

Spec 044 vs spec 051 contract reconciliation: rewrote `spec.md`, `design.md`,
`scopes.md`, and `state.json` to align with the live PASETO v4 (Ed25519)
contract. Replaced FR-051-001 (`hmac_key` → `active_private_key`),
FR-051-002 (`issuer` → `active_key_id`). Tightened FR-051-005 to defense-in-depth
(SST loader + runtime). Added FR-051-007 (security-static log-redaction).
Created `scenario-manifest.json` covering all three scenarios. Restructured
`scopes.md` into 3 scopes with concrete test files, regression E2E DoD items,
Shared Infrastructure Impact Sweep sections, Change Boundary sections, and
Consumer Impact Sweep (Scope 3).

### Completion Statement

R12 is planning-only. No source code changed. All planning-stage gates
required for implementation entry now resolve to:

- artifact-lint: PASS (EXIT=0)
- traceability-guard: 9 remaining failures, all "missing test file" entries
  for `internal/config/sst_loader_test.go`, `internal/config/log_redaction_test.go`,
  `internal/config/docs_required_keys_test.go` — these are the test files the
  scope plans require Scope 1/2/3 to create during implementation.
- state-transition-guard: planning-stage failures resolved (Check 8A regression
  E2E, Check 8B consumer trace, Check 8C shared-infra, Check 8D change boundary,
  Gate G055 policySnapshot, Gate G056 certification.scopeProgress + lockdownState,
  Gate G068 DoD-Gherkin fidelity all green); remaining failures are expected to
  resolve as scopes execute (DoD checkboxes flip and evidence is added).

### Test Evidence

R12 produced no test runs (planning-only round). Implementation evidence will
be captured per-scope in subsequent rounds.

### Code Diff Evidence

R12 produced no source code changes. The R12 diff is documentation-only:

```
specs/051-deployment-secret-auth-contract/spec.md
specs/051-deployment-secret-auth-contract/design.md
specs/051-deployment-secret-auth-contract/scopes.md
specs/051-deployment-secret-auth-contract/scenario-manifest.json
specs/051-deployment-secret-auth-contract/state.json
specs/051-deployment-secret-auth-contract/report.md
```

Implementation diffs (Round 13 onward) will land here per scope as
`### Code Diff Evidence` sections with `git diff --stat` output and the
relevant patch hunks.

### Round outcome

`planning_reconciliation_complete` — spec/design/scopes/manifest/state aligned
with spec 044; planning gates green; ready for parent-expanded implementation
to begin Scope 1.
