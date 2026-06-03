# User Validation: 079 Production Autonomous Supervisor

**Status:** awaiting-operator-ratification
**Gate:** This spec MUST NOT transition `status: in_progress → specs_hardened` until every checkbox below is checked with operator notes. No downstream Bubbles agent (`bubbles.design`, `bubbles.plan`, `bubbles.implement`, `bubbles.workflow`) is permitted to auto-pick spec 079 before that transition.

---

## Safety Dimension Ratification (from spec.md §11)

- [ ] **1. Trust boundary** — Supervisor is read-only by default; writes require operator-issued capability token with scope + TTL + max-actions. *Operator notes (token issuance/revocation workflow + storage location):*

- [ ] **2. Blast radius** — NEVER auto-deploy; proposals only, human merge; bug-filing is append-only and reversible. *Operator notes:*

- [ ] **3. Where it runs** — Separate container/host outside the smackerel-core stack; deploy-adapter-owned, configured via knb's bcdr/upkeep harness. *Operator notes (host/container placement + adapter ownership):*

- [ ] **4. Forbidden surfaces** — Supervisor MUST NOT modify `config/smackerel.yaml`, knb `manifest.yaml`, push to git from prod, take stack down, or exfiltrate user data. *Operator notes (any additions to the forbidden list):*

- [ ] **5. Failure modes** — Noisy-neighbor self-detection (SCN-079-A05), stale spec knowledge mitigation, conflict-with-planned-work mitigation (SCN-079-A04 deference). *Operator notes (acceptable residual risk):*

- [ ] **6. Boundary with `bubbles.upkeep`** — Supervisor is event-driven; upkeep is calendar-driven; they MUST NOT overlap; upkeep wins precedence. *Operator notes (reject if any overlap planned):*

- [ ] **7. Cross-product reuse decision** — Supervisor pattern is a candidate for promotion to bubbles framework foundation (`bubbles.supervisor`); spec 079 is the product-specific instance feeding the foundation. *Operator notes (confirm framework-promotion intent OR reject and re-scope as product-only):*

---

## Open Design Questions (from design.md §9) — Operator Direction Required

1. Token signing key management — operator-laptop-resident vs hardware-key-resident? *Direction:*
2. Bug-filing transport — deploy-adapter-mediated PR opener running OFF the prod host? *Direction:*
3. OpenCoverageIndex — live scan of repo mount vs pre-computed snapshot from CI? *Direction:*
4. Quiet-hours — single window or per-day schedule? *Direction:*
5. Framework promotion timing — when to lift primitives into `bubbles/`? *Direction:*

---

## Transition Instructions

When all 7 safety dimensions are ratified and the 5 open questions have operator direction:
1. Update `state.json.status` from `in_progress` to `specs_hardened`.
2. Update `state.json.execution.completedPhaseClaims` to record the analyst phase closure.
3. At that point, `bubbles.design` may refine `design.md` and `bubbles.plan` may author `scopes.md`. Implementation remains gated by the standard scope-workflow DoD.
