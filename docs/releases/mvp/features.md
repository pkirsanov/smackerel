# Features — Smackerel MVP

<!--
  MACHINE-BINDING ANNOTATIONS — Gate G101 (release-delivery-reconciliation-guard.sh).
  The HTML-comment annotation lines that follow reconcile this packet's promised
  features against validate-certified, terminal spec truth. They are additive and
  invisible in rendered Markdown: the human prose tables below remain the
  authoritative narrative and are untouched by these bindings. See the guard header
  for the exact annotation grammar and vocabulary; only the "required" delivery
  class is enforced (a required entry MUST bind a terminal, validate-certified spec
  dir). Verified delivery truth: MVP readiness review + bubbles.plan disposition,
  2026-06-06. Re-run the gate with:
    bash .github/bubbles/scripts/release-delivery-reconciliation-guard.sh \
      --repo-root "$(pwd)" --phase mvp --require-coverage
-->
<!-- bubbles:reconciled-packet schemaVersion=1 phase=mvp -->

<!-- New-in-MVP items (M1a..M5d) -->
<!-- bubbles:feature id=m1a-surfacing-prioritizer spec=specs/078-cross-surface-surfacing-prioritizer delivery=required -->
<!-- bubbles:feature id=m1b-calendar-triggered-briefs spec=specs/025-knowledge-synthesis-layer delivery=deferred-to:release-v1 -->
<!-- bubbles:feature id=m1c-basic-reminders spec=specs/054-notification-intelligence-handler delivery=carried -->
<!-- bubbles:feature id=m1c-promise-engine-full spec=specs/025-knowledge-synthesis-layer delivery=deferred-to:release-v1 -->
<!-- bubbles:feature id=m2a-graph-browse-surface spec=specs/073-web-mobile-assistant-frontend delivery=required -->
<!-- bubbles:feature id=m2b-wiki-editable-annotations spec=specs/027-user-annotations delivery=required -->
<!-- bubbles:feature id=m3-ratify-product-principles spec=none delivery=optional -->
<!-- bubbles:feature id=m4-domain-extraction-manifest-fix spec=specs/026-domain-extraction delivery=required -->
<!-- bubbles:feature id=m5a-recommendations-drift spec=specs/039-recommendations-engine delivery=carried -->
<!-- bubbles:feature id=m5b-chrome-extension-bridge spec=specs/058-chrome-extension-bridge delivery=deferred-to:release-v1 -->
<!-- bubbles:feature id=m5c-intent-policy-drift spec=specs/067-intent-driven-policy-enforcement delivery=carried -->
<!-- bubbles:feature id=m5d-spec-banner-sweep spec=specs/_ops/OPS-001-spec-banner-sweep delivery=required -->

