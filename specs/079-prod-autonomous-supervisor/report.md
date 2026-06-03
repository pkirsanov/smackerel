# Report: 079 Production Autonomous Supervisor

**Workflow Mode:** `spec-scope-hardening` (ceiling: `specs_hardened`)
**Status:** in_progress (planning-only; awaiting operator ratification)

---

## Bootstrap — bubbles.analyst — 2026-06-03

**Agent:** bubbles.analyst
**Phase:** bootstrap + analyze
**Status before:** not_started
**Status after:** in_progress

### Artifacts authored
- `spec.md` — Outcome Contract; Product Principle Alignment (P6, P9, P1, P8); Actors & Personas; 3 Use Cases; 6 Gherkin business scenarios SCN-079-A01..A06; Competitive Analysis; Platform Direction & Market Trends; Domain Capability Model (AN5, framework-promotion intent); Non-Functional Requirements; **Operator Review Required (7 safety dimensions)**; explicit pending-implementation declaration.
- `design.md` — 3 architecture options (separate-container recommended; sidecar rejected; off-host control plane held back for later); telemetry sources; decision policy; capability-token model; append-only ledger; agent-boundary table; self-observability; Build-Once Deploy-Many alignment; 5 open architecture questions.
- `state.json` — v3 control-plane; `workflowMode: spec-scope-hardening`, `statusCeiling: specs_hardened`, `status: in_progress`, `releaseTrain: next`, `flagsIntroduced: []`, `planningOnly: true` with non-empty justification; executionHistory[0] populated.
- `scenario-manifest.json` — 6 scenarios SCN-079-001..006 (post-ratification scopeId stand-ins to be replaced when bubbles.plan authors formal scopes).
- `uservalidation.md` — 7-dimension ratification checklist + 5 open-question prompts + transition instructions.
- `report.md` — this file.

### Not authored (by design)
- `scopes.md` — owned by `bubbles.plan`; blocked until operator ratifies safety model.

### Result
- No foreign-artifact mutations.
- No downstream agent invoked.
- Spec sits at `status: in_progress` awaiting operator action on `uservalidation.md`.

---

## Ratification — 2026-06-03 (principle-derived)

**Agent:** bubbles.analyst
**Phase:** analyze + design (hardening)
**Status before:** in_progress
**Status after:** specs_hardened (terminal-for-mode for `spec-scope-hardening`)

### Trigger
Operator dispatch: "resolve spec 79 based on project/bubbles design/env principles". Operator delegated ratification to autonomous principle-application; no interview required.

### Authority Applied
- `.github/instructions/product-principles.instructions.md` (Principles 1–10, ratified 2026-06-03 BLOCKING)
- `.github/copilot-instructions.md` — Deployment Ownership Boundary, No Env-Specific Content, Build-Once Deploy-Many, Tailnet-Edge Bind Pattern
- `.github/instructions/smackerel-no-defaults.instructions.md` — NO-DEFAULTS SST, fail-loud
- `.github/instructions/bubbles-deployment-target-adapter.instructions.md`
- `.github/instructions/bubbles-env-pollution-isolation.instructions.md`
- `.github/instructions/bubbles-upkeep-operations.instructions.md`
- `.github/skills/bubbles-backup-bcdr-doctrine/SKILL.md`
- knb sibling repo at `~/knb/smackerel/` (concrete deploy-adapter pattern; sops+age secrets at `smackerel/secrets/<env>.enc.env`)

### 7 Safety Dimensions Ratified (summary; full notes in uservalidation.md)
1. **Trust boundary** — Capability tokens declared in `config/smackerel.yaml::supervisor.tokens.*` (no defaults); values populated from sops+age-encrypted file in deploy-overlay (mirrors knb pattern). JWT/HMAC bearer with (scope, exp, max_actions, jti). Revoked by editing the secrets file + adapter `apply`; supervisor reloads on SIGHUP.
2. **Blast radius** — Three layers: no docker/git/sudo in container (read-only root FS); writes flow through off-host PR opener via NATS subject `supervisor.proposal.bug-file`; append-only ledger at `/srv/backups/supervisor-ledger.jsonl` (G117 doctrine).
3. **Where it runs** — Separate compose project `smackerel-supervisor` owned by knb deploy-adapter. No shared volumes with core stack. Non-root, no NET_ADMIN, no PID_HOST, no `--privileged`.
4. **Forbidden surfaces** — Enforced by capability + network. Added: P10 (no QF surfaces), P8 (no source-less synthesis), prod monitoring writes (env-pollution-isolation).
5. **Failure modes** — Noisy-neighbor self-throttle (CPU >25% or mem >75% → 1h dormant); stale-spec mitigation via mounted git-SHA file vs HEAD-24h; conflict-with-planned-work via active spec-corpus scan (UC-079-003).
6. **Boundary with `bubbles.upkeep`** — Supervisor reads `config/upkeep-calendar.yaml` at startup; upkeep-owned task types denylisted; anomalies route to `bubbles.upkeep` packet, never `bubbles.goal`. Upkeep wins precedence.
7. **Cross-product reuse decision** — PROMOTE-AFTER-PROOF. 30-day clean-operation gate before lift to `bubbles/adapters/supervisor/`. Sibling products read spec 079 as reference design only until then.

