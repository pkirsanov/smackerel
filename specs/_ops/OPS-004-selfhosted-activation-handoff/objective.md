# Ops Packet: [OPS-004] Self-Hosted Activation Handoff (Consolidated)

> **Owner:** `bubbles.devops`
> **Kind:** Deployment handoff (consolidated operator activation packet)
> **Target:** `self-hosted`
> **Deploy source SHA:** the current pushed `origin/main` HEAD at build time —
> the operator MUST resolve the live value via `git rev-parse origin/main` and
> the local build manifest's `<sourceSha>`; do **NOT** hardcode a SHA. It was
> `e0e7bc4f` when this packet was authored (2026-06-24); HEAD has since advanced
> with doc / spec-096-closeout-only commits that change **no runtime payload**
> (current tip `010a5e2f`). The authoring-time SHA `e0e7bc4f` supersedes OPS-003's
> interior payload SHA `78b293cc`.
> **Supersedes:** [`OPS-003`](../OPS-003-gap06-bug067-selfhosted-deploy-handoff/)
> for the live deploy. OPS-003 handed off GAP-06 + BUG-067-001 at `78b293cc`;
> OPS-004 is the **current consolidated activation** for `e0e7bc4f` and folds
> OPS-003's payload in alongside everything else now awaiting a self-hosted deploy.
> **Status:** `delivered_pending_activation` — all runtime work is committed on
> `main`; the live self-hosted apply + GPU verify is the operator's call and is
> intentionally NOT run from this session. No build / deploy / push / CI /
> docker / promote command was executed while authoring this packet.

---

## Summary

This packet is the **single consolidated self-hosted activation handoff** for
deploy source `e0e7bc4f`. It tells the operator exactly what to build, deploy,
and verify on the live `self-hosted` GPU stack to activate the runtime-relevant
changes that have accumulated on `main`, and which currently-`blocked` /
gated bugs and specs the live verify closes.

It is a **documentation-only** ops packet (objective + runbook + state), in the
same shape as [`OPS-003`](../OPS-003-gap06-bug067-selfhosted-deploy-handoff/). The
actual runtime deliveries and their certifications live in their own
feature/bug packets; this packet ships only the deploy / verify / rollback
procedure and the per-bug live pass signals.