<!-- Carry-forward delivered capabilities (carried = recorded, not delivery-enforced) -->
<!-- bubbles:feature id=cf-core-runtime-stack spec=specs/002-phase1-foundation delivery=carried -->
<!-- bubbles:feature id=cf-capture-pipeline spec=specs/003-phase2-ingestion delivery=carried -->
<!-- bubbles:feature id=cf-intelligence-delivery spec=specs/021-intelligence-delivery delivery=carried -->
<!-- bubbles:feature id=cf-maps-people-intel spec=specs/011-maps-connector delivery=carried -->
<!-- bubbles:feature id=cf-phase5-advanced spec=specs/006-phase5-advanced delivery=carried -->
<!-- bubbles:feature id=cf-actionable-lists spec=specs/028-actionable-lists delivery=carried -->
<!-- bubbles:feature id=cf-devops-pipeline spec=specs/029-devops-pipeline delivery=carried -->
<!-- bubbles:feature id=cf-mobile-capture spec=specs/033-mobile-capture delivery=carried -->
<!-- bubbles:feature id=cf-llm-agent-tools spec=specs/037-llm-agent-tools delivery=carried -->
<!-- bubbles:feature id=cf-cloud-drives spec=specs/038-cloud-drives-integration delivery=carried -->
<!-- bubbles:feature id=cf-cloud-photos spec=specs/040-cloud-photo-libraries delivery=carried -->
<!-- bubbles:feature id=cf-qf-companion-connector spec=specs/041-qf-companion-connector delivery=carried -->
<!-- bubbles:feature id=cf-tailnet-edge spec=specs/042-tailnet-edge-bind-pattern delivery=carried -->
<!-- bubbles:feature id=cf-bearer-auth spec=specs/044-per-user-bearer-auth delivery=carried -->
<!-- bubbles:feature id=cf-nats-hardening spec=specs/046-nats-production-hardening delivery=carried -->
<!-- bubbles:feature id=cf-backup-restore spec=specs/048-backup-restore-automation delivery=carried -->
<!-- bubbles:feature id=cf-ml-health-isolation spec=specs/050-ml-sidecar-health-isolation delivery=carried -->
<!-- bubbles:feature id=cf-twitter-connector spec=specs/056-twitter-api-connector delivery=carried -->
<!-- bubbles:feature id=cf-google-keep-live spec=specs/059-google-keep-live-mode delivery=carried -->
<!-- bubbles:feature id=cf-knowledge-ai-enrichment spec=specs/063-knowledge-ai-enrichment delivery=carried -->
<!-- bubbles:feature id=cf-structured-intent-compiler spec=specs/068-structured-intent-compiler delivery=carried -->
<!-- bubbles:feature id=cf-whatsapp-transport spec=specs/072-whatsapp-business-transport delivery=carried -->
<!-- bubbles:feature id=cf-capture-as-fallback spec=specs/074-capture-as-fallback-policy delivery=carried -->
<!-- bubbles:feature id=cf-roadmap-specs spec=specs/001-smackerel-mvp delivery=carried -->

## Carried Forward From Prior Phases (Phase 1–5)

All capabilities below were certified `done` (or equivalent terminal-for-mode) before this MVP gate. They are carried forward into MVP as-is and are NOT re-implemented by this packet. Sources: [`docs/INVESTOR_OVERVIEW.md`](../../INVESTOR_OVERVIEW.md) Phase Overview, [`specs/_spec-review-report.md`](../../../specs/_spec-review-report.md) (65 CURRENT specs).

