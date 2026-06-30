# Spec Review Report — Portfolio Audit (detect + classify)

**Generated:** 2026-06-20 by `bubbles.spec-review` (alias: Gary Laser Eyes)
**Mode:** detect + classify (no compaction); read-only `spec-review-to-doc`
**Scope:** All `specs/NNN-*/` (98 numbered specs, 001–098) + `specs/_ops/*` (8 packets)
**Depth:** quick (state.json + git history + targeted file-existence/drift checks; not full per-spec behavioural cross-check)

> **Layered newest-on-top.** The **2026-06-30 readiness-sweep refresh**
> (immediately below) is the newest layer: it adds the OPS-003 / OPS-004 home-lab
> handoff packets the prior layers omitted (the F3 stale-report finding), records
> the evo-x2 readiness-sweep remediation shipped 2026-06-30, and dispositions the
> 082 `releaseTrain` (F4) and 087/088 CI-as-producer (F5) narrative findings.
> Beneath it, the **2026-06-23 reconciliation refresh** supersedes the
> **2026-06-20 portfolio audit** for the model-target headline and the
> `097`/`098`/`099` status lines. The 2026-06-20 audit — which itself superseded
> the stale **2026-06-02 baseline** (flagged by spec 082) and the **2026-06-10
> go-live addendum**, both preserved under
> **[Historical Record](#historical-record)** — is retained verbatim beneath the
> 2026-06-23 section for audit trail. Where any two layers disagree, the newest
> wins. (The 2026-06-20 header describes that audit's 98-spec scope; the portfolio
> is now 99 numbered specs, 001–099 — see the 2026-06-23 refresh.)

---

## Summary (2026-06-30 readiness-sweep refresh)

This refresh closes the spec-review findings (F1 / F3 / F4 / F5) raised during the
cross-repo MVP / evo-x2 readiness sweep that ran and **shipped remediation on
2026-06-30** (all smackerel-side fixes committed + pushed to `origin/main`; the
sibling knb-side fixes committed + pushed in the **knb** repo — referenced here,
not imported). It is a read-only `spec-review-to-doc` layer: it documents
classifications and dispositions and **mutates no spec `state.json`**.

### 1. OPS-003 / OPS-004 home-lab handoff packets — now covered (F3)

The 2026-06-20 and 2026-06-23 layers predate the two home-lab handoff packets and
omitted them. Both are documentation-only `deploy-pointer` `_ops` packets
(objective + runbook + state), `delivered_pending_activation`, owner
`bubbles.devops`:

| Packet | Status | Trust | Classification |
|--------|--------|-------|----------------|
| `_ops/OPS-003-gap06-bug067-homelab-deploy-handoff` | `delivered_pending_activation` (certified 2026-06-23) | **SUPERSEDED** | Handed off GAP-06 (spec 054 Scope 9) + BUG-067-001 (`ML_LOG_LEVEL` fail-loud SST) at interior payload SHA `78b293cc`. **Superseded for the live deploy by OPS-004** (which declares `supersedes: OPS-003`). Retained verbatim for audit trail; one-way supersession, no contract conflict between the two. |
| `_ops/OPS-004-homelab-activation-handoff` | `delivered_pending_activation` (certified 2026-06-24) | **CURRENT** | The single consolidated, authoritative go-live mechanism. Folds OPS-003's payload in alongside every other runtime-relevant change on `main` awaiting a home-lab deploy + live GPU verify. Documents the **local-operator** build path (GitHub CI `disabled_manually`; producer = `./smackerel.sh build --target home-lab` → `local-build-manifest-<sourceSha>.yaml`, `trustModel: local-operator`, consumed identically by `promote.sh`), the PRE-DEPLOY config gates (`ML_LOG_LEVEL`; synthesis = `qwen3:30b-a3b`), per-bug live verifications, pointer-swap rollback, and Day-1 ops. |

OPS-004 is the packet the operator is routed to for go-live; OPS-003 is its direct
predecessor.

### 2. evo-x2 readiness-sweep remediation shipped 2026-06-30 (discharges F1)

The sweep (devops + code-review + spec-review passes) shipped fixes that discharge
the prior layers' standing **F1 — local-operator-path readiness** risk (the
readiness/clarity of the OPS-004 local-operator go-live path) plus the
supply-chain / fail-loud nits found alongside it. **smackerel side** (committed +
pushed to `origin/main`; tracked in `_ops/OPS-005-evo-x2-readiness-sweep-fixes`):

- **OPS-005 SF-1** — digest-pinned the two profile-gated third-party images
  (prometheus `…@sha256:2659f4c2…`, searxng `…@sha256:f134249d…`) in the SST
  (`config/smackerel.yaml`) + `deploy/contract.yaml`, matching the
  postgres/nats/ollama `tag@sha256` format, and locked both against drift in
  `internal/deploy/external_images_contract_test.go`.
- **OPS-005 SF-2** — converted the `deploy/compose.deploy.yml` core / ml /
  prometheus image refs from plain `${VAR}` to the `${VAR:?…}` fail-loud form
  (matching searxng + `smackerel-no-defaults`); resolved value unchanged, guard
  hardened.
- **OPS-005 SF-3** — operator breadcrumb in `docs/Deployment.md` Go-Live Readiness
  Checklist §4: the `validateModelEnvelopes` co-residence OOM guard holds only
  while `OLLAMA_KEEP_ALIVE` keeps interactive models resident.