> ⚠️ **Two pre-deploy facts dominate everything else** (full detail in
> [`runbook.md` → PRE-DEPLOY REQUIRED CONFIG](runbook.md#pre-deploy-required-config)):
>
> 1. **`ML_LOG_LEVEL` is a required fail-loud SST key** (BUG-067-001). The
>    bundle / `app.env` MUST carry it (recommended `info`) or the ML sidecar
>    refuses to start.
> 2. **The synthesis model selection MUST resolve to `qwen3:30b-a3b`** — the
>    standing self-hosted synthesis default — NOT the retired `deepseek-r1:7b`
>    (nor the interim `gpt-oss:20b` / spec-089's `deepseek-r1:32b`).

---

## Deploy Source & Build Producer (verified)

| Fact | Value |
|------|-------|
| Deploy source SHA | the current pushed `origin/main` HEAD at build time — resolve via `git rev-parse origin/main` + the local build manifest `<sourceSha>` (do NOT hardcode). `e0e7bc4f` at authoring (2026-06-24); tip has since advanced to `010a5e2f` with doc / spec-096-closeout-only commits (no runtime payload change) |
| Supersedes OPS-003 payload SHA | `78b293cc` (interior; subsumed here) |
| GitHub CI workflows (`build.yml` / CI / E2E / client) | **`disabled_manually`** — NOT the artifact producer |
| Artifact producer | In-repo **local-operator** build path: `./smackerel.sh build --target self-hosted` |
| Local build manifest | `dist/local-build-manifests/local-build-manifest-<sourceSha>.yaml` (`trustModel: local-operator`) |
| Promote entrypoint | `bash scripts/deploy/promote.sh --target self-hosted --build-manifest <path>` (consumes the local manifest identically to a CI manifest) |

> **Do NOT instruct the operator to trigger GitHub CI.** Because the CI
> workflows are `disabled_manually`, the images + per-env config bundle are
> produced by the in-repo local-operator build path
> ([`scripts/commands/build-self-hosted.sh`](../../../scripts/commands/build-self-hosted.sh)),
> which signs each image + the manifest with the operator's cosign key
> (`trustModel: local-operator`) and writes
> `local-build-manifest-<sourceSha>.yaml`.
> [`scripts/deploy/promote.sh`](../../../scripts/deploy/promote.sh) parses both
> the CI list-shape and the local-operator object-shape manifest through the
> same code path (Spec 082 SCOPE-082-07), so promotion is identical.

---

## What This Activation Does (build `e0e7bc4f`, deploy, verify)

These runtime-relevant changes are on `main` awaiting a self-hosted deploy. Each
row's "Activation proof" is documented as an operator step in the runbook.

| # | Item | Type | Activation proof (post-deploy) |
|---|------|------|-------------------------------|
| 1 | **BUG-067-001** — ML sidecar `ML_LOG_LEVEL` is a **required fail-loud SST key** | DEPLOYMENT-CRITICAL config (pre-deploy gate) | Bundle / `app.env` carries `ML_LOG_LEVEL` (recommended `info`); ML `/health` comes up `model_loaded: true`. Missing ⇒ sidecar fails loud at startup. |
| 2 | **BUG-047-003** — alpine OpenSSL **CVE-2026-45447** patched in `smackerel-core` (`apk upgrade libssl3 libcrypto3`) | Image re-bake + CVE re-proof | The fresh local-operator build re-bakes core and runs the Trivy CRITICAL/HIGH gate on it (scope 4 re-proof); live `/ask` sourced-answer is scope 5. |
| 3 | **GAP-06 (spec 054)** — notification decisions route through the spec-078 surfacing **budget controller** | Runtime behavior change | `smackerel_surfacing_*{producer="notification"}` series appears on core `/metrics` after a notification flows. |
| 4 | **Model default** — self-hosted synthesis default is **`qwen3:30b-a3b`** | Pre-deploy config selection | Adapter `params.yaml` `model_selection` resolves the synthesis model id to `qwen3:30b-a3b` (NOT `deepseek-r1:7b`). |

> **No new feature flag, no schema migration** is introduced by this
> activation. GAP-06 shares one already-wired `surfacing.Controller` +
> `InMemoryAck`; BUG-067-001 adds only a required SST key
> (`services.ml.log_level`, value `info` already in `config/smackerel.yaml`).

---

## Post-Deploy Live Verifications That Close Gated Work

The live self-hosted GPU stack is the proof environment for a set of bugs/specs
that are `blocked` **solely** on this deploy + GPU verify (the code is fixed
in-repo). Each is documented in the runbook with an exact operator step and a
pass signal:

| Item | What the live verify proves | Closes-on |
|------|-----------------------------|-----------|
| **BUG-064-001 / BUG-064-002** | live `/ask` returns a **sourced answer** (`scenario_id=open_knowledge`, `num_sources>0`, `termination_reason != cap_usd`), NOT the canonical refusal; no `/ask ` prefix leaks into the capture title; no 3× snippet-dump / "thinking…" header | redeploy + live GPU verify |
| **specs 084 / 087 / 088** | open-knowledge reasoning-loop + genuine-synthesis + runtime-switchable-models — operator GPU **A/B re-verify** on the live stack | live GPU verify |
| **BUG-069-002** | client-disconnect durability — a `/api/assistant/turn` (or capture) request whose client drops mid-flight STILL persists the capture to Postgres + NATS (durable write survives `r.Context()` cancel) | live stack exercise |
| **BUG-047-003 scope 5** | live `/ask` sourced-answer verify after the CVE-fix core deploy | live GPU verify |

---

## Day-1 Ops (treat as activation gates, not assumed-covered)

After apply, the operator runs the first **live self-hosted** operational cycle —
today only DEV-stack SLO evidence exists:

- **First post-apply backup** + a **restore-drill** + an **operate-plane SLO
  capture** on the live self-hosted stack. Reference
  [`docs/Operations.md`](../../../docs/Operations.md) and
  [`docs/Upkeep_Runbook.md`](../../../docs/Upkeep_Runbook.md) generically.
- **Offsite T3/T4 + BCDR remain WARN** (`offsite_required: false`) until backup
  hardware lands — this is a **known limitation, not a blocker**.

---

## Separate Operator Action — Tagged Release (NOT self-hosted)

- **BUG-058 (chrome-extension bridge)** unblocks ONLY on an **operator-cut
  TAGGED release** (`refs/tags/v*`), which lets CI keyless-cosign sign the
  chrome-bridge zip to public Rekor. This is a **CI / release action, not a
  self-hosted deploy** — it does not flow through `promote.sh` / the adapter. See
  [`runbook.md` → Tagged-Release Action](runbook.md#separate-operator-action--tagged-release-not-self-hosted).

---

## Out-of-knb Follow-up (note for completeness — NOT this handoff)

A short appendix in the runbook lists 5 bugs that were **fix-complete +
verified-green in-repo** but, at authoring, still `in_progress` because the
`state-transition-guard` G022 done-cert pipeline had not been run (light-touch
fixes): **BUG-034-004, BUG-076-001, BUG-095-001, BUG-073-003, BUG-077-002**.
**Reconciliation (2026-06-25):** 4 of the 5 have since reached `status: "done"`
in committed state.json — **BUG-034-004, BUG-073-003, BUG-076-001,
BUG-095-001**; only **BUG-077-002** (spec 077 / `BUG-002`) remains
`in_progress`. These are an **in-repo framework-cert matter (not knb / not
self-hosted)** and are listed only so nothing falls through. See
[`runbook.md` → Out-of-knb Follow-up](runbook.md#appendix--out-of-knb-follow-up-in-repo-cert-matter-not-this-handoff).

---

## Ownership Boundary (why this packet stays generic)

The self-hosted adapter, real hostnames / IPs, tailnet identity, Caddy site files,
`ufw` rules, systemd unit names, and secret values are **operator-private** and
live in the deploy-adapter overlay (the `knb` repo, resolved via
`DEPLOY_TARGETS_ROOT` → `<deployment-owner>/<product>/<target>/`) — never in
this repo (see [`.github/copilot-instructions.md`](../../../.github/copilot-instructions.md)
→ "Deployment Ownership Boundary" and "No Env-Specific Content In This Repo").
This packet describes only the **generic product-side flow** plus the shipped
changes; every target-specific value is a placeholder the adapter fills. The
deploy host is referred to ONLY as `self-hosted` / `<deploy-host>` /
`<host-bind-address>`.

See [`runbook.md`](runbook.md) for the step-by-step build, promote, apply,
per-bug live verification, rollback, and Day-1 ops procedure.