| Capability | Origin spec(s) | Status |
|------------|----------------|--------|
| Go core + Docker Compose stack + `./smackerel.sh` runtime | 001, 002 | delivered |
| PostgreSQL + pgvector + pg_trgm | 002 | delivered |
| NATS JetStream bus + production hardening | 002, 046 | delivered |
| Python ML sidecar (FastAPI + Ollama) | 002, 050 | delivered |
| Universal capture/processing pipeline | 002, 003 | delivered |
| Knowledge graph + topic lifecycle | 003 | delivered (063 AI-enrichment layer is planning-only, `specs_hardened` — see note below) |
| Semantic search (vague-in / precise-out) | 003 + 068 | delivered (063 AI-enrichment layer is planning-only, `specs_hardened` — see note below) |
| Daily/weekly digest + Telegram delivery | 021 (subset) | delivered |
| Pre-meeting briefs + contextual alerts (baseline) | 021, 025 | delivered |
| Maps timeline + trip dossier + people intelligence | 005 + 011, 005 + 015, 005 + IMAP/CalDAV | delivered |
| Expertise mapping, content fuel, learning paths, subscription tracking, serendipity | 006 family | delivered |
| **Connector roster (LOCKED in MVP)** — see table below | 007–018, 038, 040, 041, 056, 059, 072, 074, ingest | delivered |
| Annotations (baseline) | 027 | delivered |
| Actionable lists | 028 | delivered |
| DevOps pipeline + observability + monitoring | 029, 030, 047, 049, 053 | delivered |
| Live-stack testing + ML sidecar health isolation | 031, 043, 050 | delivered |
| Documentation freshness + spec-banner sweep | 032, _ops/OPS-001 | delivered |
| Mobile capture (PWA) | 033 | delivered |
| Expense tracking, recipe enhancements, meal planning | 034, 035, 036 | delivered |
| LLM agent tools + structured intent compiler + intent-driven policy enforcement | 037, 068, 067 | delivered |
| Cloud drives + cloud photo libraries + QF companion connector | 038, 040, 041 | delivered |
| Tailnet-edge bind pattern | 042 | delivered |
| Per-user bearer auth + bearer auth scope claim + deployment secret/auth contracts | 044, 060, 051, 052 | delivered |
| Deploy resource filesystem hardening | 045 | delivered |
| Backup/restore automation | 048 | delivered |
| Notification intelligence handler (baseline) | 054 | delivered |
| Notification source ntfy adapter | 055 | delivered |
| Twitter API connector | 056 | delivered |
| Browser login redirect | 057 | delivered (`done`) |
| Chrome extension bridge | 058 | deferred → release-v1 (now `blocked` on an operator-owned cosign CI release — see MVP item M5b / [`actions.md`](actions.md)) |
| Google Keep live mode | 059 | delivered |
| Conversational assistant + open-ended knowledge agent (delivers the knowledge-AI enrichment capability) | 061, 064 | delivered |
| Knowledge-AI enrichment (planning-hardening spec) | 063 | `specs_hardened` — planning-only (`product-to-planning`; authored zero source/test/migration diffs). The enrichment **capability** is delivered by 061/064; spec 063 itself is NOT a delivered implementation |
| Domain extraction pipeline | 026 | delivered (scenario-manifest drift resolved by MVP item M4) |
| Assistant HTTP transport + intent-trace observability | 069, 071 | delivered |
| Web username/password login + web/mobile assistant frontend | 070, 073 | delivered |
| WhatsApp Business transport | 072 | delivered |
| Capture-as-fallback policy | 074 | delivered |
| Phase roadmap specs (intentionally high-level) | 001, 002, 003, 004, 005, 006 | delivered |

### Connector roster (LOCKED at MVP — no additions in MVP)

Source-of-truth: [`internal/connector/`](../../../internal/connector/).

| Connector | Source spec(s) |
|-----------|----------------|
| `alerts` (gov alerts) | 017 |
| `bookmarks` | 009 |
| `browser` (history) | 010 |
| `caldav` (calendar) | — (Phase 2 ingestion family) |
| `discord` | 014 |
| `guesthost` | 013 |
| `hospitable` | 012 |
| `imap` (Gmail/IMAP baseline) | — (Phase 2 ingestion family) |
| `ingest` (universal active capture) | 002, 003 |
| `keep` (Google Keep) | 007, 059 |
| `maps` | 011 |
| `markets` (financial markets) | 018 |
| `photos` (cloud photo libraries) | 040 |
| `qfdecisions` (QF companion) | 041 |
| `rss` | — (Phase 2/4 family) |
| `twitter` | 015, 056 |
| `weather` | 016 |
| `youtube` | — (Phase 2 ingestion family) |
| `cloud-drives` | 038 |
| `telegram-share-capture` | 008 |
| `whatsapp-business-transport` | 072 |

Any new connector after this MVP gate is RELEASE-V1 scope.

## New In This Phase (MVP — operator decisions 2026-06-03; delivery reconciled 2026-06-06)

> The **Status** column was reconciled on 2026-06-06 against validate-certified,
> terminal `state.json` truth (MVP readiness review + bubbles.plan disposition).
> The machine-readable bindings for these rows are the Gate G101 annotations at the
> top of this file; `delivery=required` entries (M1a / M2a / M2b / M4 / M5d) each
> bind a terminal, validate-certified spec.

