# Spec Review: 038 Cloud Drives Integration

> **Reviewer:** bubbles.spec-review (Gary Laser Eyes)
> **Date:** 2026-05-02
> **Mode:** pre-feature-done audit
> **Scope:** spec.md, design.md, scopes.md, scenario-manifest.json vs. current implementation

---

## Trust Classification: **CURRENT** (with cosmetic MINOR_DRIFT artifact-freshness items)

The spec is an authoritative source of truth for design decisions, FRs, scenarios, and contracts. All 24 scenarios (SCN-038-001..024) align across [spec.md](spec.md), [scopes.md](scopes.md), [scenario-manifest.json](scenario-manifest.json), and the actual test code. Implementation faithfully realizes the documented design. Audit findings A-001 and A-002 were properly reconciled by the docs phase (verified). Only cosmetic stale-status markers remain.

**Verdict for `bubbles.validate`:** Spec is **safe to use as the authoritative contract** for feature-done promotion. The MINOR_DRIFT items below are non-blocking but should be cleaned up before final feature-done strict promotion.

---

## Audit Summary

| Check | Result |
|---|---|
| All 24 scenario IDs consistent across spec.md / scopes.md / scenario-manifest.json | ✅ PASS |
| All 24 scenario `liveTestExpectation` test files exist | ✅ PASS (verified 24/24 in [tests/e2e/drive/](../../tests/e2e/drive/)) |
| All 24 scenario test function names exist in code | ✅ PASS |
| Implementation directories exist per [design.md](design.md) §1 | ✅ PASS ([internal/drive/](../../internal/drive/) with all subpackages: `google/`, `save/`, `retrieve/`, `tools/`, `policy/`, `confirm/`, `extract/`, `consumers/`, `monitor/`, `scan/`, `health/`, `rules/`, `observability/`, `memprovider/`) |
| Audit finding A-001 reconciled in [design.md](design.md) §2.3 (deferred-credentials-vault) | ✅ PASS (lines 263–273 explicitly call out decision-log A1 + Scope 1 plaintext bearer + deferred vault) |
| Audit finding A-001 decision recorded in [design.md](design.md) §11 decision-log "Resolved A1" | ✅ PASS (lines 905–945) |
| Audit finding A-002 reconciled in [internal/drive/google/google.go](../../internal/drive/google/google.go) source comment | ✅ PASS (lines 264–274 attribute credentials choice to design §2.3 + §11 decision-log A1, NOT Scope 6 / decision B2) |
| `bash .github/bubbles/scripts/artifact-lint.sh specs/038-cloud-drives-integration` | ✅ PASS (only deprecated-field warnings on legacy state.json fields) |
| `bash .github/bubbles/scripts/traceability-guard.sh specs/038-cloud-drives-integration` | ✅ PASS (24/24 scenarios mapped to DoD, 24/24 mapped to test files, 24/24 mapped to report evidence, 0 unmapped, 0 warnings) |
| Per-scope `Status: [x] Done` markers in [scopes.md](scopes.md) | ✅ PASS (8/8 scopes correctly marked Done at the per-scope sections) |
| Top-level state.json `certification.completedScopes` | ✅ PASS (all 8 scopes listed) |
| All 8 scope `validate` certifications + audit + chaos + docs in `certifiedCompletedPhases` | ✅ PASS (11 entries) |

---

## Findings

### SR-038-F1 — Scope Summary table shows wrong Status for 6 of 8 scopes (MINOR_DRIFT, cosmetic)

**Location:** [scopes.md](scopes.md) lines 56–67 ("Scope Summary" table)

**Drift:** The Scope Summary table marks scopes 1, 4, 5, 6, 7, and 8 as `Not Started`. The truth — per the per-scope `Status: [x] Done` markers in the same file (lines 84, 1221, 1350, 1506, 1637, 1766) and per [state.json](state.json) `certification.completedScopes` + `certifiedCompletedPhases` — is that all 8 scopes are Done and validate-certified.