- **`docs/Upkeep_Runbook.md`** two-layer-split reconcile.
- **BUG-069-003** — direct `/api/capture` disconnect-durability fix
  (`context.WithoutCancel` + adversarial test); `in_progress`, pending the shared
  live-stack E2E with BUG-069-002 (folded into the final validation sweep — see
  the residual-item note below).

**knb side** (committed + pushed in the **knb** repo, packet
`specs/_ops/OPS-003-smackerel-evo-x2-readiness-sweep`, now complete — **referenced,
not imported** here): KF-1 `bcdr-drill.sh:43` syntax fix; KF-2 restore-test + bcdr
trap-EXIT cleanup; KF-3 backup `gzip -t` integrity gate; KF-4 restic retention
prune; KF-5 activation-doc reconcile. These land in the operator-private deploy
overlay and harden the OPS-004 Day-1 ops / BCDR path; they are owned and tracked
in knb.

### 3. Spec 082 `releaseTrain: next` rationale — documented (F4)

`specs/082-mvp-evo-x2-readiness-hardening` (status `done`, certified 2026-06-10)
carries `releaseTrain: next`, not `mvp`, despite its "MVP / evo-x2 readiness
hardening" title. F4 offered "reconcile OR document"; the honest rationale is
**documented here (082 `state.json` NOT edited)**:

- 082 was authored as a **next-train hardening batch**. Its in-repo scopes (deploy
  resource / filesystem hardening, monitoring-stack wiring, tailnet edge-bind,
  alerts-connector — see `specDependsOn`) **benefit** MVP go-live, but the work is
  **independent of the mvp-train feature set** (no `mvp`-bundle feature flag is
  gated by 082; `flagsIntroduced: []`).
- Per the release-train model the train tag is owned by **`bubbles.train`**;
  retagging 082 `next`→`mvp` is its call, is **not required for go-live**, and is
  out of scope for a read-only spec-review pass. The `next` tag is therefore
  **honest, not drift** — recorded so no future reader treats the title /
  `releaseTrain` mismatch as an unexplained inconsistency.

### 4. Specs 087 / 088 CI-as-producer narrative drift — SUPERSEDED, non-gating (F5)

