# User Validation: 079 Production Autonomous Supervisor

**Status:** ratified-2026-06-03 (principle-derived autonomous ratification)
**Gate:** Ratification complete. State transition `in_progress → specs_hardened` authorized. Downstream Bubbles agents (`bubbles.design`, `bubbles.plan`) may now pick up the spec via normal workflow gates; implementation remains DoD-gated.

---

## Safety Dimension Ratification (from spec.md §11)

- [x] **1. Trust boundary** — Supervisor is read-only by default; writes require operator-issued capability token with scope + TTL + max-actions. *Operator notes (token issuance/revocation workflow + storage location):*
  Capability tokens are SST-managed. A new `supervisor.tokens.*` block is declared in `config/smackerel.yaml` with NO defaults (fail-loud per `smackerel-no-defaults.instructions.md`). Token values are populated only from an operator-issued sops+age-encrypted secrets file in the deploy-overlay repo, mirroring the existing pattern at `knb/smackerel/secrets/<env>.enc.env`. Token format: signed JWT or HMAC bearer carrying `(scope, exp, max_actions, jti)`. Storage: only the supervisor's sops-encrypted secret file holds tokens; never logged, never persisted in plaintext on disk. Revocation: operator edits the secrets file in the deploy-overlay and re-runs the adapter `apply`; supervisor reloads on SIGHUP. Rotation aligned with knb secret-rotation calendar (G119).

- [x] **2. Blast radius** — NEVER auto-deploy; proposals only, human merge; bug-filing is append-only and reversible. *Operator notes:*
  Three layers of containment. (a) The supervisor process has zero `docker` CLI, zero `git` CLI, zero `sudo`; enforced by a minimal base image and read-only root filesystem. (b) All writes targeted at `specs/` flow through a deploy-adapter-mediated PR opener that runs OFF the prod host: the supervisor publishes a proposal JSON to a named NATS subject; an off-host worker (operator laptop or CI runner) consumes the subject, validates the envelope, and opens a PR against the smackerel repo with `human-review-required` label and zero merge authority. (c) An append-only ledger at `/srv/backups/supervisor-ledger.jsonl` (knb-managed tier, mirroring G117 upkeep-ledger doctrine) records every supervisor decision and action; corrections are new appends, never edits.

- [x] **3. Where it runs** — Separate container/host outside the smackerel-core stack; deploy-adapter-owned, configured via knb's bcdr/upkeep harness. *Operator notes (host/container placement + adapter ownership):*
  Separate Docker Compose project `smackerel-supervisor` owned by the deploy-adapter overlay in knb. NOT part of the `smackerel-core` compose project. Mounts: read-only `/var/lib/smackerel/specs-snapshot` populated by CI; read-only Prometheus federation endpoint; write access only to `/srv/backups/supervisor-ledger.jsonl`. No shared volume with `smackerel-core`, `postgres`, `nats`, or the ML sidecar. Runs as non-root UID with no `NET_ADMIN`, no `PID_HOST`, no `--privileged`.

- [x] **4. Forbidden surfaces** — Supervisor MUST NOT modify `config/smackerel.yaml`, knb `manifest.yaml`, push to git from prod, take stack down, or exfiltrate user data. *Operator notes (any additions to the forbidden list):*
  Enforced by capability AND by network: the supervisor's docker network has no route to postgres or nats publishers; it has read-only access to the Prometheus federation endpoint and subscriber-only access on `supervisor.proposal.*` subjects. Additional forbidden surfaces beyond the spec list: smackerel-core config endpoints, knb `<product>/<target>/manifest.yaml`, `git push` from prod, `docker exec` into core/postgres/nats/ml, user-data tables, ML sidecar admin endpoint, the upkeep ledger, prod monitoring writes (per `bubbles-env-pollution-isolation.instructions.md`). Principle-derived additions: Principle 10 (QF Companion Boundary) → MUST NOT propose any fix that touches QF-product surfaces; Principle 8 (Trust Through Transparency) → MUST NOT emit synthesized output without source-link attribution.

- [x] **5. Failure modes** — Noisy-neighbor self-detection (SCN-079-A05), stale spec knowledge mitigation, conflict-with-planned-work mitigation (SCN-079-A04 deference). *Operator notes (acceptable residual risk):*
  Three mitigations. (a) Noisy-neighbor self-throttle: supervisor reads its own resource metrics each cycle; if CPU > 25% of declared budget or memory > 75% of declared budget, it enters dormant mode for 1 hour. (b) Stale-spec-knowledge: supervisor refreshes the `specs-snapshot` mount each cycle by reading the mount's git-SHA file; if SHA is older than HEAD - 24h, it warns in the ledger and reduces fix-dispatch authority to read-only. (c) Conflict-with-planned-work: before any bug filing, supervisor scans the active spec corpus for `in_progress` / `specs_hardened` specs covering the symptom and defers (UC-079-003). Residual risk accepted: the supervisor may miss anomalies during a 1h self-throttle window; the operator accepts this trade as the cost of a non-noisy-neighbor design.

- [x] **6. Boundary with `bubbles.upkeep`** — Supervisor is event-driven; upkeep is calendar-driven; they MUST NOT overlap; upkeep wins precedence. *Operator notes (reject if any overlap planned):*
  Encoded mechanically: the supervisor reads `config/upkeep-calendar.yaml` at startup. Any task type owned by `bubbles.upkeep` (backup, restore-drill, BCDR-drill, patch-cycle, secret-rotation, flag-cleanup, compliance-sweep) is placed in the supervisor's denylist. Anomalies in those domains route to `bubbles.upkeep` via a packet, never directly to `bubbles.goal`. Upkeep wins precedence per `bubbles-upkeep-operations.instructions.md`. Zero overlap planned.

