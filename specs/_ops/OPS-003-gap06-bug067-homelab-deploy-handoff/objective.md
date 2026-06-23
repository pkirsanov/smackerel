# Ops Packet: [OPS-003] Home-Lab Deploy Handoff — GAP-06 + BUG-067-001

> **Owner:** `bubbles.devops`
> **Kind:** Deployment handoff (operator activation packet)
> **Target:** `home-lab`
> **Deploy source SHA:** `78b293cc` (current `main` HEAD; CI builds this tip)
> **Status:** `delivered_pending_activation` — runtime work is committed; the
> live home-lab apply is deferred to the operator and intentionally NOT run
> from this session (host is saturated with concurrent heavy builds).

---

## Summary

This packet hands off **two runtime-relevant changes** committed this session
(6 commits ahead of `origin/main`, not yet pushed) for deployment to the
`home-lab` target via the Build-Once Deploy-Many pipeline (Bubbles gate
**G081 / G074**).

| # | Change | Type | Observability after deploy |
|---|--------|------|----------------------------|
| 1 | **GAP-06** — spec 054 Scope 9: notification-intelligence decision engine now routes user-facing decisions through the shared spec-078 surfacing budget controller | Runtime behavior change | `smackerel_surfacing_*{producer="notification"}` series appears on the core `/metrics` after a notification flows |
| 2 | **BUG-067-001** — ML sidecar NO-DEFAULTS fix: `ML_LOG_LEVEL` is now a required fail-loud SST value | **DEPLOYMENT-CRITICAL config change** | ML sidecar refuses to start if `ML_LOG_LEVEL` is absent from the bundle / `app.env` |

> ⚠️ **The single most important pre-deploy fact:** the home-lab config
> bundle / `app.env` MUST provide `ML_LOG_LEVEL` (recommended value `info`).
> If it is missing, the ML sidecar **fails loud at startup**. See
> [PRE-DEPLOY REQUIRED CONFIG](runbook.md#pre-deploy-required-config) in the
> runbook. The new SST key is **already in the bundle generation path** for
> SHA `78b293cc` — the only failure mode is reusing a **stale** bundle built
> before this SHA (see the caveat in the runbook).

### Neither change needs a flag or a migration

- **No new feature flag** — GAP-06 is a cohesion fix that shares one already-wired
  `surfacing.Controller` + `InMemoryAck` across the scheduler and notification
  producers; the arbitration verdict is persisted on the existing
  `risk_assessment` JSONB column. **No schema migration.**
- **No new feature flag, no schema migration** for BUG-067-001 — it only adds a
  required SST key (`services.ml.log_level`) that already ships with a default
  value of `info` in `config/smackerel.yaml`.

---

## The 6 commits (oldest → newest)

| SHA | Subject | Deploy-relevant? |
|-----|---------|------------------|
| `b41b34f4` | chore(bubbles): sync framework (weighted runtime-lease admission + redteam Green Bastard) | No — framework assets only |
| `fb84f8e2` | docs(specs): reconcile stale spec annotations to committed reality (7 specs) | No — doc hygiene only |
| `cf490e55` | feat(notification): spec 054 Scope 9 — budget-govern event-driven notifications via spec-078 surfacing controller (closes GAP-06) | **Yes — GAP-06 runtime behavior** |
| `37e058ea` | chore(spec-054): promote to done — G088 cleared after planning-truth commit | No — state.json promotion only |
| `8595e3a6` | docs(architecture): document spec 054 Scope 9 notification surfacing-controller integration (GAP-06) | No — architecture doc note |
| `78b293cc` | fix(ml): BUG-067-001 `ML_LOG_LEVEL` fail-loud SST + portfolio reconciliation | **Yes — DEPLOYMENT-CRITICAL config key** |

**Deploy source SHA = `78b293cc`** (the HEAD tip; CI builds + signs `core` and
`ml` images and generates the per-env config bundle from this SHA).

---

## Scope & Success Conditions

This packet is **complete for its mode** (`adapter-readiness-to-packet` →
`delivered_pending_activation`) when:

1. The deploy steps, pre-deploy config requirement, verification, and rollback
   are documented unambiguously for the operator (see `runbook.md`).
2. The `ML_LOG_LEVEL` requirement and its bundle-staleness caveat are called
   out prominently.
3. The packet is handed off; the live home-lab activation is the operator's
   call and is **out of scope** for this session.

**Out of scope / explicitly deferred to the operator:**

- Running the push (`git push`), CI build, `promote.sh`, `apply`, or `verify`
  against the real home-lab host. The host is currently saturated with
  concurrent heavy builds; activation waits for quiescence.
- Any edit to the operator-private home-lab deploy adapter (lives in the `knb`
  overlay, resolved via `DEPLOY_TARGETS_ROOT`); this repo only provides the
  generic, target-agnostic contract.

---

## Ownership Boundary (why this packet stays generic)

The home-lab adapter, real hostnames/IPs, tailnet identity, Caddy site files,
`ufw` rules, systemd unit names, and secret values are **operator-private**
and live in the deploy-adapter overlay — never in this repo (see
`.github/copilot-instructions.md` → "Deployment Ownership Boundary"). This
packet describes only the **generic product-side flow** plus the two shipped
changes; every target-specific value below is a placeholder the adapter fills.

See [`runbook.md`](runbook.md) for the step-by-step deploy, verification, and
rollback procedure.
