# Spec Review Report — Portfolio Audit (detect + classify)

**Generated:** 2026-06-02 by `bubbles.spec-review` (alias: Gary Laser Eyes)
**Mode:** detect + classify (no compaction)
**Scope:** All `specs/0*/` (76 numbered specs) + `specs/_ops/*` (2 ops packets)
**Depth:** quick (state.json + git history + targeted file-existence checks; not full per-spec behavioural cross-check)

---

## Summary

| Trust level | Count | Notes |
|-------------|-------|-------|
| **CURRENT** (fresh) | 65 | Spec accurately reflects current implementation. Safe to treat as source of truth. |
| **MINOR_DRIFT** (mostly fresh) | 7 | Small staleness — prose mentions of retired files, cosmetic banner pending, or open follow-up packet. Usable but verify specific details. |
| **MAJOR_DRIFT** | 1 | spec 026 — scenario-manifest links to tests deleted by spec 066. |
| **OBSOLETE** | 0 | None detected. |
| **PARTIAL / in-progress** | 2 | spec 076 (in_progress) + ops follow-up packet F-057-V-001 (open). |

No spec requires rewrite. The single MAJOR_DRIFT finding (spec 026) requires a scenario-manifest cleanup that should be routed to `bubbles.workflow mode=improve-existing`.

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

#### specs/026-domain-extraction
- **Drift:** `scenario-manifest.json` contains 11+ links to `internal/api/domain_intent_test.go::TestParseDomainIntent_*` and to `internal/api/domain_intent.go::parseDomainIntent`. Both files were **deleted** by spec 066 SCOPE-4 (commit 1f74d5c0 / "wip: round 4", confirmed by `tests/integration/policy/legacy_absence_test.go` enforcing absence).
- **Self-awareness:** `spec.md` line 12 + `design.md` line 15 already note the parser is "superseded by" the domain-extraction pipeline. The prose is fresh; the manifest is not.
- **Impact:** Any agent loading `026/scenario-manifest.json` (traceability guard, regression baseline, test wiring) will resolve dead file paths.
- **Required dispatch:** `bubbles.workflow mode=improve-existing spec=specs/026-domain-extraction reason=spec-review:MAJOR_DRIFT` — to rewire the affected scenarios to their current canonical test homes (domain-extraction pipeline tests under `internal/intelligence/` / spec 068 compiler tests) OR mark the linked tests as historically-removed in the manifest.

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

All other 65 specs classify as CURRENT based on:
- `status` terminal-for-mode (`done`, `specs_hardened`, `delivered_pending_activation`),
- `certification.status` matching `status`,
- no recent (post-cert) commits to their primary code paths that would contradict the spec, and
- no references to files deleted/moved by later specs.

Listed alphabetically:

```
001-smackerel-mvp                       024-design-doc-reconciliation         050-ml-sidecar-health-isolation
002-phase1-foundation                   025-knowledge-synthesis-layer         051-deployment-secret-auth-contract
003-phase2-ingestion                    027-user-annotations                  052-bundle-secret-injection-contract
004-phase3-intelligence                 028-actionable-lists                  053-ci-ops-evidence-hardening
005-phase4-expansion                    029-devops-pipeline                   054-notification-intelligence-handler
006-phase5-advanced                     030-observability                     055-notification-source-ntfy-adapter
007-google-keep-connector               031-live-stack-testing                056-twitter-api-connector
008-telegram-share-capture              032-documentation-freshness           057-browser-login-redirect
009-bookmarks-connector                 033-mobile-capture                    059-google-keep-live-mode
010-browser-history-connector           034-expense-tracking                  060-bearer-auth-scope-claim
011-maps-connector                      035-recipe-enhancements               061-conversational-assistant
012-hospitable-connector                036-meal-planning                     063-knowledge-ai-enrichment
013-guesthost-connector                 037-llm-agent-tools                   064-open-ended-knowledge-agent
014-discord-connector                   038-cloud-drives-integration          068-structured-intent-compiler
015-twitter-connector                   040-cloud-photo-libraries             069-assistant-http-transport
016-weather-connector                   041-qf-companion-connector            070-web-username-password-login
017-gov-alerts-connector                042-tailnet-edge-bind-pattern         071-intent-trace-observability
018-financial-markets-connector         043-ollama-test-infrastructure        072-whatsapp-business-transport
019-connector-wiring                    044-per-user-bearer-auth              073-web-mobile-assistant-frontend
020-security-hardening                  045-deploy-resource-filesystem-…     074-capture-as-fallback-policy
021-intelligence-delivery               046-nats-production-hardening
022-operational-resilience              047-ci-image-vulnerability-gate
023-engineering-quality                 048-backup-restore-automation
                                        049-monitoring-stack
```

Verify-before-trust caveats inside CURRENT:
- **Recent assistant-stack specs (061, 063, 064, 068, 069, 070, 071, 072, 073, 074)** were certified in the last 14 days during heavy interconnected churn. Their internal coverage is fresh, but cross-spec references (e.g. a spec citing another spec's scenario IDs) should be spot-checked before reuse.
- **Phase roadmap specs (001-006)** are intentionally high-level — they describe what each phase delivered, not current code. Treat as historical roadmap, not implementation contract.

---

## Auto-Dispatch Decisions (Phase 5)

| Trigger | Required dispatch | Status |
|---------|-------------------|--------|
| `specs/026-domain-extraction` MAJOR_DRIFT (status `done`) | `bubbles.workflow mode=improve-existing spec=specs/026-domain-extraction reason=spec-review:MAJOR_DRIFT` | **NOT INVOKED** in this read-only audit — emitted as a route_required packet below. |
| MINOR_DRIFT items above | None auto-required (per skill: MINOR_DRIFT does not auto-dispatch) | Added to follow-up suggestions. |

### route_required packet

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
