# Spec Review: 040 Cloud Photo Libraries

> **Reviewer:** bubbles.spec-review (Gary Laser Eyes)
> **Date:** 2026-05-02
> **Mode:** pre-feature-done audit
> **Scope:** spec.md, design.md, scopes.md, scenario-manifest.json vs. current implementation

---

## Trust Classification: **CURRENT** (with one cosmetic MINOR_DRIFT artifact-freshness item)

The spec is an authoritative source of truth for design decisions, FRs, scenarios, and contracts. All 15 scenarios (SCN-040-001..015) align across [spec.md](spec.md), [scopes.md](scopes.md), [scenario-manifest.json](scenario-manifest.json), and the actual test code. Implementation faithfully realizes the documented design including the provider-neutral `PhotoLibrary` contract, capability matrix with limitation codes, sensitivity model, and stable-signal boundary. Manifest already uses the structured Gate G057 format. Only one cosmetic stale-status table remains.

**Verdict for `bubbles.validate`:** Spec is **safe to use as the authoritative contract** for feature-done promotion. The single MINOR_DRIFT item below is non-blocking but should be cleaned up before final feature-done strict promotion.

---

## Audit Summary

| Check | Result |
|---|---|
| All 15 scenario IDs consistent across spec.md / scopes.md / scenario-manifest.json | ✅ PASS |
| All 15 scenario `linkedTests` test files exist | ✅ PASS (verified in [tests/integration/](../../tests/integration/), [tests/e2e/](../../tests/e2e/), [tests/stress/](../../tests/stress/), [ml/tests/](../../ml/tests/), [internal/](../../internal/), [web/pwa/tests/](../../web/pwa/tests/)) |
| All 15 scenario test function names exist in code | ✅ PASS (15/15 verified for integration + e2e + stress; PWA + ML linked-test paths also exist per implement-phase evidence in state.json) |
| Implementation directories exist per [design.md](design.md) §2/§3 | ✅ PASS ([internal/connector/photos/](../../internal/connector/photos/) with `library.go`, `scanner.go`, `lifecycle.go`, `dedupe.go`, `removal.go`, `routing.go`, `sensitivity.go`, `capability_taxonomy.go`, `cross_provider.go`, `action_tokens.go`, `writer_guard.go`, `stable_signals.go`, `classification.go`, `store.go`, `exif.go`; adapters: `immich/`, `photoprism/`) |
| Sensitivity model (`SensitivityHidden`/`SensitivitySensitive`/`sensitivity_labels`) matches design §Resolved Decisions | ✅ PASS ([internal/connector/photos/sensitivity.go](../../internal/connector/photos/sensitivity.go) lines 24, 25, 39, 51, 61, 76 confirm the documented coarse `none|sensitive|hidden` plus multi-label reasons) |
| Capability taxonomy (`LimitationCode` + `LimitationDescriptor`) matches design §3.1/§3.2 | ✅ PASS ([internal/connector/photos/capability_taxonomy.go](../../internal/connector/photos/capability_taxonomy.go) implements the design's 8 LimitationCode constants and AllLimitationDescriptors registry) |
| Manifest uses structured Gate G057 fields (`requiredTestType`, `linkedTests`, `evidenceRefs`) | ✅ PASS (15/15 scenarios populated) |
| `bash .github/bubbles/scripts/artifact-lint.sh specs/040-cloud-photo-libraries` | ✅ PASS (only one deprecated-field warning on legacy state.json `scopeProgress`) |
| `bash .github/bubbles/scripts/traceability-guard.sh specs/040-cloud-photo-libraries` | ✅ PASS (15/15 scenarios mapped to DoD, 15/15 mapped to test files, 15/15 mapped to report evidence, 0 unmapped, 0 warnings) |
| Per-scope `**Status:** Done` markers in [scopes.md](scopes.md) | ✅ PASS (5/5 scopes correctly marked Done at the per-scope sections, lines 60, 140, 226, 322, 414) |
| Top-level state.json `certification.completedScopes` | ✅ PASS (all 5 scopes listed) |
| All 5 scope `validate` certifications + audit + chaos + docs in `certifiedCompletedPhases` | ✅ PASS (8 entries) |

---

## Findings

### SR-040-F1 — Scope Summary table shows wrong Status for scopes 3, 4, 5 (MINOR_DRIFT, cosmetic)

**Location:** [scopes.md](scopes.md) lines 47–53 ("Scope Summary" table)

**Drift:** The Scope Summary table marks scopes 3, 4, and 5 as `Not Started`. The truth — per the per-scope `**Status:** Done` markers in the same file (lines 60, 140, 226, 322, 414) and per [state.json](state.json) `certification.completedScopes` + `certifiedCompletedPhases` — is that all 5 scopes are Done and validate-certified.

**Current table value vs. truth:**

| Scope | Summary Table Says | Per-Scope Marker Says | state.json Certification | Truth |
|---|---|---|---|---|
| 1 Photo platform foundation | Done | Done | certified 2026-04-30 | Done |
| 2 Immich connect, scan, and search | Done | Done | certified 2026-04-30 | Done |
| 3 Lifecycle, duplicates, and removal review | **Not Started** | **Done** | certified 2026-05-01 | Done |
| 4 Capture, Telegram, and cross-feature routing | **Not Started** | **Done** | certified 2026-05-02 | Done |
| 5 Multi-provider capability governance and operations | **Not Started** | **Done** | certified 2026-05-02 | Done |

**Risk:** Low. A maintenance agent skimming only the summary table would see contradictory information vs. the per-scope markers. The per-scope markers and state.json are authoritative; the summary table was simply not updated when implement phases ran for Scopes 3–5.

**Recommendation:** Sync 3 status cells before feature-done strict promotion. Trivial single-cell edits. Owner: `bubbles.plan` or any phase agent that updates scopes.md next.

**Status:** ✅ CLOSED 2026-05-07 — Scope Summary table cells for scopes 3, 4, 5 synced to "Done" inline.

**Behavior impact:** None.

---

## Carry-Forward Findings (Already Tracked — Not New Spec-Review Findings)

These are recorded here only so a maintenance agent reading this report can see the full surface state.

| ID | Class | Owner | Source | Status |
|---|---|---|---|---|
| ml-main.py:75 SMACKEREL_AUTH_TOKEN | code SST hardening | bubbles.harden | audit phase (non-blocking) | 🟡 OPEN — explicitly routed to bubbles.harden by docs phase; not spec drift. |
| C-001..C-006 | chaos hardening | bubbles.harden | chaos phase | 🟡 OPEN — runtime hardening items, NOT spec drift. Spec correctly defines the contracts; runtime behavior at edge cases needs cleanup (notably C-003 photoprism POST/GET inconsistency, C-005 control-character query handling, C-006 unbounded action-plan scope). |

**Note on chaos C-003:** "GET /v1/photos/connectors advertises both immich AND photoprism, but POST /v1/photos/connectors[/test] rejects provider='photoprism'." This is a runtime contract bug, NOT spec drift. The spec correctly describes Immich-first + PhotoPrism as a Scope-5 cross-provider duplicate-signal adapter (read-only), not a full connect endpoint. The runtime should either drop photoprism from the list endpoint or accept it on connect — owner is `bubbles.harden`, not spec maintenance.

---

## Per-Artifact Trust

| Artifact | Trust | Notes |
|---|---|---|
| [spec.md](spec.md) | CURRENT | All 20 FRs, 13 UCs, 32 BS scenarios, 15 UI screens accurately describe implementation. Sensitivity model, lifecycle states, capability matrix, stable-signal boundary, and confirmation-token contract all match code. |
| [design.md](design.md) | CURRENT | Architecture, `PhotoLibrary`/`ProviderWriter` interface, capability matrix + limitation codes, schema, NATS subjects, decision records — all match [internal/connector/photos/](../../internal/connector/photos/) implementation. |
| [scopes.md](scopes.md) | CURRENT (with SR-040-F1 stale summary table) | Per-scope content + DoD evidence blocks all match reality. Only the summary table at the top is stale for 3 of 5 scopes. |
| [scenario-manifest.json](scenario-manifest.json) | CURRENT | Uses structured Gate G057 schema. All 15 scenarios have `requiredTestType`, `linkedTests`, `evidenceRefs`, and `status: "active"`. **This is the reference shape that 038's manifest should converge to.** |
| [state.json](state.json) | CURRENT | All certifications, scope progress, completed phases match reality. |
| [report.md](report.md) | CURRENT | Per-scope evidence + audit + chaos + docs phase sections present and aligned with state.json. |
| [test-plan.json](test-plan.json) | Not reviewed in detail | Out of scope for this freshness audit; traceability guard already validated linkage. |
| [uservalidation.md](uservalidation.md) | Not reviewed in detail | Out of scope for this freshness audit. |

---

## Maintenance Context Block (For Downstream Agents)

```markdown
## Spec Trust Map for 040 (generated by bubbles.spec-review on 2026-05-02)

### CURRENT — Safe to use as authoritative truth
- specs/040-cloud-photo-libraries/spec.md
- specs/040-cloud-photo-libraries/design.md
- specs/040-cloud-photo-libraries/scopes.md (per-scope content + DoD evidence)
- specs/040-cloud-photo-libraries/scenario-manifest.json (already in structured Gate G057 shape)
- specs/040-cloud-photo-libraries/state.json
- specs/040-cloud-photo-libraries/report.md

### MINOR_DRIFT — Usable but ignore the noted stale fragment
- specs/040-cloud-photo-libraries/scopes.md "Scope Summary" table (lines 47-53) — 3 of 5 status cells stale (scopes 3, 4, 5 marked "Not Started" but actually Done); trust the per-scope `**Status:** Done` markers and state.json instead.

### Guidance for bubbles.validate (next phase)
- Spec is canonical for design decisions, contracts, and scenarios. Safe to use as feature-done baseline.
- The stale summary-table cells above are non-blocking for promotion but should be cleaned up before final strict promotion. Recommend: ask bubbles.plan to do a one-shot summary-table sync.
- Chaos findings C-001..C-006 and the ml/app/main.py:75 SMACKEREL_AUTH_TOKEN observation belong to bubbles.harden, not validate.

### Guidance for bubbles.docs (if invoked again)
- Managed docs were already updated by docs phase 2026-05-02T19:25 (Connector_Development.md, Operations.md, Development.md, Testing.md). No new doc drift detected by this review.
```

---

## Phase Completion

- **Phase:** spec-review
- **Agent:** bubbles.spec-review
- **Mode:** pre-feature-done
- **Verdict:** CURRENT (canonical) — safe for `bubbles.validate` feature-done promotion
- **Findings:** 1 MINOR_DRIFT (cosmetic, non-blocking), trivial to fix
- **Auto-invocation of bubbles.docs:** NOT triggered (CURRENT/MINOR_DRIFT does not require docs sync per spec-review mode rules)
- **Next required owner:** `bubbles.validate` may proceed; recommend `bubbles.plan` quick sync of SR-040-F1 before final strict promotion