| ID | Capability | Owning spec(s) | Delivery shape | Status |
|----|------------|----------------|----------------|--------|
| **M1a** | Global user-interruption-budget controller ("Next Smackerel" prioritizer) — owns budget across digest + push + Telegram + web + ntfy + email-out, with measurable SLOs (≤ N nudges/day, target acted-on-rate, false-positive ceiling) | [`078-cross-surface-surfacing-prioritizer`](../../../specs/078-cross-surface-surfacing-prioritizer/) | Delivered as a dedicated spec that **adopted pre-existing in-tree controller groundwork**; **rescoped OUT of 021** by commit `640b95d0`. 5-channel × 7-producer controller, 5-decision vocabulary, `surfacing_*` metric families, SST keys | **delivered** (078 `done`, validate-certified, `releaseTrain: mvp`) |
| **M1b** | Calendar-triggered briefs — the **configurable per-category lead-time** generalization (e.g. 1-day trip briefs). NOTE: the marquee fixed **30-minute pre-meeting brief is already delivered in MVP** (spec 021); only the configurable lead-time generalization defers | [`025-knowledge-synthesis-layer`](../../../specs/025-knowledge-synthesis-layer/) Scope 9 | Portfolio-approved post-release deferral **DI-025-05**. **Clarification (do not read as "calendar briefs absent in MVP"):** the 30-minute pre-meeting brief IS delivered and wired in MVP via spec 021 — [`internal/intelligence/briefs.go`](../../../internal/intelligence/briefs.go) `GeneratePreMeetingBriefs` queries calendar events 25–35 min out and is registered as `ProducerPreMeetingBriefs` in the spec-078 surfacing controller. What defers to release-v1 is ONLY the configurable per-category lead-time generalization. The sole remaining blocker for that generalization — the M1a global interruption-budget controller — is now satisfied by spec 078 | **deferred → release-v1** (DI-025-05; the lead-time generalization only — the 30-min pre-meeting brief shipped via spec 021) |
| **M1c (basic)** | Basic time-based reminders ("ping me at Y") — scheduler-backed | [`054-notification-intelligence-handler`](../../../specs/054-notification-intelligence-handler/) + `internal/agent/tools/notification` + Telegram `/remind` | Basic time-based reminder path delivered and carried into MVP | **delivered (basic path)** |
| **M1c (full engine)** | Conditional / arrival promise engine ("ping me if X hasn't happened by Y", "remind me when Z arrives") | [`025-knowledge-synthesis-layer`](../../../specs/025-knowledge-synthesis-layer/) Scope 10 | Portfolio-approved post-release deferral **DI-025-05** | **deferred → release-v1** (DI-025-05; not MVP-required) |
| **M2a** | Wiki / graph-browse UI surface in web — views by topic / person / place / time; rendered cross-links | [`073-web-mobile-assistant-frontend`](../../../specs/073-web-mobile-assistant-frontend/) Scope 5 | Graph-browse pivot views + cross-link rendering delivered on the web/mobile assistant front-end | **delivered** (073 Scope 5 `done`, validate-certified) |
| **M2b** | In-wiki editable annotations (beyond the 027 baseline) | [`027-user-annotations`](../../../specs/027-user-annotations/) Scope 9 | Inline editable annotations on the wiki surface delivered & certified (`TestAnnotationEditingUI_FullFlow` PASS) | **delivered & certified** (027 Scope 9 `done`, validate-certified) |
| **M3** | Ratify [`docs/Product-Principles.md`](../../Product-Principles.md) 1–10 — flip [`.github/instructions/product-principles.instructions.md`](../../../.github/instructions/product-principles.instructions.md) from advisory to BLOCKING | `.github/instructions/product-principles.instructions.md` (no owning spec) | Single instruction-file edit; the file is now BLOCKING / ratified | **delivered** (instruction-file edit; `spec=none`) |
| **M4** | Spec 026 MAJOR_DRIFT fix — rewire `scenario-manifest.json` away from deleted `internal/api/domain_intent*.go` | [`026-domain-extraction`](../../../specs/026-domain-extraction/) | `improve-existing` revision; scenario-manifest drift resolved | **delivered** (026 `done`, validate-certified) |
| **M5a** | Spec 039 MINOR_DRIFT — re-point evidence cell | [`039-recommendations-engine`](../../../specs/039-recommendations-engine/) | `improve-existing` cosmetic | **delivered** (039 `done`) — **carried**, not delivery-required (pre-existing Gate G022 specialist-phase-record artifact-lint gap; see [`actions.md`](actions.md)) |
| **M5b** | Spec 058 — chrome-extension capture path | [`058-chrome-extension-bridge`](../../../specs/058-chrome-extension-bridge/) | Peripheral capture path, NOT MVP-critical | **deferred → release-v1** (058 `blocked` solely on an operator-owned keyless-OIDC cosign tagged-CI release; see [`actions.md`](actions.md)) |
| **M5c** | Spec 067 MINOR_DRIFT — past-tense inventory entry | [`067-intent-driven-policy-enforcement`](../../../specs/067-intent-driven-policy-enforcement/) | `improve-existing` cosmetic | **delivered** (067 `done`) — **carried**, not delivery-required (pre-existing Gate G022 artifact-lint gap; see [`actions.md`](actions.md)) |
| **M5d** | `_ops/OPS-001` EB-7 idempotence verification | [`_ops/OPS-001-spec-banner-sweep`](../../../specs/_ops/OPS-001-spec-banner-sweep/) | `validate-only` grep verification | **delivered** (OPS-001 `specs_hardened` = terminal-for-mode, validate-certified) |