**Current table value vs. truth:**

| Scope | Summary Table Says | Per-Scope Marker Says | state.json Certification | Truth |
|---|---|---|---|---|
| 1 Drive Foundation | Not Started | Done | certified 2026-04-27 | Done |
| 2 Scan And Monitor | Done | Done | certified 2026-04-30 | Done |
| 3 Extraction And Classification | Done | Done | certified 2026-04-30 | Done |
| 4 Search And Detail | Not Started | Done | certified 2026-04-30 | Done |
| 5 Save Rules And Write-Back | Not Started | Done | certified 2026-04-30 | Done |
| 6 Policy And Confirmation | Not Started | Done | certified 2026-05-01 | Done |
| 7 Retrieval And Agent Tools | Not Started | Done | certified 2026-05-02 | Done |
| 8 Cross-Feature And Scale | Not Started | Done | certified 2026-05-02 | Done |

**Risk:** Low. A maintenance agent skimming only the summary table would see contradictory information vs. the per-scope markers. The per-scope markers and state.json are authoritative; the summary table was simply not updated when implement phases ran.

**Recommendation:** Sync 6 status cells before feature-done strict promotion. Trivial single-cell edits. Owner: `bubbles.plan` or any phase agent that updates scopes.md next.

**Behavior impact:** None.

---

### SR-038-F2 — scenario-manifest.json claims feature `not_started` with all 24 scenarios `not_started` (MINOR_DRIFT, overlaps audit F-V1)

**Location:** [scenario-manifest.json](scenario-manifest.json) line 7 (top-level) and lines 24, 33, 42, 51, 60, 69, 78, 87, 96, 105, 114, 123, 132, 141, 150, 159, 168, 177, 186, 195, 204, 213, 222, 231 (per-scenario)

**Drift:** Top-level `"status": "not_started"` and every per-scenario `"status": "not_started"` were never updated despite all 24 scenarios being validated/certified across Scopes 1–8. Every scenario also has `"evidenceRefs": []` while [report.md](report.md) contains dozens of report-anchored evidence sections per scenario.

**Risk:** Low. Manifest is informational; runtime never reads it. However, this overlaps with the already-tracked audit finding **F-V1** (`scenario-manifest.json missing structured requiredTestType/linkedTests fields per Gate G057`). The 040 manifest already uses the newer structured format; the 038 manifest should be upgraded to the same shape and have status fields populated in the same change.

**Recommendation:** Upgrade 038's manifest to the v2 structured shape ([specs/040-cloud-photo-libraries/scenario-manifest.json](../040-cloud-photo-libraries/scenario-manifest.json) is the reference) — populate `requiredTestType`, `linkedTests`, `evidenceRefs`, and per-scenario `status: "active"` (or `"done"` once validated). This single change closes both F-V1 and SR-038-F2. Owner: `bubbles.plan`.

**Behavior impact:** None.

---

## Carry-Forward Findings (Already Tracked — Not New Spec-Review Findings)

These are recorded here only so a maintenance agent reading this report can see the full surface state.

| ID | Class | Owner | Source | Status |
|---|---|---|---|---|
| A-001 | security/docs (medium) | bubbles.docs | audit phase | ✅ RECONCILED — design §2.3 + §11 decision-log A1 + source comment all aligned. Verified in this review. |
| A-002 | docs (info) | bubbles.docs | audit phase | ✅ RECONCILED — source comment at [internal/drive/google/google.go](../../internal/drive/google/google.go) lines 264–274 corrected. Verified in this review. |
| F-V1 | planning (medium) | bubbles.plan | audit phase | 🟡 OPEN — overlaps with SR-038-F2; one fix closes both. |
| F-V2 | provenance (info) | bubbles.workflow / bubbles.plan | audit phase | 🟡 OPEN — execution.completedPhaseClaims provenance gap, unrelated to spec drift. |
| C-001..C-004 | chaos hardening | bubbles.harden | chaos phase | 🟡 OPEN — runtime hardening items, NOT spec drift. Spec correctly defines the contracts; runtime behavior at edge cases needs cleanup. |