- [x] **7. Cross-product reuse decision** — Supervisor pattern is a candidate for promotion to bubbles framework foundation (`bubbles.supervisor`); spec 079 is the product-specific instance feeding the foundation. *Operator notes (confirm framework-promotion intent OR reject and re-scope as product-only):*
  PROMOTE-AFTER-PROOF. Spec 079 ships product-specific first (smackerel self-hosted only). After 30 consecutive days of clean operation — zero forbidden-surface attempts, zero spurious bug filings, zero noisy-neighbor self-throttle triggers, all real anomalies caught within the declared detection window — the primitives (telemetry adapter, decision policy, ledger writer, capability-token verifier, proposal NATS publisher) lift into `bubbles/adapters/supervisor/` as the `bubbles.supervisor` capability foundation with a `SKILL.md`. Until that proof, sibling products (guesthost / wanderaide / qf) MUST NOT adopt — they may read spec 079 only as a reference design.

---

## Open Design Questions (from design.md §9) — Operator Direction Required

1. **Token signing key management — operator-laptop-resident vs hardware-key-resident?**
   *Direction:* Hardware-key-resident (YubiKey or equivalent) on the operator's laptop for production tokens, with a sops+age-encrypted file in the deploy-overlay as the fallback signing material. Rotation aligned with knb secret-rotation calendar (G119). Justification: principle of least surprise — mirrors the existing knb sops+age pattern; the hardware key adds defense-in-depth without inventing a new key-management surface.

2. **Bug-filing transport — deploy-adapter-mediated PR opener running OFF the prod host?**
   *Direction:* Yes. Supervisor publishes proposal JSON to NATS subject `supervisor.proposal.bug-file`. An off-host worker (operator laptop or CI runner) consumes the subject, validates the proposal envelope, and opens a PR against the smackerel repo with `human-review-required` label and no merge authority. Git credentials never reside on the prod host.

3. **OpenCoverageIndex — live scan of repo mount vs pre-computed snapshot from CI?**
   *Direction:* Pre-computed snapshot from CI. Each successful smackerel CI run on `main` publishes a tarball of `specs/*/state.json` + `specs/*/scenario-manifest.json` + a HEAD-SHA file to `ghcr.io/pkirsanov/smackerel-specs-snapshot:<sha>`. The deploy adapter mounts the latest snapshot read-only into the supervisor container. Avoids prod scanning the repo, avoids drift, gives the staleness mitigation a clean SHA comparison point.

4. **Quiet-hours — single window or per-day schedule?**
   *Direction:* Per-day schedule. Default config: bug-filing and fix-dispatch blocked Mon–Fri 21:00–07:00 local and all-day Sat/Sun; read-only telemetry collection and ledger writing always allowed. Operator overrides via `config/smackerel.yaml::supervisor.quiet_hours.schedule` (SST, fail-loud if absent). Per-day reflects self-hosted operator reality better than a single window.

5. **Framework promotion timing — when to lift primitives into `bubbles/`?**
   *Direction:* 30-day clean-operation proof (see safety dimension 7). Metrics: zero forbidden-surface attempts (audited via supervisor's own ledger sweep), zero spurious bug filings (operator verifies post-hoc), zero noisy-neighbor self-throttle triggers, all real anomalies caught within the declared detection window. After 30 days, `bubbles.framework-sync` proposes the lift to `bubbles/adapters/supervisor/` with the `SKILL.md` authored.

---

## Ratification (principle-derived)

Ratified 2026-06-03 by autonomous principle-application per operator dispatch "resolve spec 79 based on project/bubbles design/env principles". Authority:

- `.github/instructions/product-principles.instructions.md` (Principles 1–10, ratified 2026-06-03 BLOCKING)
- `.github/copilot-instructions.md` — "Deployment Ownership Boundary", "No Env-Specific Content", "Build-Once Deploy-Many", "Tailnet-Edge Bind Pattern"
- `.github/instructions/smackerel-no-defaults.instructions.md` — NO-DEFAULTS SST, fail-loud
- `.github/instructions/bubbles-deployment-target-adapter.instructions.md` — adapters own target-specific knowledge
- `.github/instructions/bubbles-env-pollution-isolation.instructions.md` — never write to prod monitoring/backup/manifest from test code
- `.github/instructions/bubbles-upkeep-operations.instructions.md` — calendar-driven hygiene boundary, upkeep precedence
- `.github/skills/bubbles-backup-bcdr-doctrine/SKILL.md` — `/srv/backups/*` tiers, append-only ledger discipline
- knb sibling repo at `~/knb/smackerel/` — concrete deploy-adapter pattern; secrets at `smackerel/secrets/<env>.enc.env` via sops+age

No operator interview required — operator delegated to principle-application in the current dispatch.

Signed: `bubbles.analyst` — 2026-06-03

---

## Transition Instructions (executed 2026-06-03)

1. `state.json.status` transitioned from `in_progress` to `specs_hardened`.
2. `state.json.execution.completedPhaseClaims` updated to record the analyst phase closure (analyze + design phases ratified by principle-application).
3. `bubbles.design` may refine `design.md` and `bubbles.plan` may author `scopes.md`. Implementation remains gated by the standard scope-workflow DoD.
