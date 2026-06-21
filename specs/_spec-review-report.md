# Spec Review Report — Portfolio Audit (detect + classify)

**Generated:** 2026-06-20 by `bubbles.spec-review` (alias: Gary Laser Eyes)
**Mode:** detect + classify (no compaction); read-only `spec-review-to-doc`
**Scope:** All `specs/NNN-*/` (98 numbered specs, 001–098) + `specs/_ops/*` (8 packets)
**Depth:** quick (state.json + git history + targeted file-existence/drift checks; not full per-spec behavioural cross-check)

> This refresh supersedes the stale **2026-06-02 baseline** (which itself was
> flagged stale by spec 082) and the **2026-06-10 go-live addendum**. Both are
> preserved verbatim below under **[Historical Record](#historical-record)** for
> audit trail. Where they disagree with this refresh, the refresh wins. This is
> the full portfolio re-classification the 2026-06-10 addendum deferred.

---

## Summary (2026-06-20 refresh)

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