---

## Per-Artifact Trust

| Artifact | Trust | Notes |
|---|---|---|
| [spec.md](spec.md) | CURRENT | All 18 FRs, 11 UCs, 25 BS scenarios, 11 UI screens accurately describe implementation. No contradictions found. |
| [design.md](design.md) | CURRENT | Architecture, Provider interface (BeginConnect/FinalizeConnect), schema, NATS contract, decision-log, capability model — all match [internal/drive/](../../internal/drive/) implementation. Audit reconciliations from docs phase verified. |
| [scopes.md](scopes.md) | CURRENT (with SR-038-F1 stale summary table) | Per-scope content + DoD evidence blocks all match reality. Only the summary table at the top is stale. |
| [scenario-manifest.json](scenario-manifest.json) | MINOR_DRIFT | Schema is the older flat format (F-V1), and status fields were never advanced (SR-038-F2). One refactor closes both. |
| [state.json](state.json) | CURRENT | All certifications, scope progress, completed phases match reality. |
| [report.md](report.md) | CURRENT | Per-scope evidence + audit + chaos + docs phase sections present and aligned with state.json. |
| [test-plan.json](test-plan.json) | Not reviewed in detail | Out of scope for this freshness audit; traceability guard already validated linkage. |
| [uservalidation.md](uservalidation.md) | Not reviewed in detail | Out of scope for this freshness audit. |

---

## Maintenance Context Block (For Downstream Agents)

```markdown
## Spec Trust Map for 038 (generated by bubbles.spec-review on 2026-05-02)

### CURRENT — Safe to use as authoritative truth
- specs/038-cloud-drives-integration/spec.md
- specs/038-cloud-drives-integration/design.md
- specs/038-cloud-drives-integration/scopes.md (per-scope content + DoD evidence)
- specs/038-cloud-drives-integration/state.json
- specs/038-cloud-drives-integration/report.md

### MINOR_DRIFT — Usable but ignore the noted stale fragments
- specs/038-cloud-drives-integration/scopes.md "Scope Summary" table (lines 56-67) — 6 of 8 status cells stale; trust the per-scope `Status: [x] Done` markers and state.json instead.
- specs/038-cloud-drives-integration/scenario-manifest.json — top-level + every per-scenario status field stale + missing structured fields (overlaps audit F-V1). Trust scopes.md test plan tables and traceability-guard output instead.

### Guidance for bubbles.validate (next phase)
- Spec is canonical for design decisions, contracts, and scenarios. Safe to use as feature-done baseline.
- Two stale-fragment items above are non-blocking for promotion but should be cleaned up before final strict promotion. Recommend: ask bubbles.plan to do a one-shot manifest upgrade + summary-table sync.
- Audit findings A-001 / A-002 are RECONCILED. Do not re-flag.
- Chaos findings C-001..C-004 belong to bubbles.harden, not validate.

### Guidance for bubbles.docs (if invoked again)
- Managed docs were already updated by docs phase 2026-05-02T19:25 (Connector_Development.md, Operations.md, Development.md, Testing.md). No new doc drift detected by this review.
```

---

## Phase Completion

- **Phase:** spec-review
- **Agent:** bubbles.spec-review
- **Mode:** pre-feature-done
- **Verdict:** CURRENT (canonical) — safe for `bubbles.validate` feature-done promotion
- **Findings:** 2 MINOR_DRIFT (cosmetic, non-blocking), both narrow and trivial to fix
- **Auto-invocation of bubbles.docs:** NOT triggered (CURRENT/MINOR_DRIFT does not require docs sync per spec-review mode rules)
- **Next required owner:** `bubbles.validate` may proceed; recommend `bubbles.plan` quick sync of SR-038-F1 + SR-038-F2 (and audit F-V1) before final strict promotion
