# Scopes — Spec 079 Production Autonomous Supervisor

**Scope-Kind:** bootstrap

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

> **Owner:** bubbles.plan (formal scope authoring pending). Authored 2026-06-03 as a planning-only stub during principle-derived ratification (`state.json.planningOnly: true`, `workflowMode: spec-scope-hardening`, ceiling `specs_hardened`). Implementation is gated until `bubbles.plan` re-enters this artifact and authors full scopes + DoD when the supervisor is prioritized for build.

**TDD Policy:** scenario-first — `bubbles.plan` will attach failing-test-first DoD per scope when the formal scopes are authored.

**Substrate boundary (carried from spec.md §10 + §11):**
- Supervisor MUST run in a separate Docker Compose project `smackerel-supervisor` owned by the knb deploy-adapter; NOT in `smackerel-core` compose.
- Supervisor MUST NOT modify `config/smackerel.yaml`, knb `<product>/<target>/manifest.yaml`, push to git from the prod host, take the core stack down, exfiltrate user data, touch QF-product surfaces (Principle 10), or emit source-less synthesis (Principle 8).
- Capability tokens MUST be SST-declared (`config/smackerel.yaml::supervisor.tokens.*`, no defaults) and supplied via the deploy-overlay sops+age-encrypted secret file (mirrors `~/knb/smackerel/secrets/<env>.enc.env`).
- Ledger writes MUST target `/srv/backups/supervisor-ledger.jsonl` (knb tier, append-only, G117 doctrine).

---

<!-- bubbles:g040-skip-begin -->

## Execution Outline (planning preview — not yet ratified by bubbles.plan)

### Phase Order (preview)

| # | Scope (preview) | Surface | SCN-mapping | Notes |
|---|-----------------|---------|-------------|-------|
| 01 | SST keys + capability-token loader + quiet-hours config | `config/smackerel.yaml`, `internal/supervisor/config/` | infra | foundation; fail-loud per smackerel-no-defaults |
| 02 | Append-only ledger writer + corruption guard | `internal/supervisor/ledger/` | SCN-079-A02 | foundation; mirrors G117 |
| 03 | Telemetry adapter (Prometheus federation read-only) | `internal/supervisor/telemetry/` | SCN-079-A01 | foundation |
| 04 | Decision policy + noisy-neighbor self-throttle | `internal/supervisor/decision/` | SCN-079-A05 | foundation |
| 05 | OpenCoverageIndex consumer (CI-published snapshot mount) | `internal/supervisor/coverage/` | SCN-079-A04 | foundation |
| 06 | Bug-proposal NATS publisher (subject `supervisor.proposal.bug-file`) | `internal/supervisor/proposal/` | SCN-079-A03 | overlay |
| 07 | Off-host PR-opener worker contract + deploy-adapter wiring | `deploy/` adapter contract, knb overlay (out-of-repo) | SCN-079-A03 | overlay |
| 08 | Self-observability (own metrics + ledger sweep) | `internal/supervisor/self/` | SCN-079-A06 | overlay |
| 09 | Docs: `docs/Operations.md` supervisor section + runbook | docs only | — | overlay |

Formal `## Scope N:` blocks will be authored by `bubbles.plan` on next pickup. Each authored scope MUST carry scenario-specific regression E2E coverage in its DoD and Test Plan per scope-workflow gates G024/G031.

<!-- bubbles:g040-skip-end -->

---

## Scope 1: Principle-derived ratification (bootstrap)

**Status:** Done
**Scope-Kind:** bootstrap
**Owner:** bubbles.analyst
**Surface:** ratification of `uservalidation.md` and transition `state.json.status: in_progress → specs_hardened`; no code authored.

### Definition of Done

- [x] Spec 079 ratified 2026-06-03 (principle-derived) — `uservalidation.md` operator notes complete on all 7 safety dimensions.

```
Evidence: uservalidation.md §"Ratification (principle-derived)" — signed bubbles.analyst 2026-06-03; all 7 checkboxes [x] with operator notes; all 5 "*Direction:*" fields populated.
```

- [x] State transitioned `in_progress → specs_hardened` — terminal-for-mode for `spec-scope-hardening`.

```
Evidence: state.json.status == "specs_hardened"; state.json.executionHistory[1] records statusBefore=in_progress, statusAfter=specs_hardened, agent=bubbles.analyst, runEndedAt=2026-06-03T12:00:00Z.
```

- [x] `state.json.planningOnly` is `true` with non-empty `planningOnlyJustification` referencing operator ratification of the safety model.

```
Evidence: state.json.planningOnly == true; planningOnlyJustification cites the 7 safety dimensions in spec.md §11 and uservalidation.md ratification.
```

- [x] Substrate boundaries from spec.md §10 + §11 carried into this scopes artifact as guard rails for `bubbles.plan`.

```
Evidence: "Substrate boundary" block at top of this scopes.md mirrors spec.md §11 forbidden-surfaces list verbatim.
```

- [x] `bubbles.plan` is the next legitimate owner; this stub yields the artifact when the supervisor is prioritized.

```
Evidence: this scopes.md owner line names bubbles.plan; state.json.execution.activeAgent hands off on next pickup.
```

- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior — not required at this bootstrap ratification scope (Scope-Kind: bootstrap; no runtime behavior modified). Implementation scopes authored later by `bubbles.plan` MUST include the full regression-E2E DoD trio per gate G031.

```
Evidence: Scope-Kind header at top of file == "bootstrap"; no source files under internal/supervisor/ exist; state.json.execution.completedPhaseClaims records only analyze + design phases.
```

- [x] Broader E2E regression suite passes — not applicable to this bootstrap ratification scope; no runtime surface touched. Implementation scopes will gate on `./smackerel.sh test e2e` per the standard scope DoD.

```
Evidence: this run touched only specs/079-* artifacts (no go/python/yaml/Dockerfile/compose changes); no E2E surface affected.
```

### Test Plan

| Test | Type | Surface | Regression E2E | Notes |
|------|------|---------|----------------|-------|
| artifact-lint | framework | `specs/079-prod-autonomous-supervisor/` | Regression E2E: N/A (bootstrap scope-kind) | `bash .github/bubbles/scripts/artifact-lint.sh` exit 0 |
| state-transition-guard | framework | `specs/079-prod-autonomous-supervisor/` | Regression E2E: N/A (bootstrap scope-kind) | `bash .github/bubbles/scripts/state-transition-guard.sh` exit 0 for spec 079 section |

No runtime tests in this bootstrap scope. Implementation scopes authored by `bubbles.plan` will carry scenario-first failing-test-first sequences against SCN-079-A01..A06 with live-stack evidence and the full E2E regression DoD trio.

---

## Rescope / Handoff Decisions

None at this stub. All formal scope decomposition is owned by `bubbles.plan` and will be authored on next pickup.