`specs/087-open-knowledge-genuine-synthesis` and
`specs/088-runtime-switchable-models` (both `blocked`, `releaseTrain: next`,
**non-MVP**) carry a stale **"CI-as-producer"** deploy narrative in their
`state.json` `ci` / `deploy` fields (e.g. "build workflow: build-images ✓ … cosign
keyless + Rekor signed … ghcr digest push"). That contradicts **OPS-004's
authoritative local-operator path** (GitHub CI is `disabled_manually`; the live
producer is `./smackerel.sh build --target home-lab`, `trustModel:
local-operator`).

**Disposition:** the 087/088 CI-as-producer narrative is **SUPERSEDED by OPS-004**
(the packet the operator is actually routed to for go-live) and is **non-gating
for MVP** (087/088 are non-MVP, `next`-train, and `blocked` solely on the
operator-owned `bubbles.devops` home-lab apply + live GPU A/B re-verify, which
OPS-004 already documents via the local-operator path). **Recommendation:**
reconcile 087/088's own `ci` / `deploy` narrative to the local-operator producer
**when they next unblock** (owner `bubbles.devops` / `bubbles.docs`). **No
087/088 `state.json` edited by this pass** (ownership-respecting).

### Trust deltas / dispatch (2026-06-30)

- **No new MAJOR_DRIFT or OBSOLETE on any certified spec** → **no
  `bubbles.workflow mode=improve-existing` dispatch owed.** The 087/088 narrative
  drift is real but **SUPERSEDED** by an authoritative sibling packet (a stale
  annotation, not a contract divergence in a certified spec) and **non-gating**;
  per the spec-review skill it is documented + recommended for next-unblock
  reconcile, not auto-dispatched.
- **F1 / F3 discharged** by the 2026-06-30 sweep remediation + this refresh;
  **F4 / F5 dispositioned** above. The one **open** smackerel readiness item is
  **BUG-069-003**'s live-stack E2E (shared with BUG-069-002), folded into the
  final go-live validation sweep on the live home-lab stack — tracked under its
  own bug packet, `in_progress`.
- Read-only `spec-review-to-doc`; outcome = `docs_updated`. **No spec `state.json`
  mutated** (082 / 084 / 087 / 088 / … untouched); the only writes this pass are
  this report layer and the OPS-005 packet close-out (SF-4 → done).

### Validation Checklist (2026-06-30)

- [x] OPS-003 + OPS-004 read from disk and classified (SUPERSEDED / CURRENT) against their `state.json` `supersedes` / `status`.
- [x] 2026-06-30 sweep remediation cross-referenced to OPS-005 (SF-1 / SF-2 / SF-3) + BUG-069-003 + the knb OPS-003 sweep packet (referenced, not imported).
- [x] 082 `releaseTrain: next` rationale documented; `state.json` read-only (NOT edited).
- [x] 087 / 088 CI-as-producer narrative dispositioned SUPERSEDED + non-gating; `state.json` read-only (NOT edited).
- [x] Older layers (2026-06-23, 2026-06-20, Historical Record) preserved verbatim (append-only, newest-on-top).
- [x] No spec `state.json` mutated by this pass; no MAJOR_DRIFT / OBSOLETE → no mandatory dispatch.
- [x] No knb / operator PII (generic home-lab / operator framing; knb items referenced by packet id only).

---

## Summary (2026-06-23 reconciliation refresh)

This refresh reconciles five 2026-06-20 annotations to committed `HEAD` reality
(the model-config sync that was "uncommitted working-tree" on 2026-06-20 has
since landed; `097` certified `done`; `099` shipped):

1. **Model-target headline CORRECTED — committed reality is `qwen3:30b-a3b`, not
   `gpt-oss:20b`.** The 2026-06-20 refresh (headline #3 + the `087`/`088`/`089`
   notes) recorded the home-lab open-knowledge *synthesis* target as the
   uncommitted-working-tree `gpt-oss:20b` (+ `gemma4:26b` gather). That interim
   value was **discarded, never committed**. The committed standing synthesis
   default is now **`qwen3:30b-a3b`** (`config/smackerel.yaml` ~L1230, commit
   `05b9f677`, pushed; selection delegated to the deploy-adapter `params.yaml`;
   lineage `deepseek-r1:32b` → interim `gpt-oss:20b` → `qwen3:30b-a3b`). The
   GATHER model remains `gemma4:26b`. Any note citing `gpt-oss:20b` as the
   current synthesis target is stale.
2. **Model reconciliation is now COMMITTED (was "uncommitted working tree").**
   The 2026-06-20 caveat ("the committed tree still carries the
   pre-reconciliation deepseek/llama narrative … until the operator commits the
   10-file changeset") is discharged: `config/smackerel.yaml`'s `qwen3:30b-a3b`
   profile + synthesis migration is committed (`05b9f677`, pushed), and the
   BUG-067-001 ML NO-DEFAULTS fix is committed (`78b293cc`;
   `policy-exception-baseline.json` is now empty). The 2026-06-23 annotation pass
   refreshed `087`/`088` `blockedReason` + `devopsExecution.deploy` and added the
   `089` `report.md` qwen3 supersession breadcrumb to match.
3. **`097-card-rewards-gcal-delivery` → `done`** (was `in_progress`). Certified
   `done` (commit `3688666a`, "spec(097): certify card-rewards gcal delivery
   done"; renumbered from `089` via `c7f31b29`). Reclassifies **CURRENT**; it
   leaves the PARTIAL/in-progress set.
4. **`099-preflight-resource-guard` added — `done`, `releaseTrain: mvp`.** New
   post-`098` spec (certified `done`, commits `7034f49f` / `6029e4eb`).
   Classifies **CURRENT**. Portfolio is now **99 numbered specs (001–099)**.
5. **`098` CI-red Trivy MINOR_DRIFT reconciled.** The embedded "Trivy
   smackerel-ml RED → `publish-build-manifest` SKIPPED" condition is resolved at
   the infrastructure level: the ml CVE remediation (`4debc4f0` + `d684f7bc`)
   turned CI green, and the portfolio-reconciliation commit `78b293cc`
   (BUG-067-001 ML fail-loud SST + portfolio reconciliation, which also refreshed
   the `docs/releases/mvp/features.md` go-live inputs) closed the go-live drift;
   CI now publishes a server-only build-manifest. `098`'s own inline
   Discovered-Issues note is a cosmetic update-on-next-touch (the spec's contract
   is independently GREEN in-repo); the *drift* is reconciled.

### Status tally (2026-06-23, state.json authority)

92 `done` + 2 `specs_hardened` (`063`, `079`) + 4 `blocked` (`058`, `084`,
`087`, `088`) + 1 `in_progress` (`096`) = **99**. Deltas vs 2026-06-20: `097`
`in_progress`→`done`; `099` added (`done`); `096` remains genuinely mid-flight.
The four blocked specs remain honest, validated-in-repo, gated on the
operator-owned `bubbles.devops` home-lab apply + live re-verify (`058` on the
keyless-OIDC / public-Rekor CI-release row).

### Trust-level deltas vs 2026-06-20

| Spec | 2026-06-20 | 2026-06-23 | Reason |
|------|-----------|-----------|--------|
| `097` | in_progress (PARTIAL) | **CURRENT** (done) | Certified done `3688666a`. |
| `099` | not covered | **CURRENT** (done) | New spec; done; mvp train. |
| `098` | MINOR_DRIFT (Trivy-red note) | **CURRENT** (drift reconciled) | CI green (`4debc4f0`/`d684f7bc`) + `78b293cc`; inline note cosmetically pending. |
| `087`/`088` | blocked (gpt-oss note) | blocked (qwen3 reconciled) | `blockedReason`/`deploy` refreshed; status unchanged. |
| `089` | CURRENT (gpt-oss note) | CURRENT (qwen3 breadcrumb) | `report.md` supersession breadcrumb → `qwen3:30b-a3b`. |

> **`MINOR_DRIFT` carry-overs `039` / `067`** are unchanged by this refresh (still
> cosmetic deleted-file prose); re-point on next touch as before.

---

## Summary (2026-06-20 refresh)

> **SUPERSEDED 2026-06-23** for the model-target headline (#3) and the
> `097`/`098`/`099` status lines — see the **2026-06-23 reconciliation refresh**
> above. Preserved verbatim for audit trail.

| Trust level | Count | Notes |
|-------------|-------|-------|
| **CURRENT** (fresh) | 89 | Spec accurately reflects current implementation. Safe to treat as source of truth. Includes 089 (freshly reconciled — see below). |
| **MINOR_DRIFT** (mostly fresh) | 3 | Cosmetic / annotation staleness only — contracts sound. `039`, `067` (deleted-file prose, carried from 2026-06-02), `098` (now-stale CI-RED / manifest-skipped discovered-issue note). |
| **MAJOR_DRIFT** | 0 | None. |
| **OBSOLETE** | 0 | None. |
| **PARTIAL / in-progress / blocked** | 6 | Non-terminal but honestly self-documented. blocked: `058`, `084`, `087`, `088`; in_progress: `096`, `097`. |

**Coverage:** 98 numbered specs (001–098) + 8 `_ops` packets. The `_ops` packets:
`OPS-001-spec-banner-sweep` and `OPS-002-g088-certifiedat-backfill` are
`specs_hardened` (terminal-for-mode); `F-057-V-001-e2e-ui-harness` (resolved by
spec 077) and the five `sweep-round-*` packets are README-only historical/by-design
records (no `state.json`). None are drift.

**Status reconciliation (verified live, state.json authority):**
90 `done` + 2 `specs_hardened` (`063`, `079`) + 4 `blocked` (`058`, `084`, `087`,
`088`) + 2 `in_progress` (`096`, `097`) = 98. `089` is `done`.

**No spec requires rewrite. Zero MAJOR_DRIFT, zero OBSOLETE.** The only true drift
is three cosmetic MINOR_DRIFT items; the six non-terminal specs each honestly
document their own state in `state.json`.

### Headline findings

1. **Portfolio grew 76 → 98 numbered specs** since the 2026-06-02 baseline (specs
   `077`–`098` are all post-baseline). This refresh classifies the full set.
2. **CI is GREEN on HEAD `ad372f13`** (Actions run `27879677569`) after the
   `smackerel-ml` Trivy CVE remediation — litellm `CVE-2026-49468` +
   starlette `CVE-2026-48818`/`CVE-2026-54283`, fixed in commits `4debc4f0`
   (litellm 1.84.0 + fastapi/starlette ≥1.3.1) and `d684f7bc` (correct the ml
   Dockerfile starlette force-upgrade to `==1.3.1`). The build workflow now
   publishes a build-manifest. **Any spec note implying CI is red on the ml image
   or that no manifest is published is now stale** — the single spec carrying such
   a note is `098` (MINOR_DRIFT, below).
3. **Uncommitted working-tree model reconciliation.** A model-config sync is live
   in the working tree (10 modified files, NOT committed): it repoints the
   home-lab assistant synthesis selection from `deepseek-r1:32b` / `llama3.1:8b`
   to the operator's optimized `gpt-oss:20b` + `gemma4:26b` set and adds
   **record-only** supersession notes to specs `087`/`088`/`089` + the A/B
   experiment doc. `config/smackerel.yaml`'s home-lab block is now deepseek/llama-free.
   Verified: status / certification / history for `087`/`088`/`089` are **unchanged**
   (only `notes` / `report.md` annotations were appended); `087`'s `state.json` was
   not touched at all (only its `report.md`). No un-annotated "current home-lab
   model = deepseek/llama" claim remains elsewhere — the residual deepseek/llama
   mentions in `045`/`052`/`071`/`072`/`084` are all incidental/historical
   (memory-profile catalogs, git-log quote evidence, wiring notes, resolution
   rationale), not current-selection contracts.
4. **`096`'s own `state.json` `notes` are internally stale** — they still narrate
   the 2026-06-18 analyst-bootstrap claim that `design.md`/`scopes.md`/`report.md`
   are "intentionally absent," but all seven artifacts now exist and SCOPE-01..07
   + §13 observability have landed. The spec is correctly `in_progress` (not
   promotable); the stale notes are a within-spec annotation lag, not a trust
   contradiction.
5. **The four blocked specs are honest, not drifted.** `058` is gated solely on a
   keyless-OIDC / public-Rekor signing row that requires a real tagged CI release;
   `084`/`087`/`088` are validated-in-repo and gated on the owner-directed
   `bubbles.devops` deploy handoff (the CI-green precondition is now satisfied;
   the live home-lab apply + GPU A/B re-verify remain operator-owned).

---

## Method (2026-06-20)

1. Read `state.json` (`status`, `certification.status`, `mode`, `releaseTrain`,
   `blockedReason`, `notes`) for all 98 numbered specs + 8 `_ops` packets.
2. Reconciled the live status tally against the portfolio (90/2/4/2 split confirmed).
3. Cross-checked recent commits (`git log --since=2026-06-09`) and the
   uncommitted working tree (`git status --short` = 10 modified files) against
   spec coverage areas.
4. Verified the CVE remediation chain (commits `4debc4f0`, `d684f7bc`, HEAD
   `ad372f13`) and that the build workflow is green / publishes a manifest.
5. Inspected the uncommitted model-config reconciliation diffs (state.json `notes`
   + `report.md` supersession blocks for `087`/`088`/`089`; `config/smackerel.yaml`
   home-lab block) to confirm they are record-only and status-preserving.
6. Re-tested the 2026-06-02 MINOR_DRIFT signals: confirmed
   `internal/api/domain_intent.go` is still deleted and `039`/`067` still carry the
   prose references.
7. Grepped the portfolio for stale "CI red / manifest not published / unresolved
   CVE" claims and for un-annotated deepseek/llama current-selection claims.

Depth caveat: full behavioural cross-check (Gherkin-vs-code) was not performed for
every spec — only where signals indicated drift.

---

## Findings by Trust Level (2026-06-20)

### MAJOR_DRIFT — Do NOT rely on spec until fixed

_None._

### OBSOLETE — Ignore entirely

_None._

### MINOR_DRIFT — Usable but verify

| Spec | Status | Drift | Action |
|------|--------|-------|--------|
| `specs/039-recommendations-engine` | done | `spec.md` evidence still cites `internal/api/domain_intent.go` (deleted by spec 066 SCOPE-4) as live evidence of price-filter parsing; the spec self-notes supersession. Cosmetic prose-only. Carried unchanged from 2026-06-02. | Re-point the evidence cell at the spec 068 compiled-intent path on next touch. No guard impact. |
| `specs/067-intent-driven-policy-enforcement` | done | `design.md` / `spec.md` inventory still list `internal/api/domain_intent.go` as a current legacy surface (deleted by 066). Historical-record framing is plausible (067 is the policy spec that motivated 066's deletion) but reads present-tense. Carried unchanged from 2026-06-02. | Re-frame as past-tense ("retired by spec 066 SCOPE-4") on next touch. |
| `specs/098-ci-server-manifest-client-decoupling` | done | `state.json` (l.159) + `report.md` (l.340) carry a validation note recording the foreign `build-images` "Trivy vulnerability scan — smackerel-ml" step as **RED** (run `27865311625`) and the consequent **skipped `publish-build-manifest`**, with the live server-only-manifest path "not exercised" under Discovered Issues. **That condition was remediated** by the CVE fix (`4debc4f0` + `d684f7bc`); CI is green on HEAD with a manifest published. The spec's own contract is independently proven in-repo (12/12 contract tests GREEN) — only the embedded CI-state annotation drifted. | On next touch, update the Discovered-Issues note to record that the Trivy-ml gate is now green and the server-only-manifest path is exercisable / exercised. Optional `bubbles.docs` sync if `docs/Deployment.md` go-live inputs reference the old red-CI state. |

### PARTIAL / in-progress / blocked (not drift)

| Spec | State | Notes |
|------|-------|-------|
| `specs/058-chrome-extension-bridge` | blocked | All external-infra blockers discharged (MV3 e2e harness 11/11 PASS, live-Postgres + HTMX admin landed, build-manifest contract + supply-chain proofs PASSING). The **sole irreducible remaining DoD row** is keyless-OIDC: `cosign sign-blob` under a GitHub Actions OIDC token recorded in a real public-Rekor entry — requires a tagged CI release and cannot be honestly produced on a dev box. Honestly self-documented; out of MVP go-live scope. |
| `specs/084-open-knowledge-reasoning-loop` | blocked | Validated-in-repo: 3 scopes Done, 9/9 unit tests GREEN, `check`/`format`/`artifact-lint`/`traceability-guard` pass. Gated on the owner-directed `bubbles.devops` handoff (isolated push + CI + home-lab apply + operator live re-verify). CI-green precondition now satisfied; live deploy remains operator-owned. `certifiedAt` correctly null. |
| `specs/087-open-knowledge-genuine-synthesis` | blocked | Same validated-in-repo / devops-handoff gate as 084. **Freshly reconciled** (uncommitted): `report.md` now carries a record-only supersession note (deepseek-r1:7b synthesis arm → `gpt-oss:20b` + `gemma4:26b`). Status/certification unchanged. |
| `specs/088-runtime-switchable-models` | blocked | Same gate; 40/40 DoD + 30/30 spec-088 tests GREEN in-repo. **Freshly reconciled** (uncommitted): `report.md` + `state.json` `notes` carry the record-only supersession note (switchable set → `[gpt-oss:20b, gemma4:26b]`). Status/certification unchanged. |
| `specs/096-multi-provider-model-connections` | in_progress | Genuinely mid-flight: SCOPE-01..07 + §13 observability landed across 2026-06-18..20; all seven artifacts present. **Caveat:** the `state.json` `notes` field is internally stale (still claims the 2026-06-18 analyst-bootstrap artifact absence). Correctly non-terminal; not promotable past `in_progress`. |
| `specs/097-card-rewards-gcal-delivery` | in_progress | Renumbered from `089` on 2026-06-20 (commit `c7f31b29`) to de-duplicate the `SCN-089`/`FR-089` namespace against the runtime-model-hotswap spec that now owns `089`. Fresh by definition; `lastUpdated` 2026-06-20. |

### CURRENT — Safe to use as source of truth

All **89** numbered specs **not** listed in the MINOR_DRIFT or PARTIAL/blocked
sections above classify as CURRENT, on the same quick-depth basis as prior audits:
`status` terminal-for-mode, `certification.status` matching `status`, no post-cert
commits to their primary code paths that contradict the spec, and no references to
files deleted/moved by later specs.

CURRENT = `001`–`098` minus `{039, 058, 067, 084, 087, 088, 096, 097, 098}`. By range:

```
001–038            (38 specs)
040–057            (18 specs)
059–066            ( 8 specs)
068–083            (16 specs)  ← incl. 063, 079 specs_hardened (terminal-for-mode)
085, 086           ( 2 specs)
089                ( 1 spec)   ← freshly reconciled (see note)
090–095            ( 6 specs)
                   = 89 CURRENT
```

Freshly-reconciled CURRENT:
- **`089-runtime-model-hotswap-persistent-selection`** (done) — its `report.md` +
  `state.json` `notes` now carry the record-only home-lab model supersession note
  (`deepseek-r1:32b` standing default → `gpt-oss:20b` + `gemma4:26b`). Status =
  `done`, certification unchanged. Annotation is uncommitted working-tree state.

Verify-before-trust caveats inside CURRENT (unchanged from prior audits):
- **Phase roadmap specs (`001`–`006`)** are intentionally high-level — historical
  roadmap, not implementation contract.
- **Recent interconnected assistant/model specs (`061`, `063`, `064`, `068`–`074`,
  `078`, `080`–`083`, `085`, `086`, `089`–`095`)** were certified during heavy
  churn; their internal coverage is fresh, but cross-spec scenario-ID references
  should be spot-checked before reuse.
- The model-reconciliation working tree is **uncommitted**: the committed tree
  still carries the pre-reconciliation deepseek/llama narrative for `087`/`088`/`089`
  until the operator commits the 10-file changeset.

---

## Auto-Dispatch Decisions (2026-06-20, Phase 5)

| Trigger | Required dispatch | Status |
|---------|-------------------|--------|
| Any `done`/`specs_hardened` spec at **MAJOR_DRIFT** or **OBSOLETE** | `bubbles.workflow mode=improve-existing` | **None required** — zero MAJOR_DRIFT, zero OBSOLETE in the portfolio. |
| MINOR_DRIFT (`039`, `067`, `098`) | None auto-required (per skill: MINOR_DRIFT does not auto-dispatch) | Added to follow-up suggestions. |
| Managed-docs impact from MAJOR_DRIFT | `bubbles.docs` | **None required** — no MAJOR_DRIFT. `098`'s stale CI-state note is recorded as an optional `bubbles.docs` follow-up only (read-only mode — not dispatched). |

**`route_required` packets:** none. No certified spec is at MAJOR_DRIFT/OBSOLETE,
so no `bubbles.workflow mode=improve-existing` dispatch is owed. This is a read-only
`spec-review-to-doc` audit; outcome = `docs_updated`.

---

## Validation Checklist (2026-06-20)

- [x] Every queued spec analyzed (98 numbered + 8 `_ops` = 106 entries).
- [x] Every spec has a trust classification with supporting evidence (CURRENT grouped; non-CURRENT enumerated individually).
- [x] Status tally reconciled against live `state.json` (90 done / 2 specs_hardened / 4 blocked / 2 in_progress).
- [x] File-path references in MINOR_DRIFT findings verified against filesystem (`internal/api/domain_intent.go` confirmed deleted; `098` note lines confirmed at `state.json:159` / `report.md:340`).
- [x] Git history used actual commit data (`ad372f13`, `4debc4f0`, `d684f7bc`, `c7f31b29`; Actions runs `27879677569`, `27865311625`).
- [x] Uncommitted model-reconciliation changeset verified record-only / status-preserving for `087`/`088`/`089`.
- [x] Report written to `specs/_spec-review-report.md` (this refresh; prior body preserved under Historical Record).
- [x] Compact mode NOT engaged (detect + classify only).
- [x] No MAJOR_DRIFT/OBSOLETE → no mandatory dispatch; no `state.json`/status/certification modified by this audit.
- [x] No knb/operator PII in the report (generic home-lab/operator framing only).

---

# Historical Record

> **Append-only / preserved verbatim.** The two sections below are the original
> **2026-06-02 baseline** and the **2026-06-10 addendum**. They are superseded by
> the 2026-06-20 refresh above and retained for audit trail only. Where they
> disagree with the refresh, the refresh wins.

## Summary

| Trust level | Count | Notes |
|-------------|-------|-------|
| **CURRENT** (fresh) | 66 | Spec accurately reflects current implementation. Safe to treat as source of truth. |
| **MINOR_DRIFT** (mostly fresh) | 7 | Small staleness — prose mentions of retired files, cosmetic banner pending, or open follow-up packet. Usable but verify specific details. |
| **MAJOR_DRIFT** | 0 | None. (Spec 026 reclassified CURRENT on 2026-06-03 after verification dispatch — see Auto-Dispatch Decisions.) |
| **OBSOLETE** | 0 | None detected. |
| **PARTIAL / in-progress** | 2 | spec 076 (in_progress) + ops follow-up packet F-057-V-001 (open). |

No spec requires rewrite. The original MAJOR_DRIFT finding (spec 026) was verified on 2026-06-03 to be a false positive: scenario-manifest already correctly rewired by BUG-026-001; `linkedTests` fields are empty and scenarios are marked `status=superseded` with `supersededBy=066-natural-language-intent-routing`. The original grep matched narrative `supersededNote` text, not live `linkedTests`. `artifact-lint` and `traceability-guard` both pass.

---

## Method

1. Read `state.json` for all 76 numbered specs + 2 ops packets.
2. Cross-checked recent code changes (`git log --since=2026-04-01`) against spec coverage areas.
3. Spot-checked specs that reference recently deleted files (`internal/api/domain_intent.go`, deleted in spec 066 SCOPE-4 on 2026-06-02).
4. Verified that the 2026-05-28 / 2026-06-01 mass spec.md updates were the cosmetic banner sweep under `_ops/OPS-001-spec-banner-sweep` (Categories A–D), not content drift.
5. Identified rescope close-outs (065 → 076, 066 → 076, 075 → 076) and confirmed the source specs document their own rescoping properly.

Depth caveat: full behavioural cross-check (Gherkin-vs-code) was not performed for every spec — only where signals indicated drift.

---

## Findings by Trust Level

### MAJOR_DRIFT — Do NOT rely on spec until fixed

_None as of 2026-06-03._

#### specs/026-domain-extraction — RECLASSIFIED CURRENT 2026-06-03 (false positive)

Original 2026-06-02 finding claimed `scenario-manifest.json` contained 11+ links to `internal/api/domain_intent_test.go` / `internal/api/domain_intent.go` (deleted by spec 066 SCOPE-4). Verification dispatch on 2026-06-03 (`bubbles.workflow mode=improve-existing`) confirmed:

- SCN-026-8-1..8-4 are marked `status=superseded` with `supersededBy=066-natural-language-intent-routing` and **empty `linkedTests` arrays** — the manifest was already correctly rewired by BUG-026-001 traceability remediation.
- The original grep match was against the narrative `supersededNote` field (historical prose), NOT against any live `linkedTests` field.
- `artifact-lint specs/026-domain-extraction` → EXIT=0.
- `traceability-guard specs/026-domain-extraction` → EXIT=0.

No manifest changes were required. Spec 026 is CURRENT.

### MINOR_DRIFT — Usable but verify

| Spec | Drift | Action |
|------|-------|--------|
| `specs/039-recommendations-engine` | `spec.md` evidence table (line 70) cites `internal/api/domain_intent.go` as live evidence of price-filter parsing. File deleted by 066; spec itself notes supersession at line 1143. Cosmetic prose-only. | Update the evidence cell to point at spec 068 compiled-intent path next time the spec is touched. No blocking guard impact. |
| `specs/058-chrome-extension-bridge` | `status=done_with_concerns` + `certification.status=done_with_concerns`. Explicit known-concerns flag — drift is acknowledged by the artifact itself. | Verify the recorded concerns are still the only outstanding items; otherwise route to `bubbles.workflow mode=improve-existing`. |
| `specs/067-intent-driven-policy-enforcement` | `design.md` line 137 inventory still lists `internal/api/domain_intent.go` as a current legacy surface. Historical-record use is plausible (spec 067 is the *policy* spec that motivated 066's deletion) but the present-tense framing reads stale post-deletion. | Re-frame the inventory entry as past-tense ("retired by spec 066 SCOPE-4") on next touch. |
| `specs/075-legacy-retirement-telemetry` | Rescope close-out: SCOPE-1..5 moved to spec 076 (commit 67792d82). The rescope is documented in `report.md`, but any downstream artifact that referenced 075's original 6-scope shape is now stale. | Already self-documented. No action required unless an external doc references the retired scopes. |
| `specs/065-generic-micro-tools` | Same pattern: Scope 1 done, Scopes 2/3/4 rescoped to 076 (commit bce20e26). Properly closed. | None. |
| `specs/066-legacy-keyword-surface-retirement` | Same pattern: Scopes 1, 2, 4 done; 3 + 5 rescoped to 076 (commit 403daea4). Properly closed. | None. |
| `specs/_ops/OPS-001-spec-banner-sweep` | `status=specs_hardened` (terminal-for-mode `spec-scope-hardening`). The planning packet shipped; implementation handled by routine spec.md edits across 54 specs on 2026-05-28 / 2026-06-01. Idempotence (EB-7) not verified in this audit. | Run a quick `grep -L "**Status:** Done"` across the 54 enumerated specs to confirm EB-7 holds. |

### PARTIAL / in-progress (not drift)

| Spec | State | Notes |
|------|-------|-------|
| `specs/076-assistant-completion-rescope` | `in_progress` | Created 2026-06-02 to absorb the Scopes that 065/066/075 rescoped out. Fresh by definition. |
| `specs/_ops/F-057-V-001-e2e-ui-harness` | Open follow-up packet (README only, no `state.json` by design) | Tracked deferred work: no browser-engine harness exists yet. Not drift — this is a queued spec-to-be-written. |

### CURRENT — Safe to use as source of truth

All other 66 specs classify as CURRENT based on:
- `status` terminal-for-mode (`done`, `specs_hardened`, `delivered_pending_activation`),
- `certification.status` matching `status`,
- no recent (post-cert) commits to their primary code paths that would contradict the spec, and
- no references to files deleted/moved by later specs.

Listed alphabetically:

```
001-smackerel-mvp                       024-design-doc-reconciliation         050-ml-sidecar-health-isolation
002-phase1-foundation                   025-knowledge-synthesis-layer         051-deployment-secret-auth-contract
003-phase2-ingestion                    026-domain-extraction                 052-bundle-secret-injection-contract
004-phase3-intelligence                 027-user-annotations                  053-ci-ops-evidence-hardening
005-phase4-expansion                    028-actionable-lists                  054-notification-intelligence-handler
006-phase5-advanced                     029-devops-pipeline                   055-notification-source-ntfy-adapter
007-google-keep-connector               030-observability                     056-twitter-api-connector
008-telegram-share-capture              031-live-stack-testing                057-browser-login-redirect
009-bookmarks-connector                 032-documentation-freshness           059-google-keep-live-mode
010-browser-history-connector           033-mobile-capture                    060-bearer-auth-scope-claim
011-maps-connector                      034-expense-tracking                  061-conversational-assistant
012-hospitable-connector                035-recipe-enhancements               063-knowledge-ai-enrichment
013-guesthost-connector                 036-meal-planning                     064-open-ended-knowledge-agent
014-discord-connector                   037-llm-agent-tools                   068-structured-intent-compiler
015-twitter-connector                   038-cloud-drives-integration          069-assistant-http-transport
016-weather-connector                   040-cloud-photo-libraries             070-web-username-password-login
017-gov-alerts-connector                041-qf-companion-connector            071-intent-trace-observability
018-financial-markets-connector         042-tailnet-edge-bind-pattern         072-whatsapp-business-transport
019-connector-wiring                    043-ollama-test-infrastructure        073-web-mobile-assistant-frontend
020-security-hardening                  044-per-user-bearer-auth              074-capture-as-fallback-policy
021-intelligence-delivery               045-deploy-resource-filesystem-…
022-operational-resilience              046-nats-production-hardening
023-engineering-quality                 047-ci-image-vulnerability-gate
                                        048-backup-restore-automation
                                        049-monitoring-stack
```

Verify-before-trust caveats inside CURRENT:
- **Recent assistant-stack specs (061, 063, 064, 068, 069, 070, 071, 072, 073, 074)** were certified in the last 14 days during heavy interconnected churn. Their internal coverage is fresh, but cross-spec references (e.g. a spec citing another spec's scenario IDs) should be spot-checked before reuse.
- **Phase roadmap specs (001-006)** are intentionally high-level — they describe what each phase delivered, not current code. Treat as historical roadmap, not implementation contract.

---

## Auto-Dispatch Decisions (Phase 5)

| Trigger | Required dispatch | Status |
|---------|-------------------|--------|
| `specs/026-domain-extraction` MAJOR_DRIFT (status `done`) | `bubbles.workflow mode=improve-existing spec=specs/026-domain-extraction reason=spec-review:MAJOR_DRIFT` | **COMPLETED 2026-06-03** — Dispatched 2026-06-03; verified false-positive; no manifest changes required. Scenarios SCN-026-8-1..8-4 already `status=superseded` with empty `linkedTests` (BUG-026-001 prior remediation). `artifact-lint` and `traceability-guard` both EXIT=0. |
| MINOR_DRIFT items above | None auto-required (per skill: MINOR_DRIFT does not auto-dispatch) | Added to follow-up suggestions. |

### route_required packet (HISTORICAL — resolved 2026-06-03)

The packet below was emitted on 2026-06-02. It was dispatched on 2026-06-03 and the finding was verified as a false positive (see CURRENT entry for spec 026 above). No code or manifest changes resulted. Retained for audit trail.

```
agent: bubbles.workflow
mode: improve-existing
spec: specs/026-domain-extraction
reason: spec-review:MAJOR_DRIFT
rationale: scenario-manifest.json links 11+ scenarios to internal/api/domain_intent.go and internal/api/domain_intent_test.go which were deleted by spec 066 SCOPE-4 (commit 1f74d5c0). Prose in spec.md/design.md already acknowledges supersession; the manifest must be brought into line so the traceability guard and regression baseline stop resolving dead paths.
```

### Docs-agent invocation (per Phase 5 trigger table)

No MAJOR_DRIFT in a spec with managed-docs impact was found that required immediate `bubbles.docs` invocation. Spec 026's drift is scoped to its own `scenario-manifest.json`; `docs/Architecture.md` and `docs/API.md` were touched on 2026-06-02 (commit 7ffa38fd) and already reflect the post-066 reality.

---

## Validation Checklist

- [x] Every queued spec analyzed (76 numbered + 2 ops = 78).
- [x] Every spec has a trust classification with supporting evidence (CURRENT items grouped; non-CURRENT enumerated individually).
- [x] File-path references in MAJOR_DRIFT finding verified against filesystem (`internal/api/domain_intent.go` confirmed deleted; `tests/integration/policy/legacy_absence_test.go` confirmed present).
- [x] Git history used actual commit data (commits 1f74d5c0, 67792d82, 403daea4, bce20e26, 7ffa38fd).
- [x] Report written to `specs/_spec-review-report.md`.
- [x] Compact mode NOT engaged (user requested detect + classify only).
- [x] MAJOR_DRIFT route_required packet emitted (read-only audit — no `runSubagent` invocation).

---

## Addendum — 2026-06-10 (Spec 082 SCOPE-082-08)

> **Append-only.** The 2026-06-02 body above is preserved verbatim as the
> historical record. This addendum records portfolio drift observed during the
> spec 082 MVP / evo-x2 readiness review; it does NOT rewrite any prior finding.

The 2026-06-02 baseline ("76 numbered + 2 ops = 78") is now stale. Deltas
verified live on 2026-06-10:

- **Portfolio grew** beyond the 2026-06-02 snapshot: specs `077`–`082` now exist
  (`082-mvp-evo-x2-readiness-hardening` is this readiness-hardening feature).
- **Spec 076** (`assistant-completion-rescope`) is now `done` (certified
  2026-06-06), not in-progress. It carries two approved post-release exceptions
  (interactionMap removal; native iOS/Android adapters) — legitimate deferrals.
- **Spec 077** (`pwa-browser-test-harness`) shipped; the ops follow-up
  `F-057-V-001` is resolved, not open.
- **Spec 058** (`chrome-extension-bridge`) is `blocked` on a keyless-OIDC signing
  row that a local box cannot satisfy (needs one real tagged CI release). Its
  server-side ingest routes landed; only the signed-extension distributable is
  gated. Out of MVP go-live scope.
- The "2 in_progress" framing in the Summary no longer holds; treat the
  per-spec `state.json` as the authority for any go/no-go decision.

This addendum is the go-live decision-input refresh referenced by the
[Go-Live Readiness Checklist](../docs/Deployment.md#go-live-readiness-checklist-evo-x2--home-lab).
A full portfolio re-classification (re-running detect + classify across all
`specs/0*/`) remains future work; this addendum only corrects the inputs that
materially affect the evo-x2 MVP go-live.