**MVP delivery summary.** M1a / M2a / M2b / M4 / M5d are **delivered and validate-certified** — these are the Gate G101 `delivery=required` bindings. M1c's basic time-based reminder path is **delivered** and carried; M5a / M5c are **delivered** (`done`) but classified **carried** (kept out of the required set by a pre-existing Gate G022 specialist-phase-record artifact-lint gap on those two specs); M3 is **delivered** via a single instruction-file edit (no owning spec, `spec=none`). M1b, the M1c full conditional/arrival promise engine, and M5b are **portfolio-approved deferrals to release-v1** (M1b / M1c-full: DI-025-05, with the former sole blocker now satisfied by spec 078; M5b: 058 `blocked` on an operator-owned cosign CI release). Note: M1a was delivered through a **dedicated spec (078)** that adopted pre-existing in-tree controller groundwork rescoped out of 021 by commit `640b95d0`, rather than by extending 021 in place — so the earlier "no new spec is required for MVP" framing no longer holds for M1a.

### Post-reconciliation mvp-train specs (outside the M-series product-feature catalog)

The M-series catalog above was frozen at the 2026-06-06 readiness reconciliation. The
2026-06-23 reconciliation flagged two `releaseTrain: mvp` specs that reached terminal `done`
*after* that freeze — within the MVP window — as absent from this delivery record (finding
F-13). Both sit **outside** the M1–M5 product-feature catalog by design (one is a
delivery-channel extension, the other is build/infra), so neither is added to the M-series
table nor bound as a Gate G101 `delivery=required` feature. They are recorded here for
release-train completeness:

| Spec | Capability | Train | Status | Why outside the M-series catalog |
|------|------------|-------|--------|----------------------------------|
| [`097-card-rewards-gcal-delivery`](../../../specs/097-card-rewards-gcal-delivery/) | Card-rewards → Google Calendar delivery channel | `mvp` | `done` (full-delivery, validate-certified) | Delivery-channel extension of the card-rewards companion family — not a standalone M-series product feature |
| [`099-preflight-resource-guard`](../../../specs/099-preflight-resource-guard/) | Pre-flight host-resource (OOM) guard for the build/test runtime | `mvp` | `done` (full-delivery, validate-certified) | Build/infra hardening — no end-user product surface |