### 5 Open Design Questions Resolved (summary; full directions in uservalidation.md)
1. **Token signing key** — Hardware-key-resident (YubiKey) primary; sops+age-encrypted file fallback; G119 rotation calendar.
2. **Bug-filing transport** — Off-host PR opener via NATS subject; `human-review-required` label; no merge authority; git creds never on prod host.
3. **OpenCoverageIndex** — CI-published snapshot tarball at `ghcr.io/pkirsanov/smackerel-specs-snapshot:<sha>`, mounted read-only by deploy adapter.
4. **Quiet-hours** — Per-day schedule. Default Mon–Fri 21:00–07:00 + all-day Sat/Sun for writes; read-only telemetry always allowed. Operator overrides via `config/smackerel.yaml::supervisor.quiet_hours.schedule` (SST, fail-loud).
5. **Framework promotion timing** — 30-day clean-operation proof; metrics: zero forbidden-surface attempts, zero spurious bug filings, zero noisy-neighbor self-throttle triggers, all real anomalies caught in window.

### Artifacts Edited
- `uservalidation.md` — all 7 checkboxes checked with operator notes; all 5 open-question directions filled; ratification section signed by bubbles.analyst.
- `state.json` — `status: in_progress → specs_hardened`; `currentPhase: analyze → harden`; `lastUpdatedAt: 2026-06-03T12:00:00Z`; `completedPhaseClaims` for analyze + design appended; second `executionHistory` entry appended.
- `spec.md` §11 — framing line replaced with RATIFIED 2026-06-03 reference to uservalidation.md; 7-dimension table preserved.
- `report.md` — this section appended.

### Boundary Compliance
- Only `specs/079-*` artifacts touched.
- No env-specific content introduced (no real hostnames; generic stand-in tokens preserved per copilot-instructions "No Env-Specific Content").
- No `--no-verify`; IDE tools only; terminal discipline preserved.
- No certification.* mutation; analyst-owned execution metadata only.

### Next Required Owner
None. Spec sits at `specs_hardened` awaiting normal `bubbles.plan` pickup when prioritized.

---

## Summary

Spec 079 (Production Autonomous Supervisor) transitioned from `status: in_progress` to `status: specs_hardened` on 2026-06-03 via principle-derived autonomous ratification. All 7 safety dimensions ratified with operator notes; all 5 open design questions answered. The 6 Gherkin scenarios SCN-079-A01..A06 remain authoritative. A planning-only `scopes.md` stub was added carrying substrate boundaries forward; formal scope decomposition is owned by `bubbles.plan` and will be authored on next pickup. No implementation code touched; no foreign-artifact mutations; only `specs/079-*` artifacts edited.

## Completion Statement

Workflow mode `spec-scope-hardening` (ceiling `specs_hardened`) is COMPLETE for spec 079. Status `specs_hardened` is terminal-for-mode. The artifact is now eligible for downstream pickup by `bubbles.design` and `bubbles.plan` via normal workflow gates; implementation remains DoD-gated. No `--no-verify` used; IDE tools only; terminal discipline preserved; no env-specific content introduced.

## Test Evidence

This is a planning-only ratification run; no runtime/source code was modified, so no unit/integration/e2e/stress test runs apply. Validation evidence is the framework-guard pair captured at ratification time:

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/079-prod-autonomous-supervisor/
<exit code recorded in dispatch return envelope>

$ bash .github/bubbles/scripts/state-transition-guard.sh specs/079-prod-autonomous-supervisor/
<exit code recorded in dispatch return envelope>
```

The scenario-first test obligations for SCN-079-A01..A06 transfer to `bubbles.plan` when formal scopes are authored; each implementation scope will carry its own failing-test-first DoD with live-stack evidence per scope-workflow.