These rows are informational lineage only; they are NOT Gate G101 `delivery=required`
bindings and do not change the enforced required set (M1a / M2a / M2b / M4 / M5d). This
record is not asserted to be an exhaustive census of post-freeze mvp-train specs — it captures
the two specs flagged by finding F-13.

## Plan-to-Release Traceability

| MVP item | Delivered / bound spec | Workflow mode | G101 delivery |
|----------|------------------------|---------------|---------------|
| M1a | `specs/078-cross-surface-surfacing-prioritizer` | `improve-existing` | required |
| M1b | `specs/025-knowledge-synthesis-layer` (Scope 9) | deferred | deferred-to:release-v1 |
| M1c (basic) | `specs/054-notification-intelligence-handler` | `improve-existing` | carried |
| M1c (full engine) | `specs/025-knowledge-synthesis-layer` (Scope 10) | deferred | deferred-to:release-v1 |
| M2a | `specs/073-web-mobile-assistant-frontend` (Scope 5) | `improve-existing` | required |
| M2b | `specs/027-user-annotations` (Scope 9) | `improve-existing` | required |
| M3  | `.github/instructions/product-principles.instructions.md` | `bubbles.docs` single-file edit | optional (`spec=none`) |
| M4  | `specs/026-domain-extraction` | `improve-existing` | required |
| M5a | `specs/039-recommendations-engine` | `improve-existing` | carried |
| M5b | `specs/058-chrome-extension-bridge` | `improve-existing` (blocked) | deferred-to:release-v1 |
| M5c | `specs/067-intent-driven-policy-enforcement` | `improve-existing` | carried |
| M5d | `specs/_ops/OPS-001-spec-banner-sweep` | `validate-only` | required |

## Capability evidence trace

Every "delivered" claim in the carry-forward table traces to a spec folder under [`specs/`](../../../specs/) whose `state.json` was certified at `done` (or terminal-for-mode equivalent) per [`specs/_spec-review-report.md`](../../../specs/_spec-review-report.md) audit on 2026-06-02. No capability is claimed delivered without a spec reference. No capability is fabricated.

Every new-in-MVP delivery claim is machine-bound by the Gate G101 annotations at the top of this file and was verified against validate-certified, terminal `state.json` truth on 2026-06-06 (the `delivery=required` entries — M1a / M2a / M2b / M4 / M5d — each bind a spec whose completed phases include `validate`). Deferrals (M1b, M1c full engine, M5b) and carried items (M1c basic, M5a, M5c) are recorded but not delivery-enforced. No capability is claimed delivered without a spec reference, and no capability is fabricated. Spec 081 (Python NATS parity) is `done` on the `next` train and is intentionally out of this MVP packet — recorded here only as next-train lineage.

## Release-train classification (grandfathered pre-078 specs)

The [release-trains policy](../../Release_Trains.md) ([`config/release-trains.yaml`](../../../config/release-trains.yaml))
expects every spec to declare a `releaseTrain` field. Spec **078** is the first spec authored
under that policy and the first to carry the field; the ~81 numbered specs predating 078 (the
Phase 1–5 carry-forward set) do **not** carry a per-spec `releaseTrain` field. Their MVP-train
membership is established **by this packet's Gate G101 per-feature binding annotations**
(grandfathered), not by a per-spec field. This gap is intentional and explicit: the pre-078
specs are **NOT** being backfilled — the packet annotations are the authoritative train-membership
record for the MVP delivery set. All new specs (078+) carry `releaseTrain` directly, so the gap
does not grow.

## Deprecations in MVP

None. All carry-forward capabilities remain in scope. The 026 manifest drift (MVP item M4) was a manifest reconciliation, not a capability removal — the domain-extraction pipeline is delivered and in MVP. The release-v1 deferrals (M1b, the M1c full conditional/arrival promise engine, and the M5b chrome-extension bridge) are scheduling decisions, not deprecations — each remains a tracked, intended capability.
